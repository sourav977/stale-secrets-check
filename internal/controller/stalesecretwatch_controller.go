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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	securityv1beta1 "github.com/sourav977/stale-secrets-watch/api/v1beta1"
)

// StaleSecretWatchReconciler reconciles a StaleSecretWatch object
type StaleSecretWatchReconciler struct {
	client.Client
	Log             logr.Logger
	RequeueInterval time.Duration
	Scheme          *runtime.Scheme
	Recorder        record.EventRecorder
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
//+kubebuilder:rbac:groups="",resources=secrets/status,verbs=get
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=configmaps/status,verbs=get
//+kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=namespaces/status,verbs=get;update;patch

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

	logger.Info("Reconcile called")

	var staleSecretWatch securityv1beta1.StaleSecretWatch

	// Fetch the StaleSecretWatch instance
	// The purpose is check if the Custom Resource for the Kind StaleSecretWatch
	// is applied on the cluster if not we return nil to stop the reconciliation
	if err := r.Get(ctx, req.NamespacedName, &staleSecretWatch); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("StaleSecretWatch resource not found. Ignoring since StaleSecretWatch object must be deleted. Exit Reconcile.")
			// Object not found, return without requeueing
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
		// Error fetching the StaleSecretWatch instance, requeue the request
		logger.Error(err, errGetSSW)
		return ctrl.Result{}, err
	}

	// This will allow for the StaleSecretWatch to be reconciled when changes to the Secret are noticed.
	var secret corev1.Secret
	if err := controllerutil.SetControllerReference(&staleSecretWatch, &secret, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	// Validation passed, it must be validated by webhook, now create the StaleSecretWatch resource
	if err := r.createOrUpdateStaleSecretWatch(ctx, logger, &staleSecretWatch); err != nil {
		return ctrl.Result{}, err
	}

	// meta.SetStatusCondition(&staleSecretWatch.Status.Conditions, metav1.Condition{Type: staleSecretWatch.Kind, Status: metav1.ConditionTrue, Reason: "Reconciled", Message: "StaleSecretWatch resource created"})
	// if err := r.Status().Update(ctx, &staleSecretWatch); err != nil {
	// 	logger.Error(err, "Failed to update StaleSecretWatch status")
	// 	return ctrl.Result{}, err
	// }

	//now prepare the namespace and secret list to watch
	namespacesToWatch, secretsToWatch, err := r.prepareWatchList(ctx, logger, &staleSecretWatch)
	if err != nil {
		logger.Error(err, "Failed to prepare watch list")
		return ctrl.Result{}, err
	}
	logger.Info("Monitoring namespaces and secrets", "namespaces", namespacesToWatch, "secrets", secretsToWatch)

	return ctrl.Result{RequeueAfter: 2 * time.Minute}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *StaleSecretWatchReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("stale-secret-watch")
	// For defines the type of Object being *reconciled*, and configures the ControllerManagedBy to respond to create / delete /
	// update events by *reconciling the object*.
	// This is the equivalent of calling
	// Watches(&source.Kind{Type: apiType}, &handler.EnqueueRequestForObject{}).
	return ctrl.NewControllerManagedBy(mgr).
		For(&securityv1beta1.StaleSecretWatch{}).
		Owns(&corev1.ConfigMap{}, builder.OnlyMetadata).
		Complete(r)
}

func (r *StaleSecretWatchReconciler) createOrUpdateStaleSecretWatch(ctx context.Context, logger logr.Logger, ssw *securityv1beta1.StaleSecretWatch) error {
	// Check if the StaleSecretWatch resource already exists
	found := &securityv1beta1.StaleSecretWatch{}
	err := r.Get(ctx, types.NamespacedName{Name: ssw.Name, Namespace: ssw.Namespace}, found)
	if err != nil && !errors.IsNotFound(err) {
		logger.Error(err, "Failed to check if StaleSecretWatch resource exists")
		return err
	}

	if errors.IsNotFound(err) || err != nil {
		// Create the StaleSecretWatch resource as it does not exist
		logger.Info("Creating StaleSecretWatch", "Namespace", ssw.Namespace, "Name", ssw.Name)
		if ssw.Namespace == "" {
			ssw.Namespace = "default"
		}
		err = r.Create(ctx, ssw)
		if err != nil {
			logger.Error(err, "Failed to create StaleSecretWatch resource")
			return err
		}
		meta.SetStatusCondition(&ssw.Status.Conditions, metav1.Condition{Type: ssw.Kind, Status: metav1.ConditionTrue, Reason: "Created", Message: "StaleSecretWatch resource named " + ssw.Name + " created"})
	} else {
		// Update the StaleSecretWatch resource if it already exists
		logger.Info("Updating StaleSecretWatch", "Namespace", ssw.Namespace, "Name", ssw.Name)
		found.Spec = ssw.Spec
		found.Status = ssw.Status
		err = r.Update(ctx, found)
		if err != nil {
			logger.Error(err, "Failed to update StaleSecretWatch resource")
			return err
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

func (r *StaleSecretWatchReconciler) prepareWatchList(ctx context.Context, logger logr.Logger, ssw *securityv1beta1.StaleSecretWatch) ([]string, map[string][]string, error) {
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
		ns := strings.Split(ssw.Spec.StaleSecretToWatch.Namespace, ",")
		for _, n := range ns {
			namespacesToWatch = append(namespacesToWatch, strings.TrimSpace(n))
		}
	}

	// Now, list secrets in each namespace and filter based on the excludeList
	for _, ns := range namespacesToWatch {
		secretList := &corev1.SecretList{}
		if err := r.List(ctx, secretList, client.InNamespace(ns)); err != nil || errors.IsNotFound(err) {
			logger.Info("secret resources not found in " + ns)
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
