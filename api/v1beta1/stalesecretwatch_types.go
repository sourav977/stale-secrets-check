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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StaleSecretToWatch refers to the StaleSecretToWatch resource to watch for stale secrets.
type StaleSecretToWatch struct {
	// Namespace of the Secret resource. namespace=all or namespace=namespace1 or namespace=namespace1,namespace2 comma separated
	//+kubebuilder:validation:Pattern:=`^[a-zA-Z0-9-_]+$`
	Namespace string `json:"namespace"`
	// exclude stale secret watch of below secrets present in namespace
	ExcludeList []ExcludeList `json:"excludeList,omitempty"`
}

// ExcludeList is to exclude secret watch
type ExcludeList struct {
	// namespace where secret resource resides, single namespace name only
	//+kubebuilder:validation:Pattern:=`^[a-zA-Z0-9-_]+$`
	Namespace string `json:"namespace"`
	// name of the secret resource to exclude watch, comma separated or sinlge secretName example: secret1, secret2
	//+kubebuilder:validation:Pattern:=`^[a-zA-Z0-9._-]+(?:,\s*[a-zA-Z0-9._-]+)*$`
	SecretName string `json:"secretName"`
}

// StaleSecretWatchCondition represents the current condition of the StaleSecretWatch.
type StaleSecretWatchCondition struct {
	// Status of the condition.
	Status corev1.ConditionStatus `json:"status"`

	// LastUpdateTime is the timestamp of the last status update.
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`

	// Message is a human-readable message indicating details about the condition's last transition.
	// like "StaleSecretsDetected" or "StaleSecretsNotDetected"
	Message string `json:"message,omitempty"`
}

// SecretStatus provides detailed information about the monitored secret's status.
type SecretStatus struct {
	// Namespace of the secret being monitored.
	Namespace string `json:"namespace,omitempty"`

	// Name of the secret being monitored.
	Name string `json:"name,omitempty"`

	// Type or kind of the secret being monitored. Opaque dockerconfig etc
	SecretType string `json:"secretType,omitempty"`

	// LastUpdateTime is the timestamp of the last update to the monitored secret.
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`

	// IsStale indicates whether the secret is stale or not.
	IsStale bool `json:"isStale,omitempty"`

	// Message is a human-readable message indicating details
	Message string `json:"message,omitempty"`
}

// StaleSecretWatchSpec defines the desired state of StaleSecretWatch
type StaleSecretWatchSpec struct {
	// StaleSecretToWatch points to the namespace and secret to watch for stale secrets.
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	StaleSecretToWatch StaleSecretToWatch `json:"staleSecretToWatch"`

	// StaleThreshold defines the threshold (in days) beyond which a secret is considered stale.
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	StaleThresholdInDays int `json:"staleThresholdInDays"`

	// RefreshInterval is the amount of time after which the Reconciler would watch the cluster
	// Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h"
	// May be set to zero to fetch and create it once. Defaults to 1h.
	// +kubebuilder:default="1h"
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	RefreshInterval *metav1.Duration `json:"refreshInterval,omitempty"`
}

// StaleSecretWatchStatus defines the observed state of StaleSecretWatch
type StaleSecretWatchStatus struct {
	// Conditions represent the current conditions of the StaleSecretWatch resource
	// +operator-sdk:csv:customresourcedefinitions:type=status
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// SecretStatus provides detailed information about the monitored secret's status.
	SecretStatus []SecretStatus `json:"secretStatus,omitempty"`

	// StaleSecretsCount in the number of stale secret found
	StaleSecretsCount int `json:"staleSecretCount,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=ssw
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
