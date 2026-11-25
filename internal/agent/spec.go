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

// Package agent holds what the agents in this repository share: the task's
// spec.goal and spec.config, its workspace, the Workspace's MCP servers, and a
// small set of local tools with optional DiffusionGemma (dgem) PreToolUse
// safety gating.
package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/google/ax/pkg/apis/v1alpha1"
	"github.com/google/ax/runner"
)

// Config is the part of the Task and Workspace an agent needs.
type Config struct {
	// TaskName is the task's metadata.name.
	TaskName string
	// Goal is spec.goal.text when set on the Task.
	Goal string
	// SessionID is spec.session_id when set on the Task.
	SessionID string
	// Values is spec.config.
	Values map[string]any
	// Workspace is the directory the agent works in.
	Workspace string
	// OutputFile is AX_OUTPUT_FILE when set by the runner launcher contract.
	OutputFile string
	// MCP lists the Workspace's MCP servers.
	MCP []MCPServer
}

// MCPServer is an MCP server declared on the Workspace. A server with a URL
// is reached over streamable HTTP; one with a Command runs over stdio inside
// the sandbox.
type MCPServer struct {
	Name    string
	URL     string
	Command string
	Args    []string
}

// Load reads task configuration from the AX launcher contract files
// (AX_GOAL_FILE, AX_CONFIG_FILE, AX_MCP_CONFIG_FILE, AX_SESSION_FILE,
// AX_OUTPUT_FILE — google/ax#621/#734) when present, falling back to
// AX_TASK_YAML and AX_WORKSPACE_YAML.
func Load() (*Config, error) {
	if hasLauncherEnv() {
		return loadFromLauncherFiles()
	}
	var task v1alpha1.Task
	if err := runner.LoadSpec("", "AX_TASK_YAML", &task); err != nil {
		return nil, err
	}
	var ws v1alpha1.Workspace
	if err := runner.LoadSpec("", "AX_WORKSPACE_YAML", &ws); err != nil {
		return nil, err
	}
	dir := strings.TrimSpace(os.Getenv("AX_WORKSPACE"))
	if dir == "" {
		dir = task.GetSpec().GetWorkspace().GetPath()
	}
	if dir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		dir = wd
	}
	return &Config{
		TaskName:   task.GetMetadata().GetName(),
		Goal:       strings.TrimSpace(task.GetSpec().GetGoal().GetText()),
		SessionID:  strings.TrimSpace(task.GetSpec().GetSessionId()),
		Values:     task.GetSpec().GetConfig().AsMap(),
		Workspace:  dir,
		OutputFile: strings.TrimSpace(os.Getenv(runner.OutputFileEnv)),
		MCP:        mcpServers(ws.GetSpec().GetMcp()),
	}, nil
}

func hasLauncherEnv() bool {
	return os.Getenv(runner.GoalFileEnv) != "" ||
		os.Getenv(runner.ConfigFileEnv) != "" ||
		os.Getenv(runner.MCPConfigFileEnv) != "" ||
		os.Getenv(runner.SessionFileEnv) != ""
}

func loadFromLauncherFiles() (*Config, error) {
	goal, err := readOptionalFile(os.Getenv(runner.GoalFileEnv))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", runner.GoalFileEnv, err)
	}
	sessionID, err := readOptionalFile(os.Getenv(runner.SessionFileEnv))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", runner.SessionFileEnv, err)
	}
	values := map[string]any{}
	if cfgText, err := readOptionalFile(os.Getenv(runner.ConfigFileEnv)); err != nil {
		return nil, fmt.Errorf("%s: %w", runner.ConfigFileEnv, err)
	} else if cfgText != "" {
		if err := json.Unmarshal([]byte(cfgText), &values); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", runner.ConfigFileEnv, err)
		}
	}
	var servers []MCPServer
	if mcpText, err := readOptionalFile(os.Getenv(runner.MCPConfigFileEnv)); err != nil {
		return nil, fmt.Errorf("%s: %w", runner.MCPConfigFileEnv, err)
	} else if mcpText != "" {
		servers, err = parseMCPConfigJSON([]byte(mcpText))
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", runner.MCPConfigFileEnv, err)
		}
	}
	dir := strings.TrimSpace(os.Getenv("AX_WORKSPACE"))
	if dir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		dir = wd
	}
	return &Config{
		Goal:       strings.TrimSpace(goal),
		SessionID:  strings.TrimSpace(sessionID),
		Values:     values,
		Workspace:  dir,
		OutputFile: strings.TrimSpace(os.Getenv(runner.OutputFileEnv)),
		MCP:        servers,
	}, nil
}

