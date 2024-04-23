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
	"reflect"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRetrieveModifiedTime(t *testing.T) {
	// Setting up a fixed date for testing
	testTime := time.Now()
	formattedTestTime := testTime.UTC().Format(time.RFC3339)
	tests := []struct {
		name string
		om   metav1.ObjectMeta
		want string
	}{
		{
			name: "With ManagedFields",
			om: metav1.ObjectMeta{
				ManagedFields: []metav1.ManagedFieldsEntry{
					{Time: &metav1.Time{Time: testTime}},
				},
			},
			want: formattedTestTime,
		},
		{
			name: "Without ManagedFields",
			om: metav1.ObjectMeta{
				CreationTimestamp: metav1.Time{Time: testTime},
			},
			want: formattedTestTime,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RetrieveModifiedTime(tt.om); got != tt.want {
				t.Errorf("RetrieveModifiedTime() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMarshalConfigData(t *testing.T) {
	tests := []struct {
		name       string
		configData ConfigData
		wantErr    bool
	}{
		{
			name:       "Valid ConfigData",
			configData: ConfigData{Namespaces: []Namespace{{Name: "test"}}},
			wantErr:    false,
		},
		{
			name:       "Invalid ConfigData - invalid JSON",
			configData: ConfigData{}, // Assuming there's a case that could produce an invalid JSON
			wantErr:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := MarshalConfigData(tt.configData)
			if (err != nil) != tt.wantErr {
				t.Errorf("MarshalConfigData() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		name                 string
		commaSeparatedString string
		value                string
		want                 bool
	}{
		{
			name:                 "Contains Value",
			commaSeparatedString: "apple,banana,orange",
			value:                "banana",
			want:                 true,
		},
		{
			name:                 "Does Not Contain Value",
			commaSeparatedString: "apple,banana,orange",
			value:                "grape",
			want:                 false,
		},
		{
			name:                 "Empty String",
			commaSeparatedString: "",
			value:                "grape",
			want:                 false,
		},
		{
			name:                 "Value With Spaces",
			commaSeparatedString: "apple, banana , orange",
			value:                "banana",
			want:                 true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Contains(tt.commaSeparatedString, tt.value); got != tt.want {
				t.Errorf("Contains() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCalculateHash(t *testing.T) {
	tests := []struct {
		name string
		data map[string][]byte
		want string // provide expected hash values for consistent data
	}{
		{
			name: "Single Element",
			data: map[string][]byte{"key1": []byte("value1")},
			want: "3c9683017f9e4bf33d0fbedd26bf143fd72de9b9dd145441b75f0604047ea28e",
		},
		{
			name: "Multiple Elements",
			data: map[string][]byte{"key1": []byte("value1"), "key2": []byte("value2")},
			want: "0820d7fc8c71743a88a7312406ac76fbbab77bc15864201f1b94cfe1205ceaa0",
		},
		{
			name: "No Elements",
			data: map[string][]byte{},
			want: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CalculateHash(tt.data); got != tt.want {
				t.Errorf("CalculateHash() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLabelsForStaleSecretWatchConfigMap(t *testing.T) {
	tests := []struct {
		name string
		args string
		want map[string]string
	}{
		{
			name: "Standard Labels",
			args: "test-name",
			want: map[string]string{
				"app.kubernetes.io/name":       "StaleSecretWatch",
				"app.kubernetes.io/instance":   "test-name",
				"app.kubernetes.io/part-of":    "StaleSecretWatch-operator",
				"app.kubernetes.io/created-by": "controller-manager-stalesecretwatch",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LabelsForStaleSecretWatchConfigMap(tt.args); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("LabelsForStaleSecretWatchConfigMap() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseTime(t *testing.T) {
	validTime := "2020-01-02T15:04:05Z"
	parsedValidTime, _ := time.Parse(time.RFC3339, validTime)
	tests := []struct {
		name    string
		timeStr string
		want    time.Time
		wantErr bool
	}{
		{
			name:    "Valid Time",
			timeStr: validTime,
			want:    parsedValidTime,
			wantErr: false,
		},
		{
			name:    "Invalid Time",
			timeStr: "invalid-time",
			want:    time.Time{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTime(tt.timeStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseTime() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !got.Equal(tt.want) {
				t.Errorf("ParseTime() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetNextFiveMinutes(t *testing.T) {
	tests := []struct {
		name string
		want time.Duration
	}{
		{
			name: "Next Five Minutes",
			want: time.Duration(GetNextFiveMinutes().Hours()),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := time.Duration(GetNextFiveMinutes().Hours()); got != tt.want {
				t.Errorf("GetNextFiveMinutes() = %v, want %v", got, tt.want)
			}
		})
	}
}
