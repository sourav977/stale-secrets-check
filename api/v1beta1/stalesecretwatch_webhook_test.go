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

package v1beta1

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestStaleSecretWatch_Default(t *testing.T) {
	tests := []struct {
		name    string
		staleSW *StaleSecretWatch
		want    string
	}{
		{
			name: "Empty Namespace",
			staleSW: &StaleSecretWatch{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-ssw",
				},
			},
			want: "default",
		},
		{
			name: "Non-Empty Namespace",
			staleSW: &StaleSecretWatch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ssw",
					Namespace: "custom",
				},
			},
			want: "custom",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.staleSW.Default()
			if tt.staleSW.Namespace != tt.want {
				t.Errorf("Default() = %v, want %v", tt.staleSW.Namespace, tt.want)
			}
		})
	}
}

func TestStaleSecretWatch_ValidateCreate(t *testing.T) {
	tests := []struct {
		name    string
		staleSW *StaleSecretWatch
		wantErr bool
	}{
		{
			name: "Valid Data",
			staleSW: &StaleSecretWatch{
				Spec: StaleSecretWatchSpec{
					StaleThresholdInDays: 3,
					StaleSecretToWatch: StaleSecretToWatch{
						Namespace: "default",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Invalid Data - Empty Namespace",
			staleSW: &StaleSecretWatch{
				Spec: StaleSecretWatchSpec{
					StaleThresholdInDays: 3,
					StaleSecretToWatch: StaleSecretToWatch{
						Namespace: "",
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.staleSW.ValidateCreate()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCreate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestStaleSecretWatch_ValidateUpdate(t *testing.T) {
	tests := []struct {
		name    string
		staleSW *StaleSecretWatch
		old     runtime.Object
		wantErr bool
	}{
		{
			name: "Valid Update",
			staleSW: &StaleSecretWatch{
				Spec: StaleSecretWatchSpec{
					StaleThresholdInDays: 5,
					StaleSecretToWatch: StaleSecretToWatch{
						Namespace: "default",
					},
				},
			},
			old:     &StaleSecretWatch{},
			wantErr: false,
		},
		{
			name: "Invalid Update - Negative Days",
			staleSW: &StaleSecretWatch{
				Spec: StaleSecretWatchSpec{
					StaleThresholdInDays: -1,
					StaleSecretToWatch: StaleSecretToWatch{
						Namespace: "default",
					},
				},
			},
			old:     &StaleSecretWatch{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.staleSW.ValidateUpdate(tt.old)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateUpdate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestStaleSecretWatch_ValidateDelete(t *testing.T) {
	tests := []struct {
		name    string
		staleSW *StaleSecretWatch
		wantErr bool
	}{
		{
			name:    "Validate Delete - Always Pass",
			staleSW: &StaleSecretWatch{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.staleSW.ValidateDelete()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDelete() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestStaleSecretWatch_ValidateStaleSecretWatch(t *testing.T) {
	tests := []struct {
		name    string
		staleSW *StaleSecretWatch
		wantErr bool
	}{
		{
			name: "Valid StaleSecretWatch",
			staleSW: &StaleSecretWatch{
				Spec: StaleSecretWatchSpec{
					StaleThresholdInDays: 1,
					StaleSecretToWatch: StaleSecretToWatch{
						Namespace: "default",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Invalid StaleSecretWatch - Empty Namespace",
			staleSW: &StaleSecretWatch{
				Spec: StaleSecretWatchSpec{
					StaleThresholdInDays: 1,
					StaleSecretToWatch: StaleSecretToWatch{
						Namespace: "",
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.staleSW.ValidateStaleSecretWatch(); (err != nil) != tt.wantErr {
				t.Errorf("ValidateStaleSecretWatch() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
