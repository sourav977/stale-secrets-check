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
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

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
	typeAvailable             = "Available"
	typeDegraded              = "Degraded"
	typeUnavailable           = "Unavailable"
	errGetSSW                 = "could not get StaleSecretWatch"
	errNSnotEmpty             = "staleSecretToWatch.namespace cannot be empty"
	stalesecretwatchFinalizer = "security.stalesecretwatch.io/finalizer"
)

//+kubebuilder:rbac:groups=security.stalesecretwatch.io,resources=stalesecretwatches,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=security.stalesecretwatch.io,resources=stalesecretwatches/status,verbs=get;patch;update
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
	r.Recorder.Event(&staleSecretWatch, "Normal", "ReconcileStarted", "Reconciliation process started")

	// Fetch the StaleSecretWatch instance
	// The purpose is check if the Custom Resource for the Kind StaleSecretWatch
	// is applied on the cluster if not we return nil to stop the reconciliation
	if err := r.Get(ctx, req.NamespacedName, &staleSecretWatch); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("StaleSecretWatch resource not found. Ignoring since StaleSecretWatch object must be deleted. Exit Reconcile.")
			r.Recorder.Event(&staleSecretWatch, "Normal", "NotFound", "StaleSecretWatch resource not found. Ignoring and exiting reconcile loop.")
			// Object not found, return without requeueing
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
		// Error fetching the StaleSecretWatch instance, requeue the request
		logger.Error(err, errGetSSW)
		return ctrl.Result{}, err
	}

	if err := r.updateStatusCondition(ctx, &staleSecretWatch, "ReconcileStarted", metav1.ConditionTrue, "ReconcileInitiated", "Reconciliation process has started"); err != nil {
		return ctrl.Result{}, err
	}

	// Check if the instance is marked to be deleted
	// this handles deletion and finalizers
	if staleSecretWatch.GetDeletionTimestamp() != nil {
		return r.removeFinalizer(ctx, logger, &staleSecretWatch, &req)
	}

	// Add a finalizer if it does not exist
	r.addFinalizer(ctx, logger, &staleSecretWatch, &req)

	// Check if the ConfigMap already exists, if not create a new one
	cm := &corev1.ConfigMap{}
	if err := r.Get(ctx, types.NamespacedName{Name: "hashed-secrets-stalesecretwatch", Namespace: "default"}, cm); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("ConfigMap not found, creating new one")
			newCM, err := r.configMapForStaleSecretWatch(&staleSecretWatch)
			if err != nil {
				logger.Error(err, "Failed to define new ConfigMap for StaleSecretWatch")
				r.Recorder.Event(&staleSecretWatch, "Warning", "ConfigMapCreationFailed", "Failed to define new ConfigMap")
				return ctrl.Result{}, err
			}

			if err := r.Create(ctx, newCM); err != nil {
				logger.Error(err, "Failed to create new ConfigMap")
				r.Recorder.Event(&staleSecretWatch, "Warning", "ConfigMapCreationFailed", "Failed to create new ConfigMap")
				return ctrl.Result{}, err
			}
			r.Recorder.Event(&staleSecretWatch, "Normal", "ConfigMapCreated", "New ConfigMap created successfully")
			if err := r.updateStatusCondition(ctx, &staleSecretWatch, "ConfigMapCreated", metav1.ConditionTrue, "ConfigMapCreated", "New ConfigMap created successfully for StaleSecretWatch"); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true}, nil
		}
		logger.Error(err, "Failed to get ConfigMap")
		r.Recorder.Event(&staleSecretWatch, "Warning", "ConfigMapFetchFailed", "Failed to fetch ConfigMap")
		return ctrl.Result{}, err
	}

	//now prepare the namespace and secret list to watch
	secretsToWatch, err := r.prepareWatchList(ctx, logger, &staleSecretWatch)
	if err != nil {
		logger.Error(err, "Failed to prepare watch list")
		r.Recorder.Event(&staleSecretWatch, "Warning", "PrepareWatchlistFailed", "Failed to prepare watch list")
		return ctrl.Result{}, err
	}
	if err := r.updateStatusCondition(ctx, &staleSecretWatch, "PrepareWatchlist", metav1.ConditionTrue, "PrepareWatchlist", "Successfully prepared watch list for StaleSecretWatch"); err != nil {
		return ctrl.Result{}, err
	}
	logger.Info("Monitoring namespaces and secrets", "secrets", secretsToWatch)

	// Refetch the ConfigMap right before updating it to ensure it's the latest version
	err = r.Get(ctx, types.NamespacedName{Name: "hashed-secrets-stalesecretwatch", Namespace: "default"}, cm)
	if err != nil {
		logger.Error(err, "Failed to refetch the ConfigMap")
		return ctrl.Result{}, err
	}

	// calculateAndStoreHashedSecrets will calculate the secret's data hash and will
	// store in configmap
	herr := r.calculateAndStoreHashedSecrets(ctx, logger, secretsToWatch, cm)
	if herr != nil {
		logger.Error(herr, "calculateAndStoreHashedSecrets error")
		r.Recorder.Event(&staleSecretWatch, "Warning", "HashedSecretsCalculationFailed", "Failed to calculate and store hashed secrets")
		return ctrl.Result{}, err
	}

	r.Recorder.Event(&staleSecretWatch, "Normal", "ReconcileComplete", "Reconciliation complete")
	if err := r.updateStatusCondition(ctx, &staleSecretWatch, "ReconcileComplete", metav1.ConditionTrue, "Success", "Reconciliation process completed successfully"); err != nil {
		return ctrl.Result{}, err
	}

	// Check for daily tasks and possibly requeue
	dailyDone, result, err := r.performDailyChecks(ctx, logger, &staleSecretWatch)
	if err != nil {
		return result, err
	}
	if dailyDone {
		return result, nil
	}

	logger.Info("Regular reconciliation complete, requeue after 10 minutes")
	return ctrl.Result{RequeueAfter: 10 * time.Minute}, nil
}

