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

package main

import (
	"testing"

	"github.com/ghchinoy/ax-images/internal/agent"
)

func TestParseOptions_DualMode(t *testing.T) {
	t.Run("baseline_without_dgem", func(t *testing.T) {
		cfg := &agent.Config{
			Values: map[string]any{
				"prompt":       "Inspect repository",
				"maxTurns":     float64(10),
				"builtinTools": []any{"google_search"},
			},
		}
		opts, err := parseOptions(cfg)
		if err != nil {
			t.Fatalf("parseOptions failed: %v", err)
		}
		if opts.safetyGate {
			t.Errorf("opts.safetyGate = true, want false in baseline mode")
		}
		if opts.agent != defaultAgent {
			t.Errorf("opts.agent = %q, want %q", opts.agent, defaultAgent)
		}
	})

	t.Run("dgem_safety_gate_enabled", func(t *testing.T) {
		cfg := &agent.Config{
			Values: map[string]any{
				"prompt":     "Deploy database migration",
				"safetyGate": true,
			},
		}
		opts, err := parseOptions(cfg)
		if err != nil {
			t.Fatalf("parseOptions failed: %v", err)
		}
		if !opts.safetyGate {
			t.Errorf("opts.safetyGate = false, want true when spec.config.safetyGate is set")
		}
	})
}
