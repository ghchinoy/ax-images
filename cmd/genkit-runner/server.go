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
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2acompat/a2av0"
	a2agrpcv0 "github.com/a2aproject/a2a-go/v2/a2agrpc/v0"
	a2agrpcv1 "github.com/a2aproject/a2a-go/v2/a2agrpc/v1"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
	"github.com/firebase/genkit/go/ai"
	aix "github.com/firebase/genkit/go/ai/exp"
	"github.com/firebase/genkit/go/ai/exp/localstore"
	"github.com/firebase/genkit/go/core/api"
	"github.com/firebase/genkit/go/genkit"
	genkitx "github.com/firebase/genkit/go/genkit/exp"
	"github.com/firebase/genkit/go/plugins/anthropic"
	"github.com/firebase/genkit/go/plugins/compat_oai/openai"
	"github.com/firebase/genkit/go/plugins/googlegenai"
	"github.com/firebase/genkit/go/plugins/mcp"
	"github.com/firebase/genkit/go/plugins/middleware"
	"github.com/ghchinoy/ax-images/internal/agent"
	"github.com/google/ax/pkg/apis/v1alpha1"
	"github.com/google/ax/runner"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"golang.org/x/net/http2"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

// dynamicHandler delegates HTTP requests to the inner handler set once
// initialization completes on workspace readiness.
type dynamicHandler struct {
	mu      sync.RWMutex
	handler http.Handler
}

func (d *dynamicHandler) Set(h http.Handler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handler = h
}

func (d *dynamicHandler) Ready() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.handler != nil
}

func (d *dynamicHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	d.mu.RLock()
	h := d.handler
	d.mu.RUnlock()
	if h == nil {
		http.Error(w, "agent initializing", http.StatusServiceUnavailable)
		return
	}
	h.ServeHTTP(w, r)
}

func taskHasAgentConfig(task *v1alpha1.Task) bool {
	spec := task.GetSpec()
	return spec != nil && len(spec.GetCommands()) == 0 &&
		(len(spec.GetConfig().GetFields()) > 0 || strings.TrimSpace(spec.GetGoal().GetText()) != "")
}

type pushedAssignment struct {
	task *v1alpha1.Task
	ws   *v1alpha1.Workspace
}