// removeFinalizer handles deletion and finalizers
func (r *StaleSecretWatchReconciler) removeFinalizer(ctx context.Context, logger logr.Logger, staleSecretWatch *securityv1beta1.StaleSecretWatch, req *ctrl.Request) (ctrl.Result, error) {
	if controllerutil.ContainsFinalizer(staleSecretWatch, stalesecretwatchFinalizer) {
		logger.Info("Performing Finalizer Operations")
		r.doFinalizerOperationsForStaleSecretWatch(staleSecretWatch)
		r.Recorder.Event(staleSecretWatch, "Normal", "FinalizerOpsComplete", "Finalizer operations completed")

		// Retry logic to handle conflicts
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			// Refetch the latest object to minimize the window for conflicts
			if getErr := r.Get(ctx, req.NamespacedName, staleSecretWatch); getErr != nil {
				logger.Error(getErr, "Failed to fetch StaleSecretWatch for finalizer removal")
				return getErr
			}

			logger.Info("Removing Finalizer for staleSecretWatch after successfully perform the operations")
			controllerutil.RemoveFinalizer(staleSecretWatch, stalesecretwatchFinalizer)
			updateErr := r.Update(ctx, staleSecretWatch)
			if updateErr != nil {
				logger.Error(updateErr, "Failed to remove finalizer")
				r.Recorder.Event(staleSecretWatch, "Warning", "FinalizerRemovalFailed", "Failed to remove finalizer")
			}
			return updateErr
		})

		if err != nil {
			return ctrl.Result{}, err
		}

		r.Recorder.Event(staleSecretWatch, "Normal", "FinalizerRemoved", "Finalizer removed, resource cleanup complete")
		return ctrl.Result{}, nil
	}
	return ctrl.Result{}, nil
}

