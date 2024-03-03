/*
Copyright 2024.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// SecretStoreRef refers to the SecretStore resource to watch for stale secrets.
type SecretStoreRef struct {
	// Name of the SecretStore resource.
	Name string `json:"name"`

	// Namespace of the SecretStore resource.
	Namespace string `json:"namespace"`

	// Key within the SecretStore from which to monitor secrets.
	Key string `json:"key,omitempty"`

	// SecretType specifies the type or kind of secrets to watch (e.g., username/password, API key).
	SecretType string `json:"secretType,omitempty"`

	// Variable within the specified secret (if applicable).
	Variable string `json:"variable,omitempty"`

	ExcludeList ExcludeList `json:"excludeList"`
}

// StaleSecretWatchCondition represents the current condition of the StaleSecretWatch.
type StaleSecretWatchCondition struct {
	// Type of the condition.
	Type string `json:"type"`

	// Status of the condition.
	Status metav1.ConditionStatus `json:"status"`

	// LastTransitionTime is the timestamp when the condition last transitioned.
	LastTransitionTime metav1.Time `json:"lastTransitionTime"`

	// Reason is a brief reason for the condition's last transition.
	Reason string `json:"reason,omitempty"`

	// Message is a human-readable message indicating details about the condition's last transition.
	Message string `json:"message,omitempty"`
}

// SecretStatus provides detailed information about the monitored secret's status.
type SecretStatus struct {
	// Name of the secret being monitored.
	Name string `json:"name,omitempty"`

	// Namespace of the secret being monitored.
	Namespace string `json:"namespace,omitempty"`

	// Key within the secret being monitored.
	Key string `json:"key,omitempty"`

	// Type or kind of the secret being monitored.
	SecretType string `json:"secretType,omitempty"`

	// Variable within the secret being monitored (if applicable).
	Variable string `json:"variable,omitempty"`

	// LastUpdateTime is the timestamp of the last update to the monitored secret.
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`

	// IsStale indicates whether the secret is stale or not.
	IsStale bool `json:"isStale,omitempty"`
}

// StaleSecretWatchSpec defines the desired state of StaleSecretWatch
type StaleSecretWatchSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// SecretStoreRef points to the SecretStore to watch for stale secrets.
	SecretStoreRef SecretStoreRef `json:"secretStoreRef"`

	// StaleThreshold defines the threshold (in days) beyond which a secret is considered stale.
	StaleThreshold int `json:"staleThresholdInDays"`

	// RefreshInterval is the amount of time after which the Reconciler would watch the cluster
	// Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h"
	// May be set to zero to fetch and create it once. Defaults to 1h.
	// +kubebuilder:default="1h"
	RefreshInterval *metav1.Duration `json:"refreshInterval,omitempty"`
}

// StaleSecretWatchStatus defines the observed state of StaleSecretWatch
type StaleSecretWatchStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Conditions represent the current conditions of the StaleSecretWatch.
	Conditions []StaleSecretWatchCondition `json:"conditions,omitempty"`

	// LastUpdateTime is the timestamp of the last status update.
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`

	// SecretStatus provides detailed information about the monitored secret's status.
	SecretStatus SecretStatus `json:"secretStatus,omitempty"`

	StaleSecretsCount int
}

type ExcludeList struct {
	SecretName string `json:"secretName"`
	Namespace  string `json:"namespace"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// StaleSecretWatch is the Schema for the stalesecretwatches API
type StaleSecretWatch struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StaleSecretWatchSpec   `json:"spec,omitempty"`
	Status StaleSecretWatchStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// StaleSecretWatchList contains a list of StaleSecretWatch
type StaleSecretWatchList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []StaleSecretWatch `json:"items"`
}

func init() {
	SchemeBuilder.Register(&StaleSecretWatch{}, &StaleSecretWatchList{})
}
