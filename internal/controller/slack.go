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

const (
	RT  = "rich_text"
	RTF = "rich_text_preformatted"
)

type Style struct {
	Bold bool `json:"bold,omitempty"`
}

// Updated Divider struct with explicit type field
type Divider struct {
	Type string `json:"type"`
}

// NotifySlack prepares Blocks to send msg over slack
func (r *StaleSecretWatchReconciler) NotifySlack(ctx context.Context, logger logr.Logger, staleSecretWatch *securityv1beta1.StaleSecretWatch) error {
	token := os.Getenv("SLACK_BOT_TOKEN")
	channelID := os.Getenv("SLACK_CHANNEL_ID")
	if token == "" || channelID == "" {
		return fmt.Errorf("SLACK_BOT_TOKEN or SLACK_CHANNEL_ID is not set")
	}

	clusterName := GetClusterName()
	if len(staleSecretWatch.Status.SecretStatus) <= 0 {
		successText := "All secrets are up to date. Good work!\t"
		payload := prepareSlackMessage("success", successText, channelID, nil)
		logger.Info(successText)
		return sendSlackNotification(ctx, logger, payload, token)
	}

	warningText := fmt.Sprintf("Below is a list of secret resources where secret data has not been modified for StaleThresholdInDays %d days in %s Cluster!! \n Total %d number of stale secrets found.\n", staleSecretWatch.Spec.StaleThresholdInDays, clusterName, staleSecretWatch.Status.StaleSecretsCount)
	payload := prepareSlackMessage("warning", warningText, channelID, staleSecretWatch.Status.SecretStatus)
	// Marshal the complete payload
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		logger.Error(err, "Failed to encode ConfigData to JSON")
		return err
	}
	logger.Info("This Info will send to Slack: ", "payload", string(payloadBytes))
	return sendSlackNotification(ctx, logger, payload, token)
}

// prepareSlackMessage prepares slack msg
func prepareSlackMessage(msgType, text, channelID string, statuses []securityv1beta1.SecretStatus) SlackPayload {
	var blocks []Block

	// Initial common blocks
	blocks = append(blocks, Block{
		Type: "section",
		Text: &TextElement{
			Type: "mrkdwn",
			Text: "*Daily check completed successfully.*\n\n",
		},
	}, Block{
		Type: "divider",
	})

	if msgType != "warning" {
		blocks = append(blocks, generateSuccessBlock(text))
	} else {
		blocks = append(blocks, generateWarningBlock(text))
		// Add secret details if present
		for _, status := range statuses {
			info := fmt.Sprintf("\"secret_name\": \"%s\",\n\"namespace\": \"%s\",\n\"type\": \"%s\",\n\"created\": \"%s\",\n\"last_modified\": \"%s\"",
				status.Name, status.Namespace, status.SecretType, status.Created.Format("2006-01-02T15:04:05Z"), status.LastModified.Format("2006-01-02T15:04:05Z"))
			blocks = append(blocks, Block{
				Type: RT,
				Elements: []Element{
					{
						Type:   RTF,
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
	}

	// Final divider
	blocks = append(blocks, Block{Type: "divider"})

	return SlackPayload{
		//Channel: "C06UV9S4DC0", // Your Slack channel ID
		Channel: channelID,
		Blocks:  blocks,
	}
}

// generateWarningBlock appends warning Block to main Block
func generateWarningBlock(text string) Block {
	return Block{
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
			{
				Type: "rich_text_section",
				Elements: []Markup{
					{
						Type:  "text",
						Text:  text,
						Style: &Style{Bold: true},
					},
					{
						Type: "text",
						Text: "\n\n",
					},
				},
			},
		},
	}
}

// generateSuccessBlock adds success Block to main Block
func generateSuccessBlock(text string) Block {
	return Block{
		Type: "rich_text",
		Elements: []Element{
			{
				Type: "rich_text_section",
				Elements: []Markup{
					{
						Type:  "text",
						Text:  text,
						Style: &Style{Bold: true},
					},
					{
						Type: "emoji",
						Name: "smiley",
					},
				},
			},
		},
	}
}

// sendSlackNotification actually sends message to slack channel
// using curl/http to send msg to slack rather using slack golang library
func sendSlackNotification(ctx context.Context, logger logr.Logger, payload SlackPayload, token string) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		logger.Error(err, "Failed to encode ConfigData to JSON")
		return err
	}

	logger.Info("This Info will send to Slack", "payload", string(payloadBytes))
	reqBody := bytes.NewReader(payloadBytes)
	req, err := http.NewRequestWithContext(ctx, "POST", "https://slack.com/api/chat.postMessage", reqBody)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	logger.Info("Message sent to Slack successfully")
	return nil
}