// addFinalizer handles adding finalizers
func (r *StaleSecretWatchReconciler) addFinalizer(ctx context.Context, logger logr.Logger, staleSecretWatch *securityv1beta1.StaleSecretWatch, req *ctrl.Request) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(staleSecretWatch, stalesecretwatchFinalizer) {
		logger.Info("Adding Finalizer for staleSecretWatch")
		// Refetch latest before adding of finalizer
		if err := r.Get(ctx, req.NamespacedName, staleSecretWatch); err != nil {
			logger.Error(err, "Failed to fetch StaleSecretWatch before finalizer removal")
			return ctrl.Result{}, err
		}
		controllerutil.AddFinalizer(staleSecretWatch, stalesecretwatchFinalizer)
		if err := r.Update(ctx, staleSecretWatch); err != nil {
			logger.Error(err, "Failed to add finalizer")
			r.Recorder.Event(staleSecretWatch, "Warning", "FinalizerAdditionFailed", "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		r.Recorder.Event(staleSecretWatch, "Normal", "FinalizerAdded", "Finalizer added to CR successfully")
		if err := r.updateStatusCondition(ctx, staleSecretWatch, "FinalizerAdded", metav1.ConditionTrue, "FinalizerAdded", "Finalizer added successfully to StaleSecretWatch"); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *StaleSecretWatchReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("stale-secret-watch")
	// For defines the type of Object being *reconciled*, and configures the ControllerManagedBy to respond to create / delete /
	// update events by *reconciling the object*.
	// This is the equivalent of calling
	// Watches(&source.Kind{Type: apiType}, &handler.EnqueueRequestForObject{}).
	mapFunc := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, a client.Object) []reconcile.Request {
		var requests []reconcile.Request
		list := &securityv1beta1.StaleSecretWatchList{}
		if err := mgr.GetClient().List(context.Background(), list); err != nil {
			log.Log.Error(err, "Failed to list StaleSecretWatch for reconciling Secret changes")
			return nil
		}
		for _, item := range list.Items {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      item.Name,
					Namespace: item.Namespace,
				},
			})
		}
		return requests
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&securityv1beta1.StaleSecretWatch{}).
		Owns(&corev1.ConfigMap{}, builder.OnlyMetadata).
		Watches(&corev1.Secret{}, mapFunc).
		Complete(r)
}

// updateStatusCondition updates the status of staleSecretWatch resource
func (r *StaleSecretWatchReconciler) updateStatusCondition(ctx context.Context, staleSecretWatch *securityv1beta1.StaleSecretWatch, conditionType string, status metav1.ConditionStatus, reason, message string) error {
	updateFunc := func() error {
		// Fetch the latest version of the resource
		latest := &securityv1beta1.StaleSecretWatch{}
		if err := r.Get(ctx, client.ObjectKey{Name: staleSecretWatch.Name, Namespace: staleSecretWatch.Namespace}, latest); err != nil {
			return err
		}

		// Update status conditions on the latest version of the resource
		meta.SetStatusCondition(&latest.Status.Conditions, metav1.Condition{
			Type:    conditionType,
			Status:  status,
			Reason:  reason,
			Message: message,
		})

		// Try to update
		return r.Status().Update(ctx, latest)
	}

	// Attempt to update with a retry on conflict
	err := retry.OnError(retry.DefaultRetry, apierrors.IsConflict, updateFunc)
	if err != nil {
		log.FromContext(ctx).Error(err, "Failed to update StaleSecretWatch status with retry")
		return err
	}

	return nil
}

// updateSecretStatuses updates SecretStatus struct
func (r *StaleSecretWatchReconciler) updateSecretStatuses(logger logr.Logger, cm *corev1.ConfigMap, staleSecretWatch *securityv1beta1.StaleSecretWatch) error {
	var configData ConfigData
	if err := json.Unmarshal(cm.BinaryData["data"], &configData); err != nil {
		logger.Error(err, "Failed to decode ConfigMap data")
		return fmt.Errorf("failed to decode ConfigMap data: %v", err)
	}

	currentTime := time.Now().UTC()
	staleThreshold := time.Duration(staleSecretWatch.Spec.StaleThresholdInDays) * 24 * time.Hour // convert days to hours

	for _, namespace := range configData.Namespaces {
		for _, secret := range namespace.Secrets {
			created, err := ParseTime(secret.Created)
			if err != nil {
				return err
			}
			lastModifiedTime, err := ParseTime(secret.LastModified)
			if err != nil {
				return err
			}
			// logger.Info("Time details",
			// 	"currentTime", currentTime,
			// 	"lastModifiedTime", lastModifiedTime,
			// 	"timeSinceLastModified", currentTime.Sub(lastModifiedTime),
			// 	"staleThreshold", staleThreshold,
			// 	"conditionResult", currentTime.Sub(lastModifiedTime) > staleThreshold)
			if currentTime.Sub(lastModifiedTime).Abs() > staleThreshold.Abs() {
				secretStatus := securityv1beta1.SecretStatus{
					Namespace:    namespace.Name,
					Name:         secret.Name,
					IsStale:      true,
					SecretType:   secret.Type,
					Created:      metav1.Time{Time: created},
					LastModified: metav1.Time{Time: lastModifiedTime},
					Message:      fmt.Sprintf("This secret is considered stale because it has not been modified/updated in over %d days.", staleSecretWatch.Spec.StaleThresholdInDays),
				}
				staleSecretWatch.Status.SecretStatus = append(staleSecretWatch.Status.SecretStatus, secretStatus)
				staleSecretWatch.Status.StaleSecretsCount++
			}
		}
	}
	return nil
}

