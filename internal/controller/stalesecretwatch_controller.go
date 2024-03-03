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

package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	stalesecretwatchv1 "github.com/sourav977/stale-secrets-check/api/v1beta1"
)

// StaleSecretWatchReconciler reconciles a StaleSecretWatch object
type StaleSecretWatchReconciler struct {
	client.Client
	Log             logr.Logger
	RequeueInterval time.Duration
	Scheme          *runtime.Scheme
}

const (
	errGetSSW = "could not get StaleSecretWatch"
)

//+kubebuilder:rbac:groups=github.com.github.com,resources=stalesecretwatches,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=github.com.github.com,resources=stalesecretwatches/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=github.com.github.com,resources=stalesecretwatches/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the StaleSecretWatch object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.15.0/pkg/reconcile
func (r *StaleSecretWatchReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	log := r.Log.WithValues("stalesecretwatch", req.NamespacedName)

	var staleSecretWatch stalesecretwatchv1.StaleSecretWatch

	// Fetch the StaleSecretWatch instance
	if err := r.Get(ctx, req.NamespacedName, &staleSecretWatch); err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return without requeueing
			return ctrl.Result{}, nil
		}
		// Error fetching the StaleSecretWatch instance, requeue the request
		log.Error(err, errGetSSW)
		return ctrl.Result{}, err
	}

	refreshInt := r.RequeueInterval
	if staleSecretWatch.Spec.RefreshInterval != nil {
		refreshInt = staleSecretWatch.Spec.RefreshInterval.Duration
	}

	// refreshinterval logic to be done

	// Calculate the threshold time for a secret to be considered stale (90 days)
	thresholdTime := time.Now().AddDate(0, 0, -90)

	// Iterate over the secrets and check if any are stale
	for _, secret := range secretsList.Items {
		// Check if the secret creation or modification time is older than the threshold time
		if secret.ObjectMeta.CreationTimestamp.Time.Before(thresholdTime) {
			staleSecrets = append(staleSecrets, secret)
		}
	}

	// Logic to take action based on stale secrets (e.g., send notification, delete, etc.)

	// Update the status of the StaleSecretWatch instance
	staleSecretWatch.Status.StaleSecretsCount = len(staleSecrets)
	if err := r.Status().Update(ctx, staleSecretWatch); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Reconciliation successful")
	return ctrl.Result{}, nil

}

// SetupWithManager sets up the controller with the Manager.
func (r *StaleSecretWatchReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&stalesecretwatchv1.StaleSecretWatch{}).
		Owns(&v1.ConfigMap{}, builder.OnlyMetadata).
		Complete(r)
}

func (r *StaleSecretWatchReconciler) listAndInspectSecrets(ctx context.Context, instance *stalesecretwatchv1.StaleSecretWatch) ([]corev1.Secret, error) {
	var secrets []corev1.Secret

	// Check if the namespace is specified
	if instance.Spec.SecretStoreRef.Namespace != "" && instance.Spec.SecretStoreRef.Namespace != "all" {
		// Namespace is specified, so list secrets only in that namespace
		var secretListInNamespace corev1.SecretList
		err := r.Client.List(ctx, &secretListInNamespace, client.InNamespace(instance.Spec.SecretStoreRef.Namespace))
		if err != nil {
			return nil, err
		}
		secrets = secretListInNamespace.Items
	} else {
		// Namespace is not specified, so list secrets across all namespaces by default
		var allSecrets corev1.SecretList
		err := r.Client.List(ctx, &allSecrets)
		if err != nil {
			return nil, err
		}
		secrets = allSecrets.Items
	}

	// Filter secrets based on the provided key, secret type, and variable
	for _, secret := range secrets {
		if instance.Spec.SecretStoreRef.Key != "" && secret.Data[instance.Spec.SecretStoreRef.Key] == nil {
			continue
		}
		if instance.Spec.SecretStoreRef.SecretType != "" && secret.Labels["type"] != instance.Spec.SecretStoreRef.SecretType {
			continue
		}
		if instance.Spec.SecretStoreRef.Variable != "" && secret.Annotations["variable"] != instance.Spec.SecretStoreRef.Variable {
			continue
		}
		secrets = append(secrets, secret)
	}

	return secrets, nil
}

func (r *StaleSecretWatchReconciler) detectStaleSecrets(secrets []corev1.Secret, staleThreshold int) ([]corev1.Secret, error) {
	var staleSecrets []corev1.Secret

	for _, secret := range secrets {
		// Compare the creation or modification time with the current time
		age := time.Now().Sub(secret.GetCreationTimestamp().Time)
		if age.Hours() > float64(staleThreshold*24) {
			staleSecrets = append(staleSecrets, secret)
		}
	}

	return staleSecrets, nil
}

func (r *StaleSecretWatchReconciler) storeAndCheckHashedSecrets(ctx context.Context, instance *stalesecretwatchv1.StaleSecretWatch, secrets []corev1.Secret) error {
	for _, secret := range secrets {
		// Calculate hash of the secret value
		hash := calculateHash(secret.Data)

		// Check if the hash already exists in the ConfigMap
		// Implement logic to fetch the stored hash from ConfigMap and compare with the calculated hash
	}

	return nil
}

func (r *StaleSecretWatchReconciler) updateStatus(ctx context.Context, instance *stalesecretwatchv1.StaleSecretWatch, staleSecrets []corev1.Secret) error {
	// Update the status with the stale secrets count and other details
	conditions := []stalesecretwatchv1.StaleSecretWatchCondition{
		{
			Type:               "StaleSecretsDetected",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: metav1.Now(),
			Reason:             "StaleSecretsFound",
			Message:            fmt.Sprintf("Found %d stale secrets", len(staleSecrets)),
		},
	}

	status := stalesecretwatchv1.StaleSecretWatchStatus{
		Conditions:        conditions,
		LastUpdateTime:    metav1.Now(),
		StaleSecretsCount: len(staleSecrets),
	}

	instance.Status = status
	if err := r.Status().Update(ctx, instance); err != nil {
		return err
	}

	return nil
}

func calculateHash(data map[string][]byte) string {
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

func (r *StaleSecretWatchReconciler) calculateAndStoreHashedSecrets(ctx context.Context, secrets []corev1.Secret) error {
	// Iterate over each secret
	for _, secret := range secrets {
		// Calculate hash of secret data
		hash := calculateHash(secret.Data)

		// Create or update ConfigMap with hashed secret data
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "hashed-secrets", // Name of the ConfigMap
				Namespace: secret.Namespace,
			},
			Data: map[string]string{
				secret.Name: hash,
			},
		}

		// Check if ConfigMap exists
		found := &corev1.ConfigMap{}
		err := r.Get(ctx, client.ObjectKey{Namespace: cm.Namespace, Name: cm.Name}, found)
		if err != nil {
			if errors.IsNotFound(err) {
				// ConfigMap does not exist, create it
				err = r.Create(ctx, cm)
				if err != nil {
					return err
				}
			} else {
				return err
			}
		} else {
			// ConfigMap exists, update it
			found.Data[secret.Name] = hash
			err = r.Update(ctx, found)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
