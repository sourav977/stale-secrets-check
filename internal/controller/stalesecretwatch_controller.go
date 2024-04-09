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
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	typeAvailable             = "Available"
	typeDegraded              = "Degraded"
	typeUnavailable           = "Unavailable"
	errGetSSW                 = "could not get StaleSecretWatch"
	errNSnotEmpty             = "staleSecretToWatch.namespace cannot be empty"
	stalesecretwatchFinalizer = "security.stalesecretwatch.io/finalizer"
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
		if apierrors.IsNotFound(err) {
			logger.Info("StaleSecretWatch resource not found. Ignoring since StaleSecretWatch object must be deleted. Exit Reconcile.")
			// Object not found, return without requeueing
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
		// Error fetching the StaleSecretWatch instance, requeue the request
		logger.Error(err, errGetSSW)
		return ctrl.Result{}, err
	}

	// Let's just set the status as Unknown when no status are available
	if staleSecretWatch.Status.Conditions == nil || len(staleSecretWatch.Status.Conditions) == 0 {
		meta.SetStatusCondition(&staleSecretWatch.Status.Conditions, metav1.Condition{Type: typeUnavailable, Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting reconciliation"})
		r.Recorder.Event(&staleSecretWatch, "Normal", typeUnavailable, "No CR of type StaleSecretWatch currently available")
		if err := r.Status().Update(ctx, &staleSecretWatch); err != nil {
			logger.Error(err, "Failed to update staleSecretWatch status")
			return ctrl.Result{}, err
		}

		// Let's re-fetch the staleSecretWatch Custom Resource after update the status
		// so that we have the latest state of the resource on the cluster and we will avoid
		// raise the issue "the object has been modified, please apply
		// your changes to the latest version and try again" which would re-trigger the reconciliation
		// if we try to update it again in the following operations
		if err := r.Get(ctx, req.NamespacedName, &staleSecretWatch); err != nil {
			logger.Error(err, "Failed to re-fetch staleSecretWatch")
			return ctrl.Result{}, err
		}
	}

	// Let's add a finalizer. Then, we can define some operations which should
	// occurs before the custom resource to be deleted.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/finalizers
	if !controllerutil.ContainsFinalizer(&staleSecretWatch, stalesecretwatchFinalizer) {
		logger.Info("Adding Finalizer for staleSecretWatch")
		if ok := controllerutil.AddFinalizer(&staleSecretWatch, stalesecretwatchFinalizer); !ok {
			logger.Error(apierrors.FromObject(&staleSecretWatch), "Failed to add finalizer into the custom resource")
			return ctrl.Result{Requeue: true}, nil
		}

		if err := r.Update(ctx, &staleSecretWatch); err != nil {
			logger.Error(err, "Failed to update custom resource to add finalizer")
			return ctrl.Result{}, err
		}
		r.Recorder.Event(&staleSecretWatch, "Normal", "AddedFinalizer", "Finalizer added to CR")
	}

	// Check if the staleSecretWatch instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set.
	isStaleSecretWatchMarkedToBeDeleted := staleSecretWatch.GetDeletionTimestamp() != nil
	if isStaleSecretWatchMarkedToBeDeleted {
		if controllerutil.ContainsFinalizer(&staleSecretWatch, stalesecretwatchFinalizer) {
			logger.Info("Performing Finalizer Operations for staleSecretWatch before delete CR")

			// Let's add here an status "Downgrade" to define that this resource begin its process to be terminated.
			meta.SetStatusCondition(&staleSecretWatch.Status.Conditions, metav1.Condition{Type: typeDegraded,
				Status: metav1.ConditionUnknown, Reason: "Finalizing",
				Message: fmt.Sprintf("Performing finalizer operations for the custom resource: %s ", staleSecretWatch.Name)})

			if err := r.Status().Update(ctx, &staleSecretWatch); err != nil {
				logger.Error(err, "Failed to update staleSecretWatch status")
				return ctrl.Result{}, err
			}
			r.Recorder.Event(&staleSecretWatch, "Normal", "Finalizing", "downgrading status, performing finalizer operations for the custom resource")

			// Perform all operations required before remove the finalizer and allow
			// the Kubernetes API to remove the custom resource.
			r.doFinalizerOperationsForStaleSecretWatch(&staleSecretWatch)

			// TODO(user): If you add operations to the doFinalizerOperationsForStaleSecretWatch method
			// then you need to ensure that all worked fine before deleting and updating the Downgrade status
			// otherwise, you should requeue here.

			// Re-fetch the staleSecretWatch Custom Resource before update the status
			// so that we have the latest state of the resource on the cluster and we will avoid
			// raise the issue "the object has been modified, please apply
			// your changes to the latest version and try again" which would re-trigger the reconciliation
			found := securityv1beta1.StaleSecretWatch{}
			if err := r.Get(ctx, req.NamespacedName, &found); err != nil {
				logger.Error(err, "Failed to re-fetch staleSecretWatch")
				return ctrl.Result{}, err
			}

			meta.SetStatusCondition(&staleSecretWatch.Status.Conditions, metav1.Condition{Type: typeDegraded,
				Status: metav1.ConditionTrue, Reason: "Finalizing",
				Message: fmt.Sprintf("Finalizer operations for custom resource %s name were successfully accomplished", staleSecretWatch.Name)})

			if err := r.Status().Update(ctx, &found); err != nil {
				logger.Error(err, "Failed to update staleSecretWatch status")
				return ctrl.Result{}, err
			}

			logger.Info("Removing Finalizer for staleSecretWatch after successfully perform the operations")
			if ok := controllerutil.RemoveFinalizer(&staleSecretWatch, stalesecretwatchFinalizer); !ok {
				logger.Error(apierrors.FromObject(&staleSecretWatch), "Failed to remove finalizer for staleSecretWatch")
				return ctrl.Result{Requeue: true}, nil
			}

			if err := r.Update(ctx, &found); err != nil {
				logger.Error(err, "Failed to remove finalizer for staleSecretWatch")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// This will allow for the StaleSecretWatch to be reconciled when changes to the Secret are noticed.
	// var secret corev1.Secret
	// if err := controllerutil.SetControllerReference(&staleSecretWatch, &secret, r.Scheme); err != nil {
	// 	return ctrl.Result{}, err
	// }

	// Validation passed, it must be validated by webhook, now create the StaleSecretWatch resource
	// if err := r.createOrUpdateStaleSecretWatch(ctx, logger, &staleSecretWatch); err != nil {
	// 	return ctrl.Result{}, err
	// }

	// meta.SetStatusCondition(&staleSecretWatch.Status.Conditions, metav1.Condition{Type: staleSecretWatch.Kind, Status: metav1.ConditionTrue, Reason: "Reconciled", Message: "StaleSecretWatch resource created"})
	// if err := r.Status().Update(ctx, &staleSecretWatch); err != nil {
	// 	logger.Error(err, "Failed to update StaleSecretWatch status")
	// 	return ctrl.Result{}, err
	// }

	// Check if the ConfigMap already exists, if not create a new one
	cm := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: "hashed-secrets-stalesecretwatch", Namespace: "default"}, cm)
	if err != nil && apierrors.IsNotFound(err) {
		// Define a new ConfigMap
		scm, err := r.configMapForStaleSecretWatch(&staleSecretWatch)
		if err != nil {
			logger.Error(err, "Failed to define new ConfigMap resource for StaleSecretWatch")

			// The following implementation will update the status
			meta.SetStatusCondition(&staleSecretWatch.Status.Conditions, metav1.Condition{Type: typeAvailable,
				Status: metav1.ConditionFalse, Reason: "Reconciling",
				Message: fmt.Sprintf("Failed to create ConfigMap for the custom resource (%s): (%s)", staleSecretWatch.Name, err)})

			if err := r.Status().Update(ctx, &staleSecretWatch); err != nil {
				logger.Error(err, "Failed to update StaleSecretWatch status")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}

		logger.Info("Creating a new ConfigMap",
			"ConfigMap.Namespace", scm.Namespace, "ConfigMap.Name", scm.Name)
		if err = r.Create(ctx, scm); err != nil {
			logger.Error(err, "Failed to create new ConfigMap",
				"ConfigMap.Namespace", scm.Namespace, "ConfigMap.Name", scm.Name)
			return ctrl.Result{}, err
		}
		r.Recorder.Event(cm, "Normal", typeAvailable, "Configmap "+cm.Name+"created in default namespace")

		// ConfigMap created successfully
		// We will requeue the reconciliation so that we can ensure the state
		// and move forward for the next operations
		return ctrl.Result{}, nil
	} else if err != nil {
		logger.Error(err, "Failed to get ConfigMap")
		// Let's return the error for the reconciliation be re-trigged again
		return ctrl.Result{}, err
	}

	//now prepare the namespace and secret list to watch
	secretsToWatch, err := r.prepareWatchList(ctx, logger, &staleSecretWatch)
	if err != nil {
		logger.Error(err, "Failed to prepare watch list")
		return ctrl.Result{}, err
	}
	logger.Info("Monitoring namespaces and secrets", "secrets", secretsToWatch)

	herr := r.calculateAndStoreHashedSecrets(ctx, secretsToWatch, cm)
	if herr != nil {
		logger.Error(herr, "calculateAndStoreHashedSecrets error")
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
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

// finalizeStaleSecretWatch will perform the required operations before delete the CR.
func (r *StaleSecretWatchReconciler) doFinalizerOperationsForStaleSecretWatch(ssw *securityv1beta1.StaleSecretWatch) {
	// TODO(user): Add the cleanup steps that the operator
	// needs to do before the CR can be deleted. Examples
	// of finalizers include performing backups and deleting
	// resources that are not owned by this CR, like a PVC.

	// The following implementation will raise an event
	r.Recorder.Event(ssw, "Warning", "Deleting",
		fmt.Sprintf("Custom Resource %s is being deleted from the namespace %s",
			ssw.Name,
			ssw.Namespace))
}

// deploymentForMemcached returns a Memcached Deployment object
func (r *StaleSecretWatchReconciler) configMapForStaleSecretWatch(ssw *securityv1beta1.StaleSecretWatch) (*corev1.ConfigMap, error) {
	ls := labelsForStaleSecretWatchConfigMap(ssw.Name)
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hashed-secrets-stalesecretwatch", // Name of the ConfigMap
			Namespace: "default",
			Labels:    ls,
		},
		Data: map[string]string{},
	}
	// Set the ownerRef for the Deployment
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/owners-dependents/
	if err := ctrl.SetControllerReference(ssw, cm, r.Scheme); err != nil {
		return nil, err
	}
	return cm, nil
}

// labelsForMemcached returns the labels for selecting the resources
// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/
func labelsForStaleSecretWatchConfigMap(name string) map[string]string {
	return map[string]string{"app.kubernetes.io/name": "StaleSecretWatch",
		"app.kubernetes.io/instance":   name,
		"app.kubernetes.io/part-of":    "StaleSecretWatch-operator",
		"app.kubernetes.io/created-by": "controller-manager-stalesecretwatch",
	}
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

func (r *StaleSecretWatchReconciler) calculateAndStoreHashedSecrets(ctx context.Context, secretsToWatch map[string][]string, cm *corev1.ConfigMap) error {
	configData := ConfigData{
		Namespaces: make(map[string]Namespace),
	}

	for namespace, secrets := range secretsToWatch {
		namespaceData := Namespace{
			Secrets: make(map[string]Secret),
		}
		var count = 0
		for _, secretName := range secrets {
			secret, err := r.getSecret(ctx, namespace, secretName)
			if err != nil {
				return fmt.Errorf("failed to get secret %s in namespace %s: %v", secretName, namespace, err)
			}
			count++
			// Calculate hash of secret data
			hash := sha256.Sum256(secret.Data["data"])

			// Construct Version struct
			version := Version{
				Data:     hex.EncodeToString(hash[:]),
				Created:  secret.CreationTimestamp.Time.UTC().Format(time.RFC3339),
				Modified: secret.ObjectMeta.GetCreationTimestamp().Time.UTC().Format(time.RFC3339),
			}
			key := "version" + strconv.Itoa(count)
			// Construct Secret struct
			secretData := Secret{
				Versions: map[string]Version{
					key: version,
				},
			}

			// Store secret data in Namespace struct
			namespaceData.Secrets[secretName] = secretData
		}

		// Store namespace data in ConfigData struct
		configData.Namespaces[namespace] = namespaceData
	}

	// Encode ConfigData struct to JSON
	jsonData, err := r.marshalConfigData(configData)
	if err != nil {
		return fmt.Errorf("failed to encode ConfigData to JSON: %v", err)
	}

	// Store JSON data in ConfigMap
	cm.BinaryData = map[string][]byte{"data": jsonData}

	// Create or update ConfigMap
	if err := r.CreateOrUpdateConfigMap(ctx, cm); err != nil {
		return fmt.Errorf("failed to create or update ConfigMap: %v", err)
	}

	return nil
}

func (r *StaleSecretWatchReconciler) getSecret(ctx context.Context, namespace, name string) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, secret)
	if err != nil {
		return nil, err
	}
	return secret, nil
}

func (r *StaleSecretWatchReconciler) marshalConfigData(configData ConfigData) ([]byte, error) {
	jsonData, err := json.Marshal(configData)
	if err != nil {
		return nil, fmt.Errorf("failed to encode ConfigData to JSON: %v", err)
	}
	return jsonData, nil
}

func (r *StaleSecretWatchReconciler) CreateOrUpdateConfigMap(ctx context.Context, cm *corev1.ConfigMap) error {
	found := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: cm.Name, Namespace: cm.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
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
	found.BinaryData = cm.BinaryData
	err = r.Update(ctx, found)
	if err != nil {
		return fmt.Errorf("failed to update ConfigMap: %v", err)
	}

	return nil
}

// func (r *StaleSecretWatchReconciler) calculateAndStoreHashedSecrets(ctx context.Context, secrets []corev1.Secret) error {
// 	// Iterate over each secret
// 	for _, secret := range secrets {
// 		// Calculate hash of secret data
// 		hash := calculateHash(secret.Data)

// 		// Create or update ConfigMap with hashed secret data
// 		cm := &corev1.ConfigMap{
// 			ObjectMeta: metav1.ObjectMeta{
// 				Name:      "hashed-secrets-stalesecretwatch", // Name of the ConfigMap
// 				Namespace: secret.Namespace,
// 			},
// 			Data: map[string]string{
// 				secret.Name: hash,
// 			},
// 		}

// 		// Check if ConfigMap exists
// 		found := &corev1.ConfigMap{}
// 		err := r.Get(ctx, client.ObjectKey{Namespace: cm.Namespace, Name: cm.Name}, found)
// 		if err != nil {
// 			if apierrors.IsNotFound(err) {
// 				// ConfigMap does not exist, create it
// 				err = r.Create(ctx, cm)
// 				if err != nil {
// 					return err
// 				}
// 			} else {
// 				return err
// 			}
// 		} else {
// 			// ConfigMap exists, update it
// 			found.Data[secret.Name] = hash
// 			err = r.Update(ctx, found)
// 			if err != nil {
// 				return err
// 			}
// 		}
// 	}

// 	return nil
// }

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

	return secretsToWatch, nil

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
