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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// func RetrieveModifiedTime(om metav1.ObjectMeta) string {
// 	// Iterate over managedFields to find the element where "manager" contains "edit"
// 	for _, field := range om.ManagedFields {
// 		if strings.Contains(field.Manager, "edit") {
// 			// Assuming Time is of type metav1.Time and you want to format it
// 			if field.Time != nil {
// 				return field.Time.UTC().Format(time.RFC3339)
// 			}
// 			break
// 		}
// 	}
// 	// If no "edit" action found, return the creation timestamp
// 	return om.CreationTimestamp.UTC().Format(time.RFC3339)
// }

func RetrieveModifiedTime(om metav1.ObjectMeta) string {
	if len(om.ManagedFields) > 0 {
		return om.ManagedFields[len(om.ManagedFields)-1].Time.UTC().Format(time.RFC3339)
	}
	// If no "edit" action found, return the creation timestamp
	return om.CreationTimestamp.UTC().Format(time.RFC3339)
}

func MarshalConfigData(configData ConfigData) ([]byte, error) {
	jsonData, err := json.Marshal(configData)
	if err != nil {
		return nil, fmt.Errorf("failed to encode ConfigData to JSON: %v", err)
	}
	return jsonData, nil
}

// contains checks if a comma-separated string contains a specific value.
func Contains(commaSeparatedString, value string) bool {
	values := strings.Split(commaSeparatedString, ",")
	for _, v := range values {
		if strings.TrimSpace(v) == value {
			return true
		}
	}
	return false
}

func CalculateHash(data map[string][]byte) string {
	hash := sha256.New()

	// Sort the data map by key to ensure consistent hash calculation
	var keys []string
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	// Append each key-value pair to the hash
	for _, key := range keys {
		hash.Write(data[key])
	}

	// Return the hexadecimal representation of the hash
	return hex.EncodeToString(hash.Sum(nil))
}

// labelsForStaleSecretWatchConfigMap returns the labels for selecting the resources
// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/
func LabelsForStaleSecretWatchConfigMap(name string) map[string]string {
	return map[string]string{"app.kubernetes.io/name": "StaleSecretWatch",
		"app.kubernetes.io/instance":   name,
		"app.kubernetes.io/part-of":    "StaleSecretWatch-operator",
		"app.kubernetes.io/created-by": "controller-manager-stalesecretwatch",
	}
}

// Helper to parse time in RFC3339 from the JSON data
func ParseTime(timeStr string) (time.Time, error) {
	return time.Parse(time.RFC3339, timeStr)
}

// // GetNextNineAMUTC ensures that your logic only runs once per day at the specified time.
// func GetNextNineAMUTC() time.Duration {
// 	now := time.Now().UTC()
// 	nextNineAM := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, time.UTC)
// 	if now.After(nextNineAM) {
// 		nextNineAM = nextNineAM.Add(24 * time.Hour)
// 	}
// 	return nextNineAM.Sub(now)
// }

// GetNextFiveMinutes calculates the duration until the next 5-minute mark to facilitate running a task every 5 minutes.
func GetNextFiveMinutes() time.Duration {
	now := time.Now().UTC()
	nextFiveMinuteMark := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), (now.Minute()/5+1)*5, 0, 0, time.UTC)
	return nextFiveMinuteMark.Sub(now)
}
