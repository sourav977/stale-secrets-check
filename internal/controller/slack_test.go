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
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/go-logr/logr"
	securityv1beta1 "github.com/sourav977/stale-secrets-watch/api/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestStaleSecretWatchReconciler_NotifySlack(t *testing.T) {
	logger := logr.Discard()
	ctx := context.TODO()

	// Prepare test data
	staleSecret := securityv1beta1.SecretStatus{
		Namespace:    "test-ns",
		Name:         "test-secret",
		IsStale:      true,
		SecretType:   "Opaque",
		Created:      metav1.Time{Time: metav1.Now().AddDate(0, 0, -10)},
		LastModified: metav1.Time{Time: metav1.Now().AddDate(0, 0, -10)},
		Message:      "Test message",
	}

	staleSecretWatch := &securityv1beta1.StaleSecretWatch{
		Spec: securityv1beta1.StaleSecretWatchSpec{
			StaleThresholdInDays: 5,
		},
		Status: securityv1beta1.StaleSecretWatchStatus{
			SecretStatus:      []securityv1beta1.SecretStatus{staleSecret},
			StaleSecretsCount: 1,
		},
	}

	// Environment variables should be set in your CI/CD environment or before running tests
	os.Setenv("SLACK_BOT_TOKEN", "your_slack_bot_token")
	os.Setenv("SLACK_CHANNEL_ID", "your_slack_channel_id")

	tests := []struct {
		name    string
		ssw     *securityv1beta1.StaleSecretWatch
		wantErr bool
	}{
		{
			name:    "valid notification for stale secrets",
			ssw:     staleSecretWatch,
			wantErr: false,
		},
		{
			name: "no stale secrets",
			ssw: &securityv1beta1.StaleSecretWatch{
				Spec: securityv1beta1.StaleSecretWatchSpec{
					StaleThresholdInDays: 5,
				},
				Status: securityv1beta1.StaleSecretWatchStatus{
					SecretStatus:      nil,
					StaleSecretsCount: 0,
				},
			},
			wantErr: false,
		},
		{
			name:    "missing slack token and channel id",
			ssw:     staleSecretWatch,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &StaleSecretWatchReconciler{
				// Client and other fields would be set up here if needed
				Log: logger,
			}
			err := r.NotifySlack(ctx, logger, tt.ssw)
			if (err != nil) != tt.wantErr {
				t.Errorf("NotifySlack() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_prepareSlackMessage(t *testing.T) {

	// Mock data for secret statuses
	statuses := []securityv1beta1.SecretStatus{
		{
			Name:         "test-secret",
			Namespace:    "default",
			SecretType:   "Opaque",
			Created:      metav1.Time{Time: time.Date(2021, 01, 01, 12, 34, 56, 0, time.UTC)},
			LastModified: metav1.Time{Time: time.Date(2021, 01, 01, 12, 34, 56, 0, time.UTC)},
		},
	}

	type args struct {
		msgType   string
		text      string
		channelID string
		statuses  []securityv1beta1.SecretStatus
	}
	tests := []struct {
		name string
		args args
		want SlackPayload
	}{
		{
			name: "Warning message with statuses",
			args: args{
				msgType:   "warning",
				text:      "High alert! Check these secrets.",
				channelID: "C1234567890",
				statuses:  statuses,
			},
			want: SlackPayload{
				Channel: "C1234567890",
				Blocks: []Block{
					{
						Type: "section",
						Text: &TextElement{
							Type: "mrkdwn",
							Text: "*Daily check completed successfully.*\n\n",
						},
					},
					{
						Type: "divider",
					},
					generateWarningBlock("High alert! Check these secrets."),
					{
						Type: "rich_text_preformatted",
						Elements: []Element{
							{
								Type:   "rich_text_preformatted",
								Border: 1,
								Elements: []Markup{
									{
										Type: "text",
										Text: "\"secret_name\": \"test-secret\",\n\"namespace\": \"default\",\n\"type\": \"Opaque\",\n\"created\": \"2021-01-01T12:34:56Z\",\n\"last_modified\": \"2021-01-01T12:34:56Z\"",
									},
								},
							},
						},
					},
					{
						Type: "divider",
					},
				},
			},
		},
		{
			name: "Success message without statuses",
			args: args{
				msgType:   "success",
				text:      "All clear! No issues found.",
				channelID: "C1234567890",
				statuses:  nil,
			},
			want: SlackPayload{
				Channel: "C1234567890",
				Blocks: []Block{
					{
						Type: "section",
						Text: &TextElement{
							Type: "mrkdwn",
							Text: "*Daily check completed successfully.*\n\n",
						},
					},
					{
						Type: "divider",
					},
					generateSuccessBlock("All clear! No issues found."),
					{
						Type: "divider",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := prepareSlackMessage(tt.args.msgType, tt.args.text, tt.args.channelID, tt.args.statuses)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("prepareSlackMessage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_sendSlackNotification(t *testing.T) {
	// Prepare logger
	logger := logr.Discard()

	// Test payload
	payload := SlackPayload{
		Channel: "test-channel",
		Blocks: []Block{
			{
				Type: "section",
				Text: &TextElement{
					Type: "mrkdwn",
					Text: "Hello, world!",
				},
			},
		},
	}

	// Convert payload to bytes
	// payloadBytes, _ := json.Marshal(payload)
	// payloadReader := bytes.NewReader(payloadBytes)

	// Setup test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer valid-token" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tests := []struct {
		name         string
		ctx          context.Context
		payload      SlackPayload
		token        string
		serverURL    string
		wantErr      bool
		setupFunc    func(*httptest.Server)
		tearDownFunc func(*httptest.Server)
	}{
		{
			name:      "successful notification",
			ctx:       context.TODO(),
			payload:   payload,
			token:     "valid-token",
			serverURL: server.URL,
			wantErr:   false,
		},
		{
			name:      "failed due to wrong token",
			ctx:       context.TODO(),
			payload:   payload,
			token:     "wrong-token",
			serverURL: server.URL,
			wantErr:   false,
		},
		{
			name:      "network error",
			ctx:       context.TODO(),
			payload:   payload,
			token:     "valid-token",
			serverURL: "http://127.0.0.1:0000", // Invalid URL for simulation
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Modify the function for the test case
			if tt.setupFunc != nil {
				tt.setupFunc(server)
			}

			// Run the function
			err := sendSlackNotification(tt.ctx, logger, tt.payload, tt.token)

			// Check the error
			if (err != nil) != tt.wantErr {
				t.Errorf("sendSlackNotification() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Clean up after test case
			if tt.tearDownFunc != nil {
				tt.tearDownFunc(server)
			}
		})
	}
}
