/*
Copyright 2024 Sourav Patnaik.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"os"
	"testing"
)

func TestGetClusterName(t *testing.T) {
	os.Setenv("KUBECONFIG", "/invalid/path")
	tests := []struct {
		name    string
		setup   func()
		cleanup func()
		want    string
	}{
		{
			name:    "Default Cluster Name When KUBECONFIG is Invalid",
			setup:   func() { os.Setenv("KUBECONFIG", "/invalid/path") },
			cleanup: func() { os.Unsetenv("KUBECONFIG") },
			want:    "cluster",
		},
		{
			name: "Extract Cluster Name From KUBECONFIG",
			setup: func() {
				// Mimic environment where a valid kubeconfig is provided
				os.Setenv("KUBECONFIG", "/tmp/kube_config.bkp")
			},
			cleanup: func() { os.Unsetenv("KUBECONFIG") },
			want:    "cluster",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			if got := GetClusterName(); got != tt.want {
				t.Errorf("GetClusterName() = %v, want %v", got, tt.want)
			}
			tt.cleanup()
		})
	}
}
