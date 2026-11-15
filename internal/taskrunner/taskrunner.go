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

// Package taskrunner is the task runner shared by every image in this
// repository. Each image ships a single agent binary at /usr/local/bin/ax-agent
// with /usr/local/bin/ax-task-runner hard-linked to it; the agent calls Main
// when it was started under the runner's name.
package taskrunner

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
	"google.golang.org/protobuf/proto"
)

// defaultAgent is where every image installs its agent entrypoint.
const defaultAgent = "/usr/local/bin/ax-agent"

// Name is the binary name AX starts the runner under.
const Name = "ax-task-runner"

// Invoked reports whether the process was started as the runner rather than
// as the agent.
func Invoked() bool {
	return filepath.Base(os.Args[0]) == Name
}

// Main runs the task runner and exits the process on failure.
func Main() {
	var (
		cfg      runner.Config
		taskFile string
		wsFile   string
		agent    string
	)
	flag := flag.NewFlagSet(Name, flag.ExitOnError)
	flag.IntVar(&cfg.Port, "port", runner.DefaultPort, "Port for the probe and guest server")
	flag.IntVar(&cfg.MetadataPort, "metadata-port", runner.DefaultMetadataPort, "Loopback-only port for the metadata endpoints")
	flag.StringVar(&taskFile, "task-file", "", "Read the Task YAML from this file instead of AX_TASK_YAML")
	flag.StringVar(&wsFile, "workspace-file", "", "Read the Workspace YAML from this file instead of AX_WORKSPACE_YAML")
	flag.StringVar(&agent, "agent", defaultAgent, "Command to run when the task leaves spec.commands empty")
	flag.Parse(os.Args[1:])

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

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
		cfg.Task = withAgentCommand(&task, agent)
	}

	exportSpec("AX_TASK_YAML", taskFile)
	exportSpec("AX_WORKSPACE_YAML", wsFile)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := runner.Run(ctx, cfg); err != nil {
		fatal(err)
	}
}

func withAgentCommand(task *v1alpha1.Task, agent string) *v1alpha1.Task {
	if len(task.GetSpec().GetCommands()) > 0 || agent == "" {
		return task
	}
	task = proto.Clone(task).(*v1alpha1.Task)
	if task.Spec == nil {
		task.Spec = &v1alpha1.TaskSpec{}
	}
	task.Spec.Commands = []string{agent}
	return task
}

func exportSpec(envVar, file string) {
	if file == "" || os.Getenv(envVar) != "" {
		return
	}
	data, err := os.ReadFile(file)
	if err != nil {
		fatal(err)
	}
	os.Setenv(envVar, string(data))
}

func fatal(err error) {
	slog.Error("agent runner failed", "error", err)
	os.Exit(1)
}