// doFinalizerOperationsForStaleSecretWatch will perform the required operations before delete the CR.
func (r *StaleSecretWatchReconciler) doFinalizerOperationsForStaleSecretWatch(ssw *securityv1beta1.StaleSecretWatch) {
	// TODO(user): Add the cleanup steps that the operator
	// needs to do before the CR can be deleted. Examples
	// of finalizers include performing backups and deleting
	// resources that are not owned by this CR, like a PVC.

	// The following implementation will raise an event
	r.Recorder.Event(ssw, "Warning", "Deleting", fmt.Sprintf("Custom Resource %s is being deleted from the namespace %s", ssw.Name, ssw.Namespace))

}

// configMapForStaleSecretWatch returns a new stalesecretwatch ConfigMap object
func (r *StaleSecretWatchReconciler) configMapForStaleSecretWatch(ssw *securityv1beta1.StaleSecretWatch) (*corev1.ConfigMap, error) {
	ls := LabelsForStaleSecretWatchConfigMap(ssw.Name)
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hashed-secrets-stalesecretwatch", // Name of the ConfigMap
			Namespace: "default",
			Labels:    ls,
		},
		Data: map[string]string{},
	}
	// ** IMPORTANT **
	// this CR will manage/owner of this configmap resource
	// remove this, if you want above cm resource remains in the cluster
	// even after the deletion of its owning StaleSecretWatch resource.
	// In such case, when a StaleSecretWatch resource gets deleted and recreated later
	// the new StaleSecretWatch resource would get that and append latest data in it.
	// uncomment "ensureConfigMapExists" function and call it in Reconcile loop
	// after r.addFinalizer(ctx, logger, &staleSecretWatch, &req)
	// and modify the Reconcile code accordingly
	if err := ctrl.SetControllerReference(ssw, cm, r.Scheme); err != nil {
		return nil, err
	}
	return cm, nil
}

// calculateAndStoreHashedSecrets retrives the secret data, calculate the hash and update that into configmap
func (r *StaleSecretWatchReconciler) calculateAndStoreHashedSecrets(ctx context.Context, logger logr.Logger, secretsToWatch map[string][]string, cm *corev1.ConfigMap) error {
	var configData ConfigData

	// Check if ConfigMap exists and load existing data if it does
	err := r.Get(ctx, types.NamespacedName{Name: cm.Name, Namespace: cm.Namespace}, cm)
	if err == nil && len(cm.BinaryData["data"]) > 0 {
		// Unmarshal existing data into configData
		if err := json.Unmarshal(cm.BinaryData["data"], &configData); err != nil {
			logger.Error(err, "Failed to decode ConfigMap data")
			return fmt.Errorf("failed to decode ConfigMap data: %v", err)
		}
	} else if err != nil && !apierrors.IsNotFound(err) {
		logger.Error(err, "Failed to get existing ConfigMap")
		return fmt.Errorf("failed to get ConfigMap: %v", err)
	}

	// Iterate over namespaces and secrets to calculate new data
	updated := false
	for namespaceName, secrets := range secretsToWatch {
		for _, secretName := range secrets {
			secret, err := r.getSecret(ctx, namespaceName, secretName)
			if err != nil {
				logger.Error(err, "Failed to get secret", "namespace", namespaceName, "name", secretName)
				return fmt.Errorf("failed to get secret %s in namespace %s: %v", secretName, namespaceName, err)
			}

			newHash := CalculateHash(secret.Data)
			newLastModified := RetrieveModifiedTime(secret.ObjectMeta)

			if existingSecret := secretDataExists(&configData, namespaceName, secretName); existingSecret != nil {
				updateOrAppendSecretData(existingSecret, newHash, newLastModified)
				r.Recorder.Event(cm, "Normal", "SecretUpdated", fmt.Sprintf("Updated existing secret data: %s/%s", namespaceName, secretName))
				updated = true
			} else {
				addSecretData(&configData, namespaceName, secretName, newHash, secret.CreationTimestamp.Time.UTC().Format(time.RFC3339), newLastModified, string(secret.Type))
				r.Recorder.Event(cm, "Normal", "SecretAdded", fmt.Sprintf("Added new secret data: %s/%s", namespaceName, secretName))
				updated = true
			}
		}
	}

	if updated {
		// Encode updated ConfigData to JSON
		jsonData, err := json.Marshal(configData)
		if err != nil {
			logger.Error(err, "Failed to encode ConfigData to JSON")
			return fmt.Errorf("failed to encode ConfigData to JSON: %v", err)
		}

		// Store updated JSON data in ConfigMap
		cm.BinaryData = map[string][]byte{"data": jsonData}
		if err := r.createOrUpdateConfigMap(ctx, cm); err != nil {
			logger.Error(err, "Failed to create or update ConfigMap")
			return fmt.Errorf("failed to create or update ConfigMap: %v", err)
		}
		r.Recorder.Event(cm, "Normal", "ConfigMapUpdated", "ConfigMap updated successfully")
		logger.Info("ConfigMap updated successfully")
	} else {
		logger.Info("No updates necessary for ConfigMap")
		r.Recorder.Event(cm, "Normal", "NoUpdateNeeded", "No updates made to ConfigMap")
	}

	return nil
}

