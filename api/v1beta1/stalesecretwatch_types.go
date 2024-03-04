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

// StaleSecretToWatch refers to the StaleSecretToWatch resource to watch for stale secrets.
type StaleSecretToWatch struct {
	// Namespace of the Secret resource. namespace=all or namespace=namespace1 or namespace=namespace1,namespace2 comma separated
	Namespace string `json:"namespace"`
	//exclude stale secret watch of below secrets present in namespace
	ExcludeList ExcludeList `json:"excludeList,omitempty"`
}

// ExcludeList is to exclude secret watch
type ExcludeList struct {
	//name of the secret resource to exclude watch
	SecretName string `json:"secretName"`
	//namespace where secret resource resides
	Namespace string `json:"namespace"`
}

// StaleSecretWatchCondition represents the current condition of the StaleSecretWatch.
type StaleSecretWatchCondition struct {
	// Status of the condition.
	Status metav1.ConditionStatus `json:"status"`

	// LastUpdateTime is the timestamp of the last status update.
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`

	// Reason is "StaleSecretsDetected"
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

	// Type or kind of the secret being monitored. Opaque dockerconfig etc
	SecretType string `json:"secretType,omitempty"`

	// LastUpdateTime is the timestamp of the last update to the monitored secret.
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`

	// LastTransitionTime is the timestamp when the condition last transitioned.
	LastTransitionTime metav1.Time `json:"lastTransitionTime"`

	// IsStale indicates whether the secret is stale or not.
	IsStale bool `json:"isStale,omitempty"`
}

// StaleSecretWatchSpec defines the desired state of StaleSecretWatch
type StaleSecretWatchSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// StaleSecretToWatch points to the namespace and secret to watch for stale secrets.
	StaleSecretToWatch StaleSecretToWatch `json:"StaleSecretToWatch"`

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

	// Conditions represent the current conditions of the secret
	Conditions []StaleSecretWatchCondition `json:"conditions,omitempty"`

	// SecretStatus provides detailed information about the monitored secret's status.
	SecretStatus []SecretStatus `json:"secretStatus,omitempty"`

	StaleSecretsCount int
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
