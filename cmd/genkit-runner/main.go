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

// Command genkit-runner is a task runner that serves a Genkit Go agent
// configured from the Task's spec.config (Dotprompt frontmatter + instruction),
// spec.goal.text, or a .prompt file in the workspace. When invoked as
// ax-launcher or ax-harness (google/ax#621/#734 --runner-volume-image mode),
// it executes a single Genkit turn from AX_GOAL_FILE/AX_CONFIG_FILE and writes
// the reply to AX_OUTPUT_FILE.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/google/ax/pkg/apis/v1alpha1"
	"github.com/google/ax/runner"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	base := filepath.Base(os.Args[0])
	if base == "ax-launcher" || base == "ax-harness" {
		if err := runGenkitLauncher(ctx); err != nil {
			fatal(err)
		}
		return
	}

	var (
		cfg      runner.Config
		taskFile string
		wsFile   string
	)
	flag.IntVar(&cfg.Port, "port", runner.DefaultPort, "Port for the probe, agent, and guest server")
	flag.IntVar(&cfg.MetadataPort, "metadata-port", runner.DefaultMetadataPort, "Loopback-only port for the metadata endpoints")
	flag.StringVar(&taskFile, "task-file", "", "Read the Task YAML from this file instead of AX_TASK_YAML")
	flag.StringVar(&wsFile, "workspace-file", "", "Read the Workspace YAML from this file instead of AX_WORKSPACE_YAML")
	flag.Parse()

	var ws v1alpha1.Workspace
	if err := runner.LoadSpec(wsFile, "AX_WORKSPACE_YAML", &ws); err != nil {
		fatal(err)
	} else if ws.GetMetadata() != nil {
		cfg.Workspace = &ws
	}

	var task v1alpha1.Task
	if err := runner.LoadSpec(taskFile, "AX_TASK_YAML", &task); err != nil {
		fatal(err)
	} else if task.GetMetadata() != nil {
		cfg.Task = &task
	}

	// Under google/ax#688, actors from shared Profile templates boot without
	// AX_TASK_YAML and receive their Task via POST /runner/v1alpha1/task.
	if cfg.Task == nil {
		cfg.AwaitPush = true
	}

	if err := runGenkitRunner(ctx, cfg); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	slog.Error("genkit runner failed", "error", err)
	os.Exit(1)
}
