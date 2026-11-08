// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Command dgem-agent acts as both the AX task runner (when invoked as
// ax-task-runner) and a System-1 DiffusionGemma (dgem) decision router CLI
// (`dgem-router -query "..."`) calling POST ${DGEM_GATEWAY_URL}/api/decide.
package main

import (
	"bytes"
	"cmp"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"math"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/ghchinoy/ax-images/internal/taskrunner"
)

const (
	hesitationGate      = 0.16
	runnerUpProbTrigger = 0.20
)

var specialistTaskMap = map[string]string{
	"billing_ops":  "specialist-billing",
	"sec_incident": "specialist-security",
	"code_triage":  "specialist-engineering",
}

// RouteDecision is the structured JSON output emitted by dgem-router.
type RouteDecision struct {
	TargetEndpoint  string             `json:"target_endpoint"`
	Model           string             `json:"model"`
	Query           string             `json:"query"`
	Specialist      string             `json:"specialist"`
	Confidence      float64            `json:"confidence"`
	Hesitation      float64            `json:"hesitation"`
	Band            string             `json:"band"`
	CrossDomain     string             `json:"cross_domain"`
	CrossDomainProb float64            `json:"cross_domain_prob"`
	Urgent          string             `json:"urgent"`
	UrgentProb      float64            `json:"urgent_prob"`
	Severity        string             `json:"severity"`
	SeverityScore   float64            `json:"severity_score"`
	WallTimeMs      int64              `json:"wall_ms"`
	Probabilities   map[string]float64 `json:"probabilities"`
	Targets         []string           `json:"targets"`
}

type optionProb struct {
	Option string
	Prob   float64
}

