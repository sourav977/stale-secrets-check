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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	securityv1beta1 "github.com/sourav977/stale-secrets-watch/api/v1beta1"
)

// StaleSecretWatchReconciler reconciles a StaleSecretWatch object
type StaleSecretWatchReconciler struct {
	client.Client
	Log             logr.Logger
	RequeueInterval time.Duration
	Scheme          *runtime.Scheme
	recorder        record.EventRecorder
}

const (
	typeAvailable   = "Available"
	typeDegraded    = "Degraded"
	typeUnavailable = "Unavailable"
	errGetSSW       = "could not get StaleSecretWatch"
	errNSnotEmpty   = "staleSecretToWatch.namespace cannot be empty"
)

//+kubebuilder:rbac:groups=security.stalesecretwatch.io,resources=stalesecretwatches,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=security.stalesecretwatch.io,resources=stalesecretwatches/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=security.stalesecretwatch.io,resources=stalesecretwatches/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete

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
	logger := log.FromContext(ctx)
	logger = logger.WithValues("stalesecretwatch", req.NamespacedName)

	logger.Info("== Reconcile called == **")

	var staleSecretWatch securityv1beta1.StaleSecretWatch

	// Fetch the StaleSecretWatch instance
	// The purpose is check if the Custom Resource for the Kind StaleSecretWatch
	// is applied on the cluster if not we return nil to stop the reconciliation
	if err := r.Get(ctx, req.NamespacedName, &staleSecretWatch); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("StaleSecretWatch resource not found. Ignoring since StaleSecretWatch object must be deleted")
			// Object not found, return without requeueing
			return ctrl.Result{}, nil
		}
		// Error fetching the StaleSecretWatch instance, requeue the request
		logger.Error(err, errGetSSW)
		return ctrl.Result{}, err
	}

	// Let's just set the status as Unknown when no status are available
	// if staleSecretWatch.Status.Conditions == nil || len(staleSecretWatch.Status.Conditions) == 0 {
	// 	meta.SetStatusCondition(&staleSecretWatch.Status.Conditions, metav1.Condition{Type: typeUnavailable, Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting reconciliation"})
	// 	if err := r.Status().Update(ctx, &staleSecretWatch); err != nil {
	// 		logger.Error(err, "Failed to update StaleSecretWatch status")
	// 		return ctrl.Result{}, err
	// 	}

	// 	// Let's re-fetch the staleSecretWatch Custom Resource after update the status
	// 	// so that we have the latest state of the resource on the cluster and we will avoid
	// 	// raise the issue "the object has been modified, please apply
	// 	// your changes to the latest version and try again" which would re-trigger the reconciliation
	// 	// if we try to update it again in the following operations
	// 	if err := r.Get(ctx, req.NamespacedName, &staleSecretWatch); err != nil {
	// 		logger.Error(err, "Failed to re-fetch StaleSecretWatch")
	// 		return ctrl.Result{}, err
	// 	}
	// }

	if staleSecretWatch.ObjectMeta.CreationTimestamp.IsZero() {
		logger.Info("StaleSecretWatch resource is being created for the first time")

		// Validate the StaleSecretWatch instance
		if err := ValidateStaleSecretWatch(&staleSecretWatch); err != nil {
			logger.Error(err, "Invalid StaleSecretWatch instance")
			return ctrl.Result{}, err
		}

		// Validation passed, create the StaleSecretWatch resource
		if err := r.Create(ctx, &staleSecretWatch); err != nil {
			logger.Error(err, "Failed to create StaleSecretWatch resource")
			return ctrl.Result{}, err
		}
		meta.SetStatusCondition(&staleSecretWatch.Status.Conditions, metav1.Condition{Type: staleSecretWatch.Kind, Status: metav1.ConditionTrue, Reason: "Reconciled", Message: "StaleSecretWatch resource created"})
		if err := r.Status().Update(ctx, &staleSecretWatch); err != nil {
			logger.Error(err, "Failed to update StaleSecretWatch status")
			return ctrl.Result{}, err
		}

	} else {
		// Resource is being updated, perform the regular reconcile logic
		logger.Info("StaleSecretWatch resource is being updated")
	}

	// // Validate the StaleSecretWatch instance
	// if err := ValidateStaleSecretWatch(&staleSecretWatch); err != nil {
	// 	logger.Error(err, "Invalid StaleSecretWatch instance")
	// 	return ctrl.Result{}, err
	// }

	namespacesToWatch, secretsToWatch, err := r.prepareWatchList(ctx, &staleSecretWatch)
	if err != nil {
		logger.Error(err, "Failed to prepare watch list")
		return ctrl.Result{}, err
	}
	logger.Info("Monitoring namespaces and secrets", "namespaces", namespacesToWatch, "secrets", secretsToWatch)

	return ctrl.Result{RequeueAfter: 2 * time.Minute}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *StaleSecretWatchReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.recorder = mgr.GetEventRecorderFor("stale-secret-watch")
	// For defines the type of Object being *reconciled*, and configures the ControllerManagedBy to respond to create / delete /
	// update events by *reconciling the object*.
	// This is the equivalent of calling
	// Watches(&source.Kind{Type: apiType}, &handler.EnqueueRequestForObject{}).
	return ctrl.NewControllerManagedBy(mgr).
		For(&securityv1beta1.StaleSecretWatch{}).
		Owns(&corev1.ConfigMap{}, builder.OnlyMetadata).
		Complete(r)
}