// updateOrAppendSecretData will append new data hash to history
func updateOrAppendSecretData(secret *Secret, newHash, lastModified string) {
	found := false
	for _, h := range secret.History {
		if h.Data == newHash {
			found = true
			break
		}
	}
	if !found {
		secret.History = append(secret.History, History{Data: newHash})
	}
	secret.LastModified = lastModified
}

// secretDataExists checks whether hash data for perticular secret data exists or not
func secretDataExists(configData *ConfigData, namespace, name string) *Secret {
	for i, ns := range configData.Namespaces {
		if ns.Name == namespace {
			for j := range ns.Secrets {
				if ns.Secrets[j].Name == name {
					return &configData.Namespaces[i].Secrets[j]
				}
			}
		}
	}
	return nil
}

// addSecretData adds new secret data hash to history
func addSecretData(configData *ConfigData, namespace, name string, newHash, created, lastModified, secretType string) {
	newSecret := Secret{
		Name:         name,
		Created:      created,
		LastModified: lastModified,
		History:      []History{{Data: newHash}},
		Type:         secretType,
	}

	for i, ns := range configData.Namespaces {
		if ns.Name == namespace {
			configData.Namespaces[i].Secrets = append(configData.Namespaces[i].Secrets, newSecret)
			return
		}
	}

	// If namespace not found, add new namespace with the secret
	configData.Namespaces = append(configData.Namespaces, Namespace{
		Name:    namespace,
		Secrets: []Secret{newSecret},
	})
}

// getSecret gets secret data
func (r *StaleSecretWatchReconciler) getSecret(ctx context.Context, namespace, name string) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, secret)
	if err != nil {
		return secret, err
	}
	return secret, nil
}

