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
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var stalesecretwatchlog = logf.Log.WithName("stalesecretwatch-validation-webhhok get called")

// SetupWebhookWithManager will setup the manager to manage the webhooks
func (r *StaleSecretWatch) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-security-stalesecretwatch-io-v1beta1-stalesecretwatch,mutating=true,failurePolicy=fail,sideEffects=None,groups=security.stalesecretwatch.io,resources=stalesecretwatches,verbs=create;update,versions=v1beta1,name=mstalesecretwatch.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &StaleSecretWatch{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *StaleSecretWatch) Default() {
	stalesecretwatchlog.Info("default", "name", r.Name)
	if r.Namespace == "" {
		r.Namespace = "default"
	}
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-security-stalesecretwatch-io-v1beta1-stalesecretwatch,mutating=false,failurePolicy=fail,sideEffects=None,groups=security.stalesecretwatch.io,resources=stalesecretwatches,verbs=create;update,versions=v1beta1,name=vstalesecretwatch.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &StaleSecretWatch{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *StaleSecretWatch) ValidateCreate() (admission.Warnings, error) {
	stalesecretwatchlog.Info("validate incoming yaml..", "name", r.Name)

	return nil, r.ValidateStaleSecretWatch()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *StaleSecretWatch) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	stalesecretwatchlog.Info("validate update", "name", r.Name)

	return nil, r.ValidateStaleSecretWatch()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *StaleSecretWatch) ValidateDelete() (admission.Warnings, error) {
	stalesecretwatchlog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil, nil
}

// ValidateStaleSecretWatch validates the fields of the StaleSecretWatch instance.
func (r *StaleSecretWatch) ValidateStaleSecretWatch() error {
	// Validate staleSecretToWatch namespace
	if r.Spec.RefreshInterval == nil {
		// Set default value to 1 hour
		defaultInterval := metav1.Duration{Duration: time.Hour}
		r.Spec.RefreshInterval = &defaultInterval
	}

	if r.Spec.StaleSecretToWatch.Namespace == "" && r.Spec.StaleSecretToWatch.Namespace != "all" {
		return fmt.Errorf(`staleSecretToWatch.namespace cannot be empty, please specify 'all' to watch all namespace or existing namespace name`)
	}

	// Validate excludeList
	for _, exclude := range r.Spec.StaleSecretToWatch.ExcludeList {
		// Validate excludeList secretName
		if exclude.SecretName == "" {
			return fmt.Errorf("excludeList.secretName cannot be empty")
		}

		// Validate excludeList namespace
		if exclude.Namespace == "" {
			return fmt.Errorf("excludeList.namespace cannot be empty")
		}

		// Validate excludeList namespace format
		if strings.Contains(exclude.Namespace, ",") || strings.Contains(exclude.Namespace, " ") {
			return fmt.Errorf("invalid characters in namespace name: %s, exclude.Namespace must be a single/existing namespace name", exclude.Namespace)
		}

	}

	// Additional validation for staleThresholdInDays
	if r.Spec.StaleThresholdInDays <= 0 {
		return fmt.Errorf("staleThresholdInDays must be a positive integer")
	}

	// Additional validation for refreshInterval
	if r.Spec.RefreshInterval != nil {
		if r.Spec.RefreshInterval.Duration <= 0 {
			return fmt.Errorf("refreshInterval duration must be a positive value")
		}
	}

	if r.Namespace == "" {
		stalesecretwatchlog.Info("namespace field to create resource was empty, setting to default", "name", r.Name)
		r.Namespace = "default"
	}

	return nil
}