func readOptionalFile(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func parseMCPConfigJSON(data []byte) ([]MCPServer, error) {
	var raw struct {
		MCPServers map[string]struct {
			URL     string   `json:"url"`
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(raw.MCPServers))
	for k := range raw.MCPServers {
		names = append(names, k)
	}
	slices.Sort(names)
	var out []MCPServer
	for _, name := range names {
		s := raw.MCPServers[name]
		if s.URL == "" && s.Command == "" {
			continue
		}
		out = append(out, MCPServer{
			Name:    name,
			URL:     s.URL,
			Command: s.Command,
			Args:    s.Args,
		})
	}
	return out, nil
}

// WriteOutput writes the agent's final response to AX_OUTPUT_FILE when set.
func (c *Config) WriteOutput(text string) error {
	if c == nil || c.OutputFile == "" || strings.TrimSpace(text) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(c.OutputFile), 0o755); err != nil {
		return err
	}
	return os.WriteFile(c.OutputFile, []byte(strings.TrimSpace(text)+"\n"), 0o644)
}

// OptionalPrompt returns spec.goal.text when set, falling back to
// spec.config.prompt (or "" when neither is set).
func (c *Config) OptionalPrompt() (string, error) {
	if c.Goal != "" {
		return c.Goal, nil
	}
	s, err := c.String("prompt")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(s), nil
}

// Prompt returns the required task prompt from spec.goal.text or
// spec.config.prompt.
func (c *Config) Prompt() (string, error) {
	s, err := c.OptionalPrompt()
	if err != nil {
		return "", err
	}
	if s == "" {
		return "", errors.New("spec.goal.text or spec.config.prompt: required")
	}
	return s, nil
}

func mcpServers(mcp *v1alpha1.MCPConfig) []MCPServer {
	all := slices.Clone(mcp.GetServers())
	for _, r := range mcp.GetRegistries() {
		all = append(all, r.GetServers()...)
	}
	var servers []MCPServer
	seen := map[string]bool{}
	for _, srv := range all {
		name := srv.GetName()
		if name == "" || seen[name] {
			slog.Warn("skipping MCP server with a missing or duplicate name", "name", name)
			continue
		}
		switch {
		case srv.GetEndpoint() != "":
			servers = append(servers, MCPServer{Name: name, URL: srv.GetEndpoint()})
		case srv.GetCommand() != "":
			servers = append(servers, MCPServer{Name: name, Command: srv.GetCommand(), Args: srv.GetArgs()})
		default:
			slog.Warn("skipping MCP server with neither an endpoint nor a command", "name", name)
			continue
		}
		seen[name] = true
	}
	return servers
}

// String returns the string at key, or "" when it is unset.
func (c *Config) String(key string) (string, error) {
	v, ok := c.Values[key]
	if !ok {
		return "", nil
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("spec.config.%s: must be a string", key)
	}
	return s, nil
}

// Required returns the non-empty string at key.
func (c *Config) Required(key string) (string, error) {
	s, err := c.String(key)
	if err == nil && s == "" {
		err = fmt.Errorf("spec.config.%s: required", key)
	}
	return s, err
}

// Int returns the integer at key, or def when it is unset.
func (c *Config) Int(key string, def int) (int, error) {
	v, ok := c.Values[key]
	if !ok {
		return def, nil
	}
	f, ok := v.(float64)
	if !ok || f != float64(int(f)) {
		return 0, fmt.Errorf("spec.config.%s: must be an integer", key)
	}
	return int(f), nil
}

// Bool returns the boolean at key, or false when it is unset.
func (c *Config) Bool(key string) (bool, error) {
	v, ok := c.Values[key]
	if !ok {
		return false, nil
	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("spec.config.%s: must be a boolean", key)
	}
	return b, nil
}

// Strings returns the list of strings at key, or nil when it is unset.
func (c *Config) Strings(key string) ([]string, error) {
	v, ok := c.Values[key]
	if !ok {
		return nil, nil
	}
	list, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("spec.config.%s: must be a list of strings", key)
	}
	out := make([]string, 0, len(list))
	for _, e := range list {
		s, ok := e.(string)
		if !ok {
			return nil, fmt.Errorf("spec.config.%s: must be a list of strings", key)
		}
		out = append(out, s)
	}
	return out, nil
}

// Object returns the object at key, or nil when it is unset.
func (c *Config) Object(key string) (map[string]any, error) {
	v, ok := c.Values[key]
	if !ok {
		return nil, nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("spec.config.%s: must be an object", key)
	}
	return m, nil
}
