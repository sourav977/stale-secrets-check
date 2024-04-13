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