// runGenkitRunner executes the AX task runner alongside the in-process Genkit
// HTTP + A2A gRPC/JSON-RPC server on cfg.Port when spec.commands is empty and
// spec.config or spec.goal.text is set (supporting both static Config.Task and
// #688 Config.AwaitPush). When spec.commands is populated in static mode, it
// delegates directly to runner.Run.
func runGenkitRunner(ctx context.Context, cfg runner.Config, extraPlugins ...api.Plugin) error {
	if !cfg.AwaitPush {
		if !taskHasAgentConfig(cfg.Task) {
			return runner.Run(ctx, cfg)
		}
		wsPath := cfg.Task.GetSpec().WorkspacePath()
		if _, err := parseTaskAgentSpec(cfg.Task, cfg.Workspace, wsPath); err != nil {
			return err
		}
	}

	publicPort := cfg.Port
	if publicPort <= 0 {
		publicPort = runner.DefaultPort
	}
	internalPort, err := pickFreeLoopbackPort()
	if err != nil {
		return fmt.Errorf("allocating internal runner port: %w", err)
	}

	var agentTask atomic.Bool
	pushedCh := make(chan pushedAssignment, 1)

	innerCfg := cfg
	innerCfg.Port = internalPort
	if !cfg.AwaitPush {
		agentTask.Store(true)
		// Strip spec.goal on the inner runner task so runner.Run completes
		// SetupWorkspace without also spawning /usr/local/bin/ax-launcher while
		// genkit-runner serves the agent in-process.
		if innerCfg.Task != nil && innerCfg.Task.GetSpec().GetGoal() != nil {
			cloned := proto.Clone(innerCfg.Task).(*v1alpha1.Task)
			cloned.Spec.Goal = nil
			innerCfg.Task = cloned
		}
	} else {
		innerCfg.PrepareTask = func(t *v1alpha1.Task, w *v1alpha1.Workspace) (*v1alpha1.Task, error) {
			if !taskHasAgentConfig(t) {
				return t, nil
			}
			wsPath := t.GetSpec().WorkspacePath()
			if _, err := parseTaskAgentSpec(t, w, wsPath); err != nil {
				return nil, err
			}
			agentTask.Store(true)
			orig := proto.Clone(t).(*v1alpha1.Task)
			select {
			case pushedCh <- pushedAssignment{task: orig, ws: w}:
			default:
			}
			// Clear spec.goal on the copy handed to runner.Run so the inner
			// runner doesn't double-launch /usr/local/bin/ax-launcher.
			stripped := proto.Clone(t).(*v1alpha1.Task)
			if stripped.Spec != nil {
				stripped.Spec.Goal = nil
			}
			return stripped, nil
		}
	}

	runnerURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", internalPort))
	http1Proxy := httputil.NewSingleHostReverseProxy(runnerURL)
	h2cProxy := httputil.NewSingleHostReverseProxy(runnerURL)
	h2cProxy.Transport = &http2.Transport{
		AllowHTTP: true,
		DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
	}

	dyn := &dynamicHandler{}
	mux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 2 && strings.HasPrefix(r.Header.Get("Content-Type"), "application/grpc") {
			if strings.HasPrefix(r.URL.Path, "/lf.a2a.v1.A2AService/") || strings.HasPrefix(r.URL.Path, "/a2a.v1.A2AService/") {
				dyn.ServeHTTP(w, r)
				return
			}
			h2cProxy.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/runner/v1alpha1/") {
			http1Proxy.ServeHTTP(w, r)
			return
		}
		switch r.URL.Path {
		case "/healthz":
			http1Proxy.ServeHTTP(w, r)
		case "/readyz":
			// In AwaitPush mode, plain /readyz must return 200 immediately for
			// Substrate's WakeupProbe; only gate /readyz?check=workspace (or
			// static-mode /readyz) on Genkit agent initialization.
			checkWS := r.URL.Query().Get("check") == "workspace"
			if (!cfg.AwaitPush || (checkWS && agentTask.Load())) && !dyn.Ready() {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"status":"initializing_agent"}`))
				return
			}
			http1Proxy.ServeHTTP(w, r)
		default:
			dyn.ServeHTTP(w, r)
		}
	})

	pubSrv := &http.Server{
		Addr:    fmt.Sprintf(":%d", publicPort),
		Handler: mux,
	}
	pubSrv.Protocols = new(http.Protocols)
	pubSrv.Protocols.SetHTTP1(true)
	pubSrv.Protocols.SetUnencryptedHTTP2(true)

	ln, err := net.Listen("tcp", pubSrv.Addr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", pubSrv.Addr, err)
	}
	go func() {
		if err := pubSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("genkit public server error", "error", err)
		}
	}()

	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()

	var (
		cleanupMu sync.Mutex
		cleanup   func()
	)
	defer func() {
		_ = pubSrv.Shutdown(context.Background())
		cleanupMu.Lock()
		c := cleanup
		cleanupMu.Unlock()
		if c != nil {
			c()
		}
	}()

	runnerErrCh := make(chan error, 1)
	go func() {
		runnerErrCh <- runner.Run(runCtx, innerCfg)
	}()

	// Wait for runner.Run to receive the task (in AwaitPush mode) and finish
	// SetupWorkspace, then initialize the Genkit agent handler before flipping
	// /readyz?check=workspace to 200.
	initErrCh := make(chan error, 1)
	go func() {
		activeTask := cfg.Task
		activeWS := cfg.Workspace
		if cfg.AwaitPush {
			select {
			case <-runCtx.Done():
				initErrCh <- nil
				return
			case pushed := <-pushedCh:
				activeTask = pushed.task
				activeWS = pushed.ws
			}
		}
		readyURL := fmt.Sprintf("http://127.0.0.1:%d/readyz", internalPort)
		if cfg.AwaitPush {
			readyURL += "?check=workspace"
		}
		client := &http.Client{Timeout: time.Second}
		for {
			if runCtx.Err() != nil {
				initErrCh <- nil
				return
			}
			resp, err := client.Get(readyURL)
			if err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					break
				}
			}
			time.Sleep(25 * time.Millisecond)
		}
		wsPath := activeTask.GetSpec().WorkspacePath()
		h, c, err := buildAgentHandler(runCtx, activeTask, activeWS, wsPath, extraPlugins...)
		if err != nil {
			initErrCh <- err
			return
		}
		cleanupMu.Lock()
		cleanup = c
		cleanupMu.Unlock()
		dyn.Set(h)
		initErrCh <- nil
	}()

	select {
	case err := <-runnerErrCh:
		return err
	case err := <-initErrCh:
		if err != nil {
			cancelRun()
			<-runnerErrCh
			return err
		}
	}
	return <-runnerErrCh
}

// runGenkitLauncher handles --runner-volume-image mode (google/ax#621/#734),
// where /opt/ax/ax-runner owns port 80 and invokes /usr/local/bin/ax-launcher
// with AX_GOAL_FILE, AX_CONFIG_FILE, AX_MCP_CONFIG_FILE, AX_SESSION_FILE, and
// AX_OUTPUT_FILE.
func runGenkitLauncher(ctx context.Context) error {
	acfg, err := agent.Load()
	if err != nil {
		return fmt.Errorf("loading launcher config: %w", err)
	}
	prompt, err := acfg.OptionalPrompt()
	if err != nil {
		return err
	}
	if prompt == "" {
		return nil
	}
	cfgStruct, err := structpb.NewStruct(acfg.Values)
	if err != nil {
		return fmt.Errorf("encoding launcher config: %w", err)
	}
	var mcpServers []*v1alpha1.MCPServer
	for _, m := range acfg.MCP {
		mcpServers = append(mcpServers, &v1alpha1.MCPServer{
			Name:     m.Name,
			Endpoint: m.URL,
			Command:  m.Command,
			Args:     m.Args,
		})
	}
	task := &v1alpha1.Task{
		Spec: &v1alpha1.TaskSpec{
			Goal:      &v1alpha1.Goal{Text: acfg.Goal},
			SessionId: acfg.SessionID,
			Config:    cfgStruct,
			Workspace: &v1alpha1.WorkspaceRef{Path: acfg.Workspace},
		},
	}
	ws := &v1alpha1.Workspace{
		Spec: &v1alpha1.WorkspaceSpec{
			Mcp: &v1alpha1.MCPConfig{Servers: mcpServers},
		},
	}
	spec, err := parseTaskAgentSpec(task, ws, acfg.Workspace)
	if err != nil {
		return err
	}
	promptDir := filepath.Join(acfg.Workspace, ".genkit", "prompts")
	if err := os.MkdirAll(promptDir, 0o755); err != nil {
		return fmt.Errorf("creating prompt directory: %w", err)
	}
	plugins := defaultPlugins(spec.Model)
	var mcpClients []*mcp.GenkitMCPClient
	for _, srv := range spec.MCPServers {
		client, err := mcp.NewGenkitMCPClient(srv.Config)
		if err != nil {
			slog.Warn("failed to connect to workspace MCP server", "server", srv.Name, "error", err)
			continue
		}
		mcpClients = append(mcpClients, client)
	}
	defer func() {
		for _, c := range mcpClients {
			_ = c.Disconnect()
		}
	}()
	g := genkit.Init(ctx,
		genkit.WithExperimental(),
		genkit.WithPlugins(plugins...),
	)
	var mcpToolNames []string
	for _, c := range mcpClients {
		tools, err := c.GetActiveTools(ctx, g)
		if err != nil {
			slog.Warn("failed to fetch tools from MCP server", "error", err)
			continue
		}
		for _, t := range tools {
			if genkit.LookupTool(g, t.Name()) == nil {
				if reg, ok := t.(api.Registerable); ok {
					genkit.RegisterAction(g, reg)
				}
			}
			mcpToolNames = append(mcpToolNames, t.Name())
		}
	}
	if err := spec.writePromptFile(promptDir, acfg.Workspace, mcpToolNames); err != nil {
		return err
	}
	genkit.LoadPromptDir(g, promptDir, "")
	storeDir := filepath.Join(acfg.Workspace, ".genkit", "snapshots")
	store, err := localstore.NewFileSessionStore[any](storeDir)
	if err != nil {
		return fmt.Errorf("creating session store: %w", err)
	}
	var opts []aix.PromptAgentOption[any]
	opts = append(opts, aix.WithSessionStore(store))
	if spec.Description != "" {
		opts = append(opts, aix.WithDescription[any](spec.Description))
	}
	ag := genkitx.DefinePromptAgent(g, spec.Name, opts...)
	sessionID := firstNonEmpty(acfg.SessionID, "default")
	out, err := ag.RunText(ctx, prompt, aix.WithSessionID[any](sessionID))
	if err != nil {
		return fmt.Errorf("genkit launcher turn: %w", err)
	}
	var reply string
	if out.Message != nil {
		reply = out.Message.Text()
	}
	if reply != "" {
		fmt.Println(reply)
	}
	if err := acfg.WriteOutput(reply); err != nil {
		slog.Warn("failed to write AX_OUTPUT_FILE", "path", acfg.OutputFile, "err", err)
	}
	return nil
}

func pickFreeLoopbackPort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

// buildAgentHandler initializes Genkit, connects any Workspace MCP servers,
// materializes the Dotprompt file from spec, defines the store-backed Genkit
// agent under wsPath/.genkit/snapshots, and mounts Genkit's native HTTP
// routes (/agents/<name>...), the A2A JSON-RPC server (/ and
// /.well-known/agent-card.json), and the A2A gRPC server
// (lf.a2a.v1.A2AService and a2a.v1.A2AService).
func buildAgentHandler(ctx context.Context, task *v1alpha1.Task, ws *v1alpha1.Workspace, wsPath string, extraPlugins ...api.Plugin) (http.Handler, func(), error) {
	spec, err := parseTaskAgentSpec(task, ws, wsPath)
	if err != nil {
		return nil, nil, err
	}

	promptDir := filepath.Join(wsPath, ".genkit", "prompts")
	if err := os.MkdirAll(promptDir, 0o755); err != nil {
		return nil, nil, fmt.Errorf("creating prompt directory: %w", err)
	}

	tracesPath := filepath.Join(wsPath, ".genkit", "traces.jsonl")
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(&genkitSpanExporter{
			agentName:  spec.Name,
			tracesPath: tracesPath,
		}),
	)
	otel.SetTracerProvider(tp)

	plugins := defaultPlugins(spec.Model)
	plugins = append(plugins, extraPlugins...)

	var mcpCleanup func()
	var mcpClients []*mcp.GenkitMCPClient
	for _, srv := range spec.MCPServers {
		client, err := mcp.NewGenkitMCPClient(srv.Config)
		if err != nil {
			slog.Warn("failed to connect to workspace MCP server", "server", srv.Name, "error", err)
			continue
		}
		mcpClients = append(mcpClients, client)
	}
	mcpCleanup = func() {
		_ = tp.Shutdown(context.Background())
		for _, c := range mcpClients {
			_ = c.Disconnect()
		}
	}

	g := genkit.Init(ctx,
		genkit.WithExperimental(),
		genkit.WithPlugins(plugins...),
	)

	// Register active tools from any connected MCP servers on g before writing
	// and loading the prompt file so discovered MCP tools are included in
	// frontmatter with a single genkit.Init call.
	var mcpToolNames []string
	for _, c := range mcpClients {
		tools, err := c.GetActiveTools(ctx, g)
		if err != nil {
			slog.Warn("failed to fetch tools from MCP server", "error", err)
			continue
		}
		for _, t := range tools {
			// In Genkit v1.14.0, GetActiveTools returns MCP tools without
			// registering them in g's action registry. Guarding with LookupTool
			// registers them on v1.14.0 and makes this block a safe no-op once
			// open PR https://github.com/firebase/genkit/pull/6988 lands.
			if genkit.LookupTool(g, t.Name()) == nil {
				if reg, ok := t.(api.Registerable); ok {
					genkit.RegisterAction(g, reg)
				}
			}
			mcpToolNames = append(mcpToolNames, t.Name())
		}
	}

	if err := spec.writePromptFile(promptDir, wsPath, mcpToolNames); err != nil {
		mcpCleanup()
		return nil, nil, err
	}
	genkit.LoadPromptDir(g, promptDir, "")

	storeDir := filepath.Join(wsPath, ".genkit", "snapshots")
	store, err := localstore.NewFileSessionStore[any](storeDir)
	if err != nil {
		mcpCleanup()
		return nil, nil, fmt.Errorf("creating session store: %w", err)
	}

	var opts []aix.PromptAgentOption[any]
	opts = append(opts, aix.WithSessionStore(store))
	if spec.Description != "" {
		opts = append(opts, aix.WithDescription[any](spec.Description))
	}
	ag := genkitx.DefinePromptAgent(g, spec.Name, opts...)

	mux := http.NewServeMux()
	for _, route := range genkitx.AllAgentRoutes(g) {
		mux.HandleFunc(route.Pattern(), route.Handler())
	}

	agentDesc := firstNonEmpty(spec.Description, fmt.Sprintf("Genkit agent %s", spec.Name))
	agentCard := &a2a.AgentCard{
		Name:               spec.Name,
		Description:        agentDesc,
		Version:            "1.0.0",
		DefaultInputModes:  []string{"text/plain"},
		DefaultOutputModes: []string{"text/plain"},
		SupportedInterfaces: []*a2a.AgentInterface{
			a2a.NewAgentInterface("localhost:80", a2a.TransportProtocolGRPC),
			a2a.NewAgentInterface("http://localhost:80/", a2a.TransportProtocolJSONRPC),
		},
		Capabilities: a2a.AgentCapabilities{
			Streaming:         true,
			ExtendedAgentCard: true,
		},
		Skills: []a2a.AgentSkill{{
			ID:          spec.Name,
			Name:        spec.Name,
			Description: agentDesc,
			Tags:        []string{"genkit", "ax"},
		}},
	}

	a2aHandler := a2asrv.NewHandler(&genkitA2AExecutor{agentName: spec.Name, agent: ag}, a2asrv.WithExtendedAgentCard(agentCard))
	mux.Handle("/", a2av0.NewJSONRPCHandler(a2aHandler))
	mux.Handle(a2asrv.WellKnownAgentCardPath, a2asrv.NewStaticAgentCardHandler(agentCard))

	a2aGRPCServer := grpc.NewServer()
	a2agrpcv1.NewHandler(a2aHandler).RegisterWith(a2aGRPCServer)
	a2agrpcv0.NewHandler(a2aHandler).RegisterWith(a2aGRPCServer)

	combined := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 2 && strings.HasPrefix(r.Header.Get("Content-Type"), "application/grpc") {
			a2aGRPCServer.ServeHTTP(w, r)
			return
		}
		mux.ServeHTTP(w, r)
	})

	slog.Info("initialized Genkit agent server",
		"agent", spec.Name,
		"model", spec.Model,
		"storeDir", storeDir,
		"mcpTools", len(mcpToolNames),
	)

	// If Task.spec.goal.spec (or spec.config.prompt) sets a boot goal and no
	// snapshot exists yet for this session, run Turn 1 asynchronously in the
	// persistent session store so it survives suspend/resume for Turn 2+.
	if spec.BootPrompt != "" {
		sessionID := firstNonEmpty(strings.TrimSpace(task.GetSpec().GetSessionId()), "default")
		if snap, err := ag.GetLatestSnapshot(ctx, sessionID, aix.WithMetadataOnly()); err != nil || snap == nil {
			go func() {
				slog.Info("running initial boot goal turn", "agent", spec.Name, "session_id", sessionID)
				out, err := ag.RunText(ctx, spec.BootPrompt, aix.WithSessionID[any](sessionID))
				if err != nil {
					slog.Error("initial boot goal turn failed", "agent", spec.Name, "session_id", sessionID, "error", err)
					return
				}
				var reply string
				if out.Message != nil {
					reply = out.Message.Text()
				}
				if outPath := filepath.Join(wsPath, ".ax", "output"); reply != "" {
					_ = os.MkdirAll(filepath.Dir(outPath), 0o755)
					_ = os.WriteFile(outPath, []byte(reply), 0o644)
				}
				slog.Info("initial boot goal turn completed", "agent", spec.Name, "session_id", sessionID, "finish_reason", string(out.FinishReason))
			}()
		}
	}

	return combined, mcpCleanup, nil
}

// defaultPlugins returns the built-in middleware plugin plus the model
// provider plugins whose credentials or model prefixes are active.
func defaultPlugins(modelName string) []api.Plugin {
	plugins := []api.Plugin{&middleware.Middleware{}}

	switch {
	case strings.HasPrefix(modelName, "googleai/"):
		if os.Getenv("GEMINI_API_KEY") != "" || os.Getenv("GOOGLE_API_KEY") != "" {
			plugins = append(plugins, &googlegenai.GoogleAI{})
		}
	case strings.HasPrefix(modelName, "vertexai/"):
		plugins = append(plugins, &googlegenai.VertexAI{
			ProjectID: firstNonEmpty(os.Getenv("GOOGLE_CLOUD_PROJECT"), os.Getenv("GCLOUD_PROJECT")),
			Location:  firstNonEmpty(os.Getenv("GOOGLE_CLOUD_LOCATION"), "global"),
		})
	case strings.HasPrefix(modelName, "anthropic/"):
		if os.Getenv("ANTHROPIC_API_KEY") != "" || os.Getenv("ANTHROPIC_AUTH_TOKEN") != "" {
			plugins = append(plugins, &anthropic.Anthropic{})
		}
	case strings.HasPrefix(modelName, "openai/"):
		if os.Getenv("OPENAI_API_KEY") != "" {
			plugins = append(plugins, &openai.OpenAI{})
		}
	default:
		if os.Getenv("GEMINI_API_KEY") != "" || os.Getenv("GOOGLE_API_KEY") != "" {
			plugins = append(plugins, &googlegenai.GoogleAI{})
		}
	}
	return plugins
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// genkitSpanRecord is persisted to /workspace/.genkit/traces.jsonl and logged
// via slog so Substrate worker logs and AX Central can reconstruct the full
// Genkit OpenTelemetry span tree for every agent turn, model call, and tool call.
type genkitSpanRecord struct {
	Agent         string  `json:"agent"`
	Span          string  `json:"span"`
	Type          string  `json:"type,omitempty"`
	Subtype       string  `json:"subtype,omitempty"`
	Status        string  `json:"status,omitempty"`
	TraceID       string  `json:"trace_id"`
	SpanID        string  `json:"span_id"`
	ParentSpanID  string  `json:"parent_span_id,omitempty"`
	StartUnixNano int64   `json:"start_unix_nano"`
	EndUnixNano   int64   `json:"end_unix_nano"`
	DurationMs    float64 `json:"duration_ms"`
}

type genkitSpanExporter struct {
	mu         sync.Mutex
	agentName  string
	tracesPath string
}

func (e *genkitSpanExporter) ExportSpans(_ context.Context, spans []sdktrace.ReadOnlySpan) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	var f *os.File
	if e.tracesPath != "" {
		f, _ = os.OpenFile(e.tracesPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if f != nil {
			defer f.Close()
		}
	}

	for _, s := range spans {
		var spanType, spanSubtype, spanState string
		for _, attr := range s.Attributes() {
			switch string(attr.Key) {
			case "genkit:type":
				spanType = attr.Value.AsString()
			case "genkit:subtype":
				spanSubtype = attr.Value.AsString()
			case "genkit:state":
				spanState = attr.Value.AsString()
			}
		}
		if spanState == "" {
			spanState = "success"
		}
		traceID := s.SpanContext().TraceID().String()
		spanID := s.SpanContext().SpanID().String()
		parentSpanID := ""
		if s.Parent().IsValid() {
			parentSpanID = s.Parent().SpanID().String()
		}
		startNano := s.StartTime().UnixNano()
		endNano := s.EndTime().UnixNano()
		durMs := float64(endNano-startNano) / 1e6

		rec := genkitSpanRecord{
			Agent:         e.agentName,
			Span:          s.Name(),
			Type:          spanType,
			Subtype:       spanSubtype,
			Status:        spanState,
			TraceID:       traceID,
			SpanID:        spanID,
			ParentSpanID:  parentSpanID,
			StartUnixNano: startNano,
			EndUnixNano:   endNano,
			DurationMs:    durMs,
		}
		if f != nil {
			if data, err := json.Marshal(rec); err == nil {
				_, _ = f.Write(append(data, '\n'))
			}
		}
		slog.Info("genkit otel span",
			"agent", e.agentName,
			"span", s.Name(),
			"type", spanType,
			"subtype", spanSubtype,
			"status", spanState,
			"duration_ms", fmt.Sprintf("%.2f", durMs),
			"start_unix_nano", startNano,
			"end_unix_nano", endNano,
			"trace_id", traceID,
			"span_id", spanID,
			"parent_span_id", parentSpanID,
		)
	}
	return nil
}

func (e *genkitSpanExporter) Shutdown(_ context.Context) error { return nil }

// genkitA2AExecutor adapts a Genkit *aix.Agent[any] to an A2A AgentExecutor.
type genkitA2AExecutor struct {
	agentName string
	agent     *aix.Agent[any]
}

func (e *genkitA2AExecutor) Execute(ctx context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		var texts []string
		if execCtx.Message != nil {
			for _, p := range execCtx.Message.Parts {
				if t, ok := p.Content.(a2a.Text); ok {
					texts = append(texts, string(t))
				}
			}
		}
		prompt := strings.Join(texts, "\n")

		var opts []aix.InvocationOption[any]
		if execCtx.ContextID != "" {
			opts = append(opts, aix.WithSessionID[any](execCtx.ContextID))
		}

		started := time.Now()
		slog.Info("A2A turn started",
			"agent", e.agentName,
			"context_id", execCtx.ContextID,
			"prompt_chars", len(prompt),
		)

		out, err := e.agent.RunText(ctx, prompt, opts...)
		if err != nil {
			slog.Error("A2A turn failed",
				"agent", e.agentName,
				"context_id", execCtx.ContextID,
				"error", err,
			)
			yield(nil, err)
			return
		}
		if out.FinishReason == aix.AgentFinishReasonFailed && out.Error != nil {
			err := fmt.Errorf("agent turn failed (%s): %s", out.Error.Status, out.Error.Message)
			slog.Error("A2A turn failed",
				"agent", e.agentName,
				"context_id", execCtx.ContextID,
				"error", err,
			)
			yield(nil, err)
			return
		}

		var replyText string
		if out.Message != nil {
			replyText = out.Message.Text()
		}
		reply := a2a.NewMessageForTask(a2a.MessageRoleAgent, execCtx, a2a.NewTextPart(replyText))
		if execCtx.ContextID != "" {
			reply.ContextID = execCtx.ContextID
		} else {
			reply.ContextID = out.SessionID
		}
		slog.Info("A2A turn completed",
			"agent", e.agentName,
			"context_id", reply.ContextID,
			"finish_reason", string(out.FinishReason),
			"duration_ms", fmt.Sprintf("%.2f", float64(time.Since(started).Nanoseconds())/1e6),
			"reply_chars", len(replyText),
		)
		yield(reply, nil)
	}
}

func (e *genkitA2AExecutor) Cancel(ctx context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		if execCtx.ContextID != "" && e.agent.Store() != nil {
			if snap, err := e.agent.GetLatestSnapshot(ctx, execCtx.ContextID, aix.WithMetadataOnly()); err == nil && snap != nil && snap.Status == aix.SnapshotStatusPending {
				_, _ = e.agent.Abort(ctx, snap.SnapshotID)
			}
		}
		yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateCanceled, nil), nil)
	}
}

// mockModelPlugin is a test helper plugin that registers a deterministic model
// on the Genkit instance during genkit.Init.
type mockModelPlugin struct {
	provider string
	model    string
	fn       func(ctx context.Context, req *ai.ModelRequest, cb ai.ModelStreamCallback) (*ai.ModelResponse, error)
}

func (p *mockModelPlugin) Name() string { return p.provider }

func (p *mockModelPlugin) Init(ctx context.Context) []api.Action {
	m := ai.NewModel(api.NewName(p.provider, p.model), &ai.ModelOptions{
		Supports: &ai.ModelSupports{Multiturn: true, SystemRole: true, Tools: true, Media: true},
	}, p.fn)
	return []api.Action{m.(api.Action)}
}
