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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/go-logr/logr"
	securityv1beta1 "github.com/sourav977/stale-secrets-watch/api/v1beta1"
)

type SlackPayload struct {
	Channel string  `json:"channel"`
	Blocks  []Block `json:"blocks"`
}

type Block struct {
	Type     string       `json:"type"`
	Text     *TextElement `json:"text,omitempty"`
	Elements []Element    `json:"elements,omitempty"`
	Divider  *Divider     `json:"divider,omitempty"`
}

type TextElement struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type Element struct {
	Type     string   `json:"type"`
	Elements []Markup `json:"elements,omitempty"`
	Text     string   `json:"text,omitempty"`
	Border   int      `json:"border,omitempty"`
	Style    *Style   `json:"style,omitempty"`
}

type Markup struct {
	Type  string `json:"type"`
	Text  string `json:"text,omitempty"`
	Name  string `json:"name,omitempty"`
	Style *Style `json:"style,omitempty"`
}

type Style struct {
	Bold bool `json:"bold,omitempty"`
}

// Updated Divider struct with explicit type field
type Divider struct {
	Type string `json:"type"`
}

// func (r *StaleSecretWatchReconciler) NotifySlack(ctx context.Context, logger logr.Logger,  staleSecretWatch *securityv1beta1.StaleSecretWatch, staleSecretsCount *securityv1beta1.StaleSecretWatch.Status.StaleSecretsCount) error {
func (r *StaleSecretWatchReconciler) NotifySlack(ctx context.Context, logger logr.Logger, staleSecretWatch *securityv1beta1.StaleSecretWatch) error {
	token := os.Getenv("SLACK_BOT_TOKEN")
	if token == "" {
		logger.Error(fmt.Errorf("SLACK_BOT_TOKEN is not set"), "Failed to get environment variable")
	}
	cluster_name := GetClusterName()
	warningText := fmt.Sprintf("Below is a list of secret resources where secret data has not been modified for 90 days in %s Cluster!!", cluster_name)

	// the fixed channel and initial blocks that are static as per your JSON structure
	payload := SlackPayload{
		Channel: "C06UV9S4DC0", // slack-channel-ID
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
			{
				Type: "rich_text",
				Elements: []Element{
					{
						Type: "rich_text_section",
						Elements: []Markup{
							{
								Type: "emoji",
								Name: "warning",
							},
							{
								Type:  "text",
								Text:  "Stale Secret Detected !!!",
								Style: &Style{Bold: true},
							},
						},
					},
				},
			},
			{
				Type: "rich_text",
				Elements: []Element{
					{
						Type: "rich_text_section",
						Elements: []Markup{
							{
								Type:  "text",
								Text:  warningText,
								Style: &Style{Bold: true},
							},
							{
								Type: "text",
								Text: "\n\n",
							},
						},
					},
				},
			},
		},
	}

	// Dynamically add blocks for each secret status
	for _, status := range staleSecretWatch.Status.SecretStatus {
		info := fmt.Sprintf("\"secret_name\": \"%s\",\n\"namespace\": \"%s\",\n\"type\": \"%s\",\n\"created\": \"%s\",\n\"last_modified\": \"%s\"",
			status.Name, status.Namespace, status.SecretType, status.Created.Format("2006-01-02T15:04:05Z"), status.LastModified.Format("2006-01-02T15:04:05Z"))

		payload.Blocks = append(payload.Blocks, Block{
			Type: "rich_text",
			Elements: []Element{
				{
					Type:   "rich_text_preformatted",
					Border: 1,
					Elements: []Markup{
						{
							Type: "text",
							Text: info,
						},
					},
				},
			},
		})
	}

	// Final divider
	payload.Blocks = append(payload.Blocks, Block{Type: "divider"})

	// Marshal the complete payload
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		logger.Error(err, "Failed to encode ConfigData to JSON")
		return err
	}
	logger.Info("This Info will send to Slack", "payload", string(payloadBytes))

	body := bytes.NewReader(payloadBytes)
	req, err := http.NewRequest("POST", "https://slack.com/api/chat.postMessage", body)
	if err != nil {
		logger.Error(err, "Error creating request")
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		logger.Info("Error sending request to Slack:", "error", err)
		return err
	}

	defer resp.Body.Close()
	logger.Info("Message sent to Slack successfully")
	return nil
}