// ValidateStaleSecretWatch validates the fields of the StaleSecretWatch instance.
func ValidateStaleSecretWatch(ssw *securityv1beta1.StaleSecretWatch) error {
	// Validate staleSecretToWatch namespace
	if ssw.Spec.RefreshInterval == nil {
		// Set default value to 1 hour
		defaultInterval := metav1.Duration{Duration: time.Hour}
		ssw.Spec.RefreshInterval = &defaultInterval
	}

	if ssw.Spec.StaleSecretToWatch.Namespace == "" && ssw.Spec.StaleSecretToWatch.Namespace != "all" {
		return fmt.Errorf(`staleSecretToWatch.namespace cannot be empty, please specify 'all' (deafult)`)
	}

	// Validate excludeList
	for _, exclude := range ssw.Spec.StaleSecretToWatch.ExcludeList {
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
			return fmt.Errorf("invalid characters in namespace name: %s, exclude.Namespace must be a single namespace name", exclude.Namespace)
		}

	}

	// Additional validation for staleThresholdInDays
	if ssw.Spec.StaleThresholdInDays <= 0 {
		return fmt.Errorf("staleThresholdInDays must be a positive integer")
	}

	// Additional validation for refreshInterval
	if ssw.Spec.RefreshInterval != nil {
		if ssw.Spec.RefreshInterval.Duration <= 0 {
			return fmt.Errorf("refreshInterval duration must be a positive value")
		}
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

// func shouldRefresh(sw securityv1beta1.StaleSecretWatch) bool {
// 	// refresh if resource version changed
// 	if sw.Status.SyncedResourceVersion != getResourceVersion(sw) {
// 		return true
// 	}

// 	// skip refresh if refresh interval is 0
// 	if sw.Spec.RefreshInterval.Duration == 0 && sw.Status.SyncedResourceVersion != "" {
// 		return false
// 	}
// 	if sw.Status.RefreshTime.IsZero() {
// 		return true
// 	}
// 	return sw.Status.RefreshTime.Add(sw.Spec.RefreshInterval.Duration).Before(time.Now())
// }

// func getResourceVersion(sw securityv1beta1.StaleSecretWatch) string {
// 	return fmt.Sprintf("%d-%s", sw.ObjectMeta.GetGeneration(), hashMeta(sw.ObjectMeta))
// }

// func hashMeta(m metav1.ObjectMeta) string {
// 	type meta struct {
// 		annotations map[string]string
// 		labels      map[string]string
// 	}
// 	return utils.ObjectHash(meta{
// 		annotations: m.Annotations,
// 		labels:      m.Labels,
// 	})
// }

// func shouldReconcile(sw securityv1beta1.StaleSecretWatch) bool {
// 	if sw.Spec.Target.Immutable && hasSyncedCondition(sw) {
// 		return false
// 	}
// 	return true
// }

// func hasSyncedCondition(sw securityv1beta1.StaleSecretWatch) bool {
// 	for _, condition := range sw.Status.Conditions {
// 		if condition.Reason == "SecretSynced" {
// 			return true
// 		}
// 	}
// 	return false
// }

func (r *StaleSecretWatchReconciler) prepareWatchList(ctx context.Context, ssw *securityv1beta1.StaleSecretWatch) ([]string, map[string][]string, error) {
	var namespacesToWatch []string
	secretsToWatch := make(map[string][]string)

	// If watching all namespaces, list all namespaces and add them to namespacesToWatch
	if ssw.Spec.StaleSecretToWatch.Namespace == "all" {
		nsList := &corev1.NamespaceList{}
		if err := r.List(ctx, nsList); err != nil {
			return nil, nil, fmt.Errorf("failed to list namespaces: %v", err)
		}
		for _, ns := range nsList.Items {
			namespacesToWatch = append(namespacesToWatch, ns.Name)
		}
	} else {
		namespacesToWatch = append(namespacesToWatch, ssw.Spec.StaleSecretToWatch.Namespace)
	}

	// Now, list secrets in each namespace and filter based on the excludeList
	for _, ns := range namespacesToWatch {
		secretList := &corev1.SecretList{}
		if err := r.List(ctx, secretList, client.InNamespace(ns)); err != nil {
			return nil, nil, fmt.Errorf("failed to list secrets in namespace %s: %v", ns, err)
		}
		for _, secret := range secretList.Items {
			// Check if this secret is in the excludeList for its namespace
			excluded := false
			for _, excludeEntry := range ssw.Spec.StaleSecretToWatch.ExcludeList {
				if excludeEntry.Namespace == ns && contains(excludeEntry.SecretName, secret.Name) {
					excluded = true
					break
				}
			}
			if !excluded {
				secretsToWatch[ns] = append(secretsToWatch[ns], secret.Name)
			}
		}
	}

	return namespacesToWatch, secretsToWatch, nil

}

// contains checks if a comma-separated string contains a specific value.
func contains(commaSeparatedString, value string) bool {
	values := strings.Split(commaSeparatedString, ",")
	for _, v := range values {
		if strings.TrimSpace(v) == value {
			return true
		}
	}
	return false
}