func main() {
	if taskrunner.Invoked() {
		taskrunner.Main()
		return
	}
	query := flag.String("query", os.Getenv("DGEM_QUERY"), "Incoming user question or incident description to route")
	tmplPath := flag.String("template", "/opt/dgem/templates/specialist_router.json.tmpl", "Path to specialist_router.json.tmpl")
	gwURL := flag.String("gateway-url", firstNonEmpty(os.Getenv("DGEM_GATEWAY_URL"), "http://192.168.64.1:8095"), "dgem serve gateway URL")
	modelID := flag.String("model", firstNonEmpty(os.Getenv("DGEM_MODEL"), "diffgemma-26b-a4b-it-q4"), "dgem model ID")
	backend := flag.String("backend", os.Getenv("DGEM_BACKEND"), "Optional X-DGem-Backend target")
	flag.Parse()

	if *query == "" {
		// When launched on container boot without -query, exit 0 so ax ssh can invoke dgem-router on demand.
		return
	}

	start := time.Now()
	bodyMap := map[string]any{
		"variables": map[string]any{
			"query":   *query,
			"samples": 1,
		},
		"layout": "document_first",
	}
	if rawTmpl, err := os.ReadFile(*tmplPath); err == nil {
		bodyMap["custom_template"] = string(rawTmpl)
	} else {
		bodyMap["template"] = "specialist_router"
	}

	payload, _ := json.Marshal(bodyMap)
	endpoint := strings.TrimRight(*gwURL, "/") + "/api/decide"
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		log.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if *backend != "" {
		req.Header.Set("X-DGem-Backend", *backend)
	}
	if tok := strings.TrimSpace(os.Getenv("DGEM_AUTH_TOKEN")); tok != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimPrefix(tok, "Bearer "))
	}

	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("dgem request failed: %v", err)
	}
	defer resp.Body.Close()
	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Fatalf("dgem HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBytes)))
	}
	wallMs := time.Since(start).Milliseconds()

	var parsed struct {
		Answers map[string]struct {
			Choice        string             `json:"choice"`
			Value         any                `json:"value"`
			Confidence    float64            `json:"confidence"`
			Score         float64            `json:"score"`
			Probabilities map[string]float64 `json:"probabilities"`
		} `json:"answers"`
	}
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		log.Fatalf("decode dgem response: %v", err)
	}

	specAns := parsed.Answers["specialist"]
	if specAns.Choice == "" {
		if s, ok := specAns.Value.(string); ok {
			specAns.Choice = s
		}
	}
	if specAns.Choice == "" {
		log.Fatalf("template %s did not return a valid 'specialist' choice slot", *tmplPath)
	}
	crossAns := parsed.Answers["cross_domain"]
	urgAns := parsed.Answers["urgent"]
	sevAns := parsed.Answers["severity"]

	var h float64
	for _, p := range specAns.Probabilities {
		if p > 0 {
			h -= p * math.Log(p)
		}
	}
	hes := math.Min(1.0, h/math.Log(3.0))
	band := "Clear"
	if hes >= 0.50 {
		band = "Very unsure"
	} else if hes >= hesitationGate {
		band = "Unsure"
	}

	crossProb := crossAns.Probabilities["yes"]
	crossLabel := "no"
	if crossProb >= 0.5 {
		crossLabel = "yes"
	}
	urgProb := urgAns.Probabilities["yes"]
	urgLabel := "no"
	if urgProb >= 0.5 {
		urgLabel = "yes"
	}

	sevLevels := []string{"low", "medium", "high", "critical"}
	sevIdx := int(math.Round(sevAns.Score))
	if sevIdx < 0 {
		sevIdx = 0
	}
	if sevIdx >= len(sevLevels) {
		sevIdx = len(sevLevels) - 1
	}

	var ranked []optionProb
	for k, p := range specAns.Probabilities {
		ranked = append(ranked, optionProb{Option: k, Prob: p})
	}
	slices.SortFunc(ranked, func(a, b optionProb) int {
		return cmp.Compare(b.Prob, a.Prob)
	})

	var targets []string
	if len(ranked) > 0 {
		if t, ok := specialistTaskMap[ranked[0].Option]; ok {
			targets = append(targets, t)
		}
	}
	if len(ranked) > 1 && (hes >= hesitationGate || crossLabel == "yes" || ranked[1].Prob >= runnerUpProbTrigger) {
		if t, ok := specialistTaskMap[ranked[1].Option]; ok {
			targets = append(targets, t)
		}
	}

	out := RouteDecision{
		TargetEndpoint:  fmt.Sprintf("gateway(%s)", endpoint),
		Model:           "dgem/" + *modelID,
		Query:           *query,
		Specialist:      specAns.Choice,
		Confidence:      specAns.Confidence,
		Hesitation:      hes,
		Band:            band,
		CrossDomain:     crossLabel,
		CrossDomainProb: crossProb,
		Urgent:          urgLabel,
		UrgentProb:      urgProb,
		Severity:        sevLevels[sevIdx],
		SeverityScore:   sevAns.Score,
		WallTimeMs:      wallMs,
		Probabilities:   specAns.Probabilities,
		Targets:         targets,
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)
	emitContainerOTelSpan(out)
}

func emitContainerOTelSpan(out RouteDecision) {
	f, err := os.OpenFile("/proc/1/fd/2", os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return
	}
	defer f.Close()
	b1 := make([]byte, 16)
	b2 := make([]byte, 8)
	_, _ = rand.Read(b1)
	_, _ = rand.Read(b2)
	endNano := time.Now().UnixNano()
	startNano := endNano - out.WallTimeMs*1e6
	logger := slog.New(slog.NewTextHandler(f, nil))
	logger.Info("genkit otel span",
		"trace_id", hex.EncodeToString(b1),
		"span_id", hex.EncodeToString(b2),
		"span", fmt.Sprintf("dgem/%s->%s", out.Specialist, strings.Join(out.Targets, ",")),
		"agent", "dgem-router",
		"subtype", fmt.Sprintf("hes=%.1f%%(%s)", out.Hesitation*100, out.Band),
		"status", "ok",
		"start_unix_nano", startNano,
		"end_unix_nano", endNano,
		"duration_ms", out.WallTimeMs,
	)
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return strings.TrimSpace(a)
	}
	return b
}
