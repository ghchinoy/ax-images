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

package taskrunner

import (
	"slices"
	"testing"

	"github.com/google/ax/pkg/apis/v1alpha1"
)

func TestWithAgentCommand(t *testing.T) {
	empty := &v1alpha1.Task{}
	got := withAgentCommand(empty, "/usr/local/bin/ax-agent")
	want := []string{"/usr/local/bin/ax-agent"}
	if !slices.Equal(got.GetSpec().GetCommands(), want) {
		t.Errorf("withAgentCommand(empty) = %v, want %v", got.GetSpec().GetCommands(), want)
	}

	explicit := &v1alpha1.Task{
		Spec: &v1alpha1.TaskSpec{
			Commands: []string{"echo", "hi"},
		},
	}
	gotExplicit := withAgentCommand(explicit, "/usr/local/bin/ax-agent")
	if !slices.Equal(gotExplicit.GetSpec().GetCommands(), []string{"echo", "hi"}) {
		t.Errorf("withAgentCommand(explicit) = %v, want [echo hi]", gotExplicit.GetSpec().GetCommands())
	}
}