// createOrUpdateConfigMap creates or updates configmap
func (r *StaleSecretWatchReconciler) createOrUpdateConfigMap(ctx context.Context, cm *corev1.ConfigMap) error {
	found := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: cm.Name, Namespace: cm.Namespace}, found)
	if err != nil && apierrors.IsNotFound(err) {
		// ConfigMap does not exist, create it
		err = r.Create(ctx, cm)
		if err != nil {
			return fmt.Errorf("failed to create ConfigMap: %v", err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to check if ConfigMap exists: %v", err)
	}

	// ConfigMap exists, update it
	found = found.DeepCopy()
	found.BinaryData = cm.BinaryData
	err = r.Update(ctx, found)
	if err != nil {
		return fmt.Errorf("failed to update ConfigMap: %v", err)
	}

	return nil
}

// prepareWatchList prepares the list of secret resources present inside the namespace, based on the data provided in yaml file
func (r *StaleSecretWatchReconciler) prepareWatchList(ctx context.Context, logger logr.Logger, ssw *securityv1beta1.StaleSecretWatch) (map[string][]string, error) {
	var namespacesToWatch []string
	secretsToWatch := make(map[string][]string)

	// If watching all namespaces, list all namespaces and add them to namespacesToWatch
	if ssw.Spec.StaleSecretToWatch.Namespace == "all" {
		nsList := &corev1.NamespaceList{}
		if err := r.List(ctx, nsList); err != nil {
			return nil, fmt.Errorf("failed to list namespaces: %v", err)
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
		if err := r.List(ctx, secretList, client.InNamespace(ns)); err != nil || apierrors.IsNotFound(err) {
			logger.Info("secret resources not found in " + ns)
			return nil, fmt.Errorf("failed to list secrets in namespace %s: %v", ns, err)
		}
		for _, secret := range secretList.Items {
			// Check if this secret is in the excludeList for its namespace
			// also exclude secret.Type == "kubernetes.io/service-account-token" & "bootstrap.kubernetes.io/token" as per business logic
			/*
				    Infrequently changing secrets:
					- kubernetes.io/service-account-token: These Secrets are
					automatically generated by Kubernetes for each ServiceAccount to provide
					access tokens for authenticating with the Kubernetes API. Service account
					tokens are typically long-lived and used for authentication purposes, so
					they don't change frequently unless the ServiceAccount itself is modified
					or deleted.
					- bootstrap.kubernetes.io/token: Bootstrap tokens are used during the
					initial setup of Kubernetes clusters to authenticate joining nodes. Once
					generated, bootstrap tokens are typically not modified frequently unless
					there's a need to create new clusters or add new nodes.

			*/
			if secret.Type == "kubernetes.io/service-account-token" || secret.Type == "bootstrap.kubernetes.io/token" {
				continue // Skip excluded types
			}
			excluded := false
			for _, excludeEntry := range ssw.Spec.StaleSecretToWatch.ExcludeList {
				if excludeEntry.Namespace == ns && Contains(excludeEntry.SecretName, secret.Name) {
					excluded = true
					break
				}
			}
			if !excluded {
				secretsToWatch[ns] = append(secretsToWatch[ns], secret.Name)
			}
		}
	}
	r.Recorder.Event(ssw, "Normal", "prepared list of secrets", "list of secrets present in different namespaces prepared for watch")

	return secretsToWatch, nil

}

// performDailyChecks will handle the logic needed to perform the daily secret status updates and then schedule the next run
func (r *StaleSecretWatchReconciler) performDailyChecks(ctx context.Context, logger logr.Logger, staleSecretWatch *securityv1beta1.StaleSecretWatch) (bool, ctrl.Result, error) {
	logger.Info("Entering performDailyChecks ...")
	lastCheckedStr, ok := staleSecretWatch.Annotations["lastChecked"]
	if !ok {
		// If the annotation is not found, proceed with the daily checks
		lastCheckedStr = "2024-01-01T00:00:00Z" // Use epoch start if not set
	}
	lastChecked, err := time.Parse(time.RFC3339, lastCheckedStr)
	if err != nil {
		logger.Error(err, "Failed to parse last checked time, proceeding with daily checks")
		lastChecked = time.Unix(0, 0) // Default to epoch start on error
	}
	if time.Since(lastChecked) < 24*time.Hour {
		logger.Info("Daily checks already performed today, skipping...")
		return false, ctrl.Result{RequeueAfter: 24*time.Hour - time.Since(lastChecked)}, nil
	}
	// Update the last checked annotation before running the daily checks
	if err := r.updateLastCheckedAnnotation(ctx, staleSecretWatch); err != nil {
		return false, ctrl.Result{}, err
	}

	currentTime := time.Now().UTC()
	//staleSecretWatch.Annotations

	//if currentTime.Hour() == 9 && currentTime.Minute() < 30 { // Checking within a 30-minute window after 9 AM
	//if currentTime.Weekday() > 0 && currentTime.Weekday() < 6 {
	if currentTime.Hour() >= 10 && currentTime.Hour() <= 20 {
		staleSecretWatch.Status.SecretStatus = []securityv1beta1.SecretStatus{}
		staleSecretWatch.Status.StaleSecretsCount = 0
		cm := &corev1.ConfigMap{}
		if err := r.Get(ctx, types.NamespacedName{Name: "hashed-secrets-stalesecretwatch", Namespace: "default"}, cm); err != nil {
			logger.Error(err, "Failed to fetch the latest ConfigMap")
			return true, ctrl.Result{}, err
		}

		if err := r.updateSecretStatuses(logger, cm, staleSecretWatch); err != nil {
			logger.Error(err, "Failed to update secret statuses")
			return true, ctrl.Result{}, err
		}

		if err := r.NotifySlack(ctx, logger, staleSecretWatch); err != nil {
			logger.Error(err, "Failed to notify Slack")
			if err == errors.New("SLACK_BOT_TOKEN or SLACK_CHANNEL_ID is not set") {
				return false, ctrl.Result{Requeue: false}, nil
			}
			return true, ctrl.Result{}, err
		}

		// Fetch the latest version to avoid update conflicts
		latest := &securityv1beta1.StaleSecretWatch{}
		if err := r.Get(ctx, types.NamespacedName{Name: staleSecretWatch.Name, Namespace: staleSecretWatch.Namespace}, latest); err != nil {
			logger.Error(err, "Failed to refetch StaleSecretWatch")
			return true, ctrl.Result{}, err
		}

		// Apply the updates from staleSecretWatch to the latest fetched version
		latest.Status.SecretStatus = staleSecretWatch.Status.SecretStatus
		latest.Status.StaleSecretsCount = staleSecretWatch.Status.StaleSecretsCount

		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			return r.Status().Update(ctx, latest)
		})

		if err != nil {
			logger.Error(err, "Failed to update StaleSecretWatch status with retry")
			return true, ctrl.Result{}, err
		}

		//logger.Info("Updated SecretStatus post-status update", "SecretStatus", staleSecretWatch.Status.SecretStatus)

		// Schedule the next check for the following day at the same time
		nextCheck := time.Now().Add(24 * time.Hour)
		requeueAfter := time.Until(nextCheck)
		logger.Info("Daily check performed, requeue scheduled", "RequeueAfter", requeueAfter)
		return true, ctrl.Result{RequeueAfter: requeueAfter}, nil

		// requeueAfter := GetNextNineAMUTC()
		// requeueAfter := GetNextFiveMinutes()
		// logger.Info("Daily check performed, requeue scheduled", "RequeueAfter", requeueAfter)
		// return true, ctrl.Result{RequeueAfter: requeueAfter}, nil
	} else if currentTime.Weekday() == 0 || currentTime.Weekday() == 6 {
		logger.Info("It's the weekend. PerformDailyCheck will run over the coming weekdays.")
	} else {
		logger.Info("This is outside the notified time. PerformanceDailyCheck will run on the next schedule.")
	}

	logger.Info("It is not time for daily checks yet, skipping...")
	// Indicate no requeue needed for daily checks, continue with other tasks
	return false, ctrl.Result{RequeueAfter: time.Until(time.Date(currentTime.Year(), currentTime.Month(), currentTime.Day(), 9, 0, 0, 0, time.UTC))}, nil
	//return false, ctrl.Result{}, nil
}

