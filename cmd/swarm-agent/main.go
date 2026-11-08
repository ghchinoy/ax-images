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

// Command swarm-agent acts as both the AX task runner (when invoked as
// ax-task-runner) and the multi-agent Gemini worker entrypoint (which delegates
// to /usr/local/bin/agent.py when spec.config.prompt or AGENT_PROMPT is set).
package main

import (
	"os"
	"os/exec"

	"github.com/ghchinoy/ax-images/internal/taskrunner"
)

func main() {
	if taskrunner.Invoked() {
		taskrunner.Main()
		return
	}
	// If neither AGENT_PROMPT nor AX_RUN_ON_BOOT is set, exit 0 so interactive
	// `ax ssh` swarm recipes (01..08) can invoke /usr/local/bin/agent.py on demand.
	if os.Getenv("AGENT_PROMPT") == "" && os.Getenv("AX_RUN_ON_BOOT") == "" {
		return
	}
	cmd := exec.Command("python3", append([]string{"/usr/local/bin/agent.py"}, os.Args[1:]...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}
}