// updateLastCheckedAnnotation Update the last checked annotation before running the daily checks
func (r *StaleSecretWatchReconciler) updateLastCheckedAnnotation(ctx context.Context, ssw *securityv1beta1.StaleSecretWatch) error {
	if ssw.Annotations == nil {
		ssw.Annotations = make(map[string]string)
	}

	// Set the current time as the last checked time
	ssw.Annotations["lastChecked"] = time.Now().Format(time.RFC3339)

	// Update the custom resource
	err := r.Update(ctx, ssw)
	if err != nil {
		log.FromContext(ctx).Error(err, "Failed to update StaleSecretWatch annotations")
		return err
	}
	return nil
}

/*
// ensureConfigMapExists Checks if cm exists first, and if it does not, then create it.
func (r *StaleSecretWatchReconciler) ensureConfigMapExists(ctx context.Context, logger logr.Logger, ssw *securityv1beta1.StaleSecretWatch) (*corev1.ConfigMap, error) {
    cm := &corev1.ConfigMap{}
    err := r.Get(ctx, types.NamespacedName{Name: "hashed-secrets-stalesecretwatch", Namespace: "default"}, cm)
    if err != nil && apierrors.IsNotFound(err) {
        logger.Info("ConfigMap not found, creating new one")
        cm, err = r.configMapForStaleSecretWatch(ssw)
        if err != nil {
            return nil, fmt.Errorf("failed to define new ConfigMap for StaleSecretWatch: %w", err)
        }

        if err := r.Create(ctx, cm); err != nil {
            return nil, fmt.Errorf("failed to create new ConfigMap: %w", err)
        }
        logger.Info("New ConfigMap created successfully")
    } else if err != nil {
        return nil, fmt.Errorf("failed to get ConfigMap: %w", err)
    }

    return cm, nil
}

*/
