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
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/go-logr/logr"
	securityv1beta1 "github.com/sourav977/stale-secrets-watch/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var cmData = map[string][]byte{"data": []byte(`{
	"namespaces": [
		{
			"name": "vivid2",
			"secrets": [
				{
					"name": "chef-user-secret2",
					"created": "2024-04-11T11:08:02Z",
					"last_modified": "2024-04-13T15:21:48Z",
					"history": [
						{
							"data": "7cdd5f2812f1590e12f31c127727746241459da8b8f35ce3a555e18af20316f1"
						}
					],
					"type": "Opaque"
				}
			]
		}
	]
}`)}

func Test_updateOrAppendSecretData(t *testing.T) {
	type args struct {
		secret       *Secret
		newHash      string
		lastModified string
	}
	tests := []struct {
		name           string
		args           args
		wantHistoryLen int
		wantLastMod    string
	}{
		{
			name: "Empty initial history - should append",
			args: args{
				secret:       &Secret{History: []History{}},
				newHash:      "hash1",
				lastModified: "2021-01-01T12:00:00Z",
			},
			wantHistoryLen: 1,
			wantLastMod:    "2021-01-01T12:00:00Z",
		},
		{
			name: "Non-empty history without matching hash - should append",
			args: args{
				secret: &Secret{History: []History{
					{Data: "hash2"},
				}},
				newHash:      "hash1",
				lastModified: "2021-01-02T12:00:00Z",
			},
			wantHistoryLen: 2,
			wantLastMod:    "2021-01-02T12:00:00Z",
		},
		{
			name: "Non-empty history with matching hash - should not append",
			args: args{
				secret: &Secret{History: []History{
					{Data: "hash1"},
				}},
				newHash:      "hash1",
				lastModified: "2021-01-03T12:00:00Z",
			},
			wantHistoryLen: 1, // No new hash added
			wantLastMod:    "2021-01-03T12:00:00Z",
		},
		{
			name: "Multiple items, hash exists - should not append",
			args: args{
				secret: &Secret{History: []History{
					{Data: "hash2"},
					{Data: "hash1"},
					{Data: "hash3"},
				}},
				newHash:      "hash1",
				lastModified: "2021-01-04T12:00:00Z",
			},
			wantHistoryLen: 3, // No new hash added
			wantLastMod:    "2021-01-04T12:00:00Z",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updateOrAppendSecretData(tt.args.secret, tt.args.newHash, tt.args.lastModified)
			if got := len(tt.args.secret.History); got != tt.wantHistoryLen {
				t.Errorf("updateOrAppendSecretData() got = %v, want %v for history length", got, tt.wantHistoryLen)
			}
			if got := tt.args.secret.LastModified; got != tt.wantLastMod {
				t.Errorf("updateOrAppendSecretData() got1 = %v, want %v for last modified", got, tt.wantLastMod)
			}
		})
	}
}

func Test_secretDataExists(t *testing.T) {
	// Setup
	testSecret1 := &Secret{Name: "testSecret1"}
	testSecret2 := &Secret{Name: "testSecret2"}
	type args struct {
		configData *ConfigData
		namespace  string
		name       string
	}
	tests := []struct {
		name string
		args args
		want *Secret
	}{
		{
			name: "Secret exists",
			args: args{
				configData: &ConfigData{
					Namespaces: []Namespace{
						{
							Name: "namespace1",
							Secrets: []Secret{
								*testSecret1,
								{Name: "someOtherSecret"},
							},
						},
					},
				},
				namespace: "namespace1",
				name:      "testSecret1",
			},
			want: testSecret1,
		},
		{
			name: "Secret does not exist",
			args: args{
				configData: &ConfigData{
					Namespaces: []Namespace{
						{
							Name: "namespace1",
							Secrets: []Secret{
								{Name: "someOtherSecret"},
							},
						},
					},
				},
				namespace: "namespace1",
				name:      "nonExistingSecret",
			},
			want: nil,
		},
		{
			name: "Namespace does not exist",
			args: args{
				configData: &ConfigData{
					Namespaces: []Namespace{
						{
							Name: "someOtherNamespace",
							Secrets: []Secret{
								{Name: "someOtherSecret"},
							},
						},
					},
				},
				namespace: "nonExistingNamespace",
				name:      "testSecret1",
			},
			want: nil,
		},
		{
			name: "No namespaces",
			args: args{
				configData: &ConfigData{
					Namespaces: []Namespace{},
				},
				namespace: "namespace1",
				name:      "testSecret1",
			},
			want: nil,
		},
		{
			name: "Multiple namespaces and secrets - secret exists",
			args: args{
				configData: &ConfigData{
					Namespaces: []Namespace{
						{
							Name: "namespace1",
							Secrets: []Secret{
								{Name: "someOtherSecret"},
							},
						},
						{
							Name: "namespace2",
							Secrets: []Secret{
								*testSecret2,
							},
						},
					},
				},
				namespace: "namespace2",
				name:      "testSecret2",
			},
			want: testSecret2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := secretDataExists(tt.args.configData, tt.args.namespace, tt.args.name); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("secretDataExists() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_addSecretData(t *testing.T) {
	// Setup
	existingNamespace := "existing-namespace"
	newNamespace := "new-namespace"
	existingSecret := Secret{
		Name:         "existingSecret",
		Created:      "2020-01-01T12:00:00Z",
		LastModified: "2020-01-02T12:00:00Z",
		History:      []History{{Data: "oldHash"}},
		Type:         "oldType",
	}
	type args struct {
		configData   *ConfigData
		namespace    string
		name         string
		newHash      string
		created      string
		lastModified string
		secretType   string
	}
	tests := []struct {
		name    string
		args    args
		want    *ConfigData
		wantErr bool
	}{
		{
			name: "Add secret to existing namespace",
			args: args{
				configData: &ConfigData{
					Namespaces: []Namespace{
						{
							Name:    existingNamespace,
							Secrets: []Secret{existingSecret},
						},
					},
				},
				namespace:    existingNamespace,
				name:         "newSecret",
				newHash:      "newHash",
				created:      "2021-01-01T12:00:00Z",
				lastModified: "2021-01-02T12:00:00Z",
				secretType:   "newType",
			},
			want: &ConfigData{
				Namespaces: []Namespace{
					{
						Name: existingNamespace,
						Secrets: []Secret{
							existingSecret,
							{
								Name:         "newSecret",
								Created:      "2021-01-01T12:00:00Z",
								LastModified: "2021-01-02T12:00:00Z",
								History:      []History{{Data: "newHash"}},
								Type:         "newType",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Add secret to new namespace",
			args: args{
				configData: &ConfigData{
					Namespaces: []Namespace{
						{
							Name:    existingNamespace,
							Secrets: []Secret{existingSecret},
						},
					},
				},
				namespace:    newNamespace,
				name:         "newSecret",
				newHash:      "newHash",
				created:      "2021-01-01T12:00:00Z",
				lastModified: "2021-01-02T12:00:00Z",
				secretType:   "newType",
			},
			want: &ConfigData{
				Namespaces: []Namespace{
					{
						Name:    existingNamespace,
						Secrets: []Secret{existingSecret},
					},
					{
						Name: newNamespace,
						Secrets: []Secret{
							{
								Name:         "newSecret",
								Created:      "2021-01-01T12:00:00Z",
								LastModified: "2021-01-02T12:00:00Z",
								History:      []History{{Data: "newHash"}},
								Type:         "newType",
							},
						},
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addSecretData(tt.args.configData, tt.args.namespace, tt.args.name, tt.args.newHash, tt.args.created, tt.args.lastModified, tt.args.secretType)
			if got := tt.args.configData; !reflect.DeepEqual(got, tt.want) {
				t.Errorf("addSecretData() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStaleSecretWatchReconciler_getSecret(t *testing.T) {
	ctx := context.TODO()
	namespace := "default"
	existingSecretName := "existing-secret"
	nonExistingSecretName := "non-existing-secret"

	// Setup the test environment
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      existingSecretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{"key": []byte("data")},
	}
	fclient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
	type fields struct {
		Client          client.Client
		Log             logr.Logger
		RequeueInterval time.Duration
		Scheme          *runtime.Scheme
		Recorder        record.EventRecorder
	}
	type args struct {
		ctx       context.Context
		namespace string
		name      string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *corev1.Secret
		wantErr bool
	}{
		{
			name: "Fetch existing secret",
			fields: fields{
				Client:          fclient,
				Log:             logr.Logger{},
				RequeueInterval: time.Second * 10,
				Scheme:          scheme,
				Recorder:        nil,
			},
			args: args{
				ctx:       ctx,
				namespace: namespace,
				name:      existingSecretName,
			},
			want:    secret,
			wantErr: false,
		},
		{
			name: "Try to fetch non-existing secret",
			fields: fields{
				Client:          fclient,
				Log:             logr.Logger{},
				RequeueInterval: time.Second * 10,
				Scheme:          scheme,
				Recorder:        nil,
			},
			args: args{
				ctx:       ctx,
				namespace: namespace,
				name:      nonExistingSecretName,
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &StaleSecretWatchReconciler{
				Client:          tt.fields.Client,
				Log:             tt.fields.Log,
				RequeueInterval: tt.fields.RequeueInterval,
				Scheme:          tt.fields.Scheme,
				Recorder:        tt.fields.Recorder,
			}
			got, err := r.getSecret(tt.args.ctx, tt.args.namespace, tt.args.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("StaleSecretWatchReconciler.getSecret() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got.DeepCopy() == tt.want.DeepCopy() {
				t.Errorf("StaleSecretWatchReconciler.getSecret() = %v, want %v", got, tt.want)
			}

		})
	}
}

func TestStaleSecretWatchReconciler_createOrUpdateConfigMap(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	// Existing config map
	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "existing-cm", Namespace: "default"},
		BinaryData: map[string][]byte{"data": []byte("old data")},
	}

	// New config map data
	newCMData := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "existing-cm", Namespace: "default"},
		BinaryData: map[string][]byte{"data": []byte("new data")},
	}

	// Completely new config map (does not exist in the cluster yet)
	newCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "new-cm", Namespace: "default"},
		BinaryData: map[string][]byte{"data": []byte("some data")},
	}

	// Client with existing ConfigMap
	clientWithExisting := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingCM).Build()
	// Client without any ConfigMaps
	clientWithoutCM := fake.NewClientBuilder().WithScheme(scheme).Build()

	type args struct {
		ctx context.Context
		cm  *corev1.ConfigMap
	}
	tests := []struct {
		name    string
		client  client.Client
		args    args
		wantErr bool
	}{
		{
			name:   "Create new ConfigMap",
			client: clientWithoutCM,
			args: args{
				ctx: context.TODO(),
				cm:  newCM,
			},
			wantErr: false,
		},
		{
			name:   "Update existing ConfigMap",
			client: clientWithExisting,
			args: args{
				ctx: context.TODO(),
				cm:  newCMData,
			},
			wantErr: false,
		},
		{
			name:   "Handle error on ConfigMap fetch",
			client: clientWithoutCM, // Simulate fetch error by trying to update non-existing without create intent
			args: args{
				ctx: context.TODO(),
				cm:  existingCM,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &StaleSecretWatchReconciler{
				Client: tt.client,
			}
			if err := r.createOrUpdateConfigMap(tt.args.ctx, tt.args.cm); (err != nil) != tt.wantErr {
				t.Errorf("StaleSecretWatchReconciler.createOrUpdateConfigMap() error = %v, wantErr %v", err, tt.wantErr)
			}
			// Optionally check results by fetching the ConfigMap
			if !tt.wantErr {
				updatedCM := &corev1.ConfigMap{}
				err := tt.client.Get(tt.args.ctx, types.NamespacedName{Name: tt.args.cm.Name, Namespace: tt.args.cm.Namespace}, updatedCM)
				if err != nil {
					t.Errorf("Failed to fetch ConfigMap: %v", err)
				}
				if fmt.Sprintf("%s", updatedCM.BinaryData["data"]) != fmt.Sprintf("%s", tt.args.cm.BinaryData["data"]) {
					t.Errorf("ConfigMap data was not updated properly, got: %s, want: %s", updatedCM.BinaryData["data"], tt.args.cm.BinaryData["data"])
				}
			}
		})
	}
}

func TestStaleSecretWatchReconciler_prepareWatchList(t *testing.T) {

	// Initialize scheme
	sch := runtime.NewScheme()
	_ = corev1.AddToScheme(sch)
	_ = securityv1beta1.AddToScheme(sch)

	// Create test logger
	logger := logr.Discard()

	// Prepare namespace and secrets
	ns1 := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns1"}}
	ns2 := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns2"}}
	secret1 := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "secret1", Namespace: "ns1"}}
	secret2 := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "secret2", Namespace: "ns1"}, Type: "bootstrap.kubernetes.io/token"}
	secret3 := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "secret3", Namespace: "ns2"}}
	secret4 := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "secret4", Namespace: "ns2"}, Type: "kubernetes.io/service-account-token"}

	// Create a fake client with the above objects
	fclient := fake.NewClientBuilder().WithScheme(sch).WithObjects(ns1, ns2, secret1, secret2, secret3, secret4).Build()

	// Define args for the tests
	sswAllNamespaces := &securityv1beta1.StaleSecretWatch{
		Spec: securityv1beta1.StaleSecretWatchSpec{
			StaleSecretToWatch: securityv1beta1.StaleSecretToWatch{
				Namespace: "all",
				ExcludeList: []securityv1beta1.ExcludeList{
					{Namespace: "ns1", SecretName: "secret2"},
				},
			},
		},
	}
	sswSpecificNamespace1 := &securityv1beta1.StaleSecretWatch{
		Spec: securityv1beta1.StaleSecretWatchSpec{
			StaleSecretToWatch: securityv1beta1.StaleSecretToWatch{
				Namespace: "ns1",
			},
		},
	}
	sswSpecificNamespace2 := &securityv1beta1.StaleSecretWatch{
		Spec: securityv1beta1.StaleSecretWatchSpec{
			StaleSecretToWatch: securityv1beta1.StaleSecretToWatch{
				Namespace: "ns1,ns2",
			},
		},
	}

	tests := []struct {
		name    string
		client  client.Client
		logger  logr.Logger
		ssw     *securityv1beta1.StaleSecretWatch
		want    map[string][]string
		wantErr bool
	}{
		{
			name:   "All namespaces with exclusion",
			client: fclient,
			logger: logger,
			ssw:    sswAllNamespaces,
			want: map[string][]string{
				"ns1": {"secret1"}, // "secret2" is excluded by type
				"ns2": {"secret3"}, // "secret4" is excluded by type
			},
			wantErr: false,
		},
		{
			name:   "Specific namespace without exclusion",
			client: fclient,
			logger: logger,
			ssw:    sswSpecificNamespace1,
			want: map[string][]string{
				"ns1": {"secret1"},
			},
			wantErr: false,
		},
		{
			name:   "Specific namespace without exclusion",
			client: fclient,
			logger: logger,
			ssw:    sswSpecificNamespace2,
			want: map[string][]string{
				"ns1": {"secret1"},
				"ns2": {"secret3"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &StaleSecretWatchReconciler{
				Client:   tt.client,
				Log:      tt.logger,
				Recorder: record.NewFakeRecorder(100),
			}
			got, err := r.prepareWatchList(context.Background(), tt.logger, tt.ssw)
			if (err != nil) != tt.wantErr {
				t.Errorf("StaleSecretWatchReconciler.prepareWatchList() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("StaleSecretWatchReconciler.prepareWatchList() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStaleSecretWatchReconciler_updateSecretStatuses(t *testing.T) {

	// Setup schema and client
	sch := runtime.NewScheme()
	_ = corev1.AddToScheme(sch)
	_ = securityv1beta1.AddToScheme(sch)

	// Logger
	logger := logr.Discard()
	cm := &corev1.ConfigMap{BinaryData: cmData}

	staleSecretWatch := &securityv1beta1.StaleSecretWatch{
		Spec: securityv1beta1.StaleSecretWatchSpec{
			StaleThresholdInDays: 1, // Secrets older than 1 day should be considered stale
		},
		Status: securityv1beta1.StaleSecretWatchStatus{
			SecretStatus: []securityv1beta1.SecretStatus{}, // Initially no stale secrets
		},
	}

	tests := []struct {
		name    string
		cm      *corev1.ConfigMap
		ssw     *securityv1beta1.StaleSecretWatch
		wantErr bool
	}{
		{
			name:    "Secret should be stale",
			cm:      cm,
			ssw:     staleSecretWatch,
			wantErr: false,
		},
		{
			name: "Secret should not be stale",
			cm: &corev1.ConfigMap{BinaryData: map[string][]byte{"data": []byte(`{
				"namespaces": [
					{
						"name": "vivid2",
						"secrets": [
							{
								"name": "chef-user-secret2",
								"created": "2024-04-11T11:08:02Z",
								"last_modified": "2024-04-13T15:21:48Z",
								"history": [
									{
										"data": "7cdd5f2812f1590e12f31c127727746241459da8b8f35ce3a555e18af20316f1"
									}
								],
								"type": "Opaque"
							}
						]
					}
				]
			}`)}},
			ssw: &securityv1beta1.StaleSecretWatch{
				Spec: securityv1beta1.StaleSecretWatchSpec{
					StaleThresholdInDays: 0, // No threshold for staleness
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &StaleSecretWatchReconciler{
				Client: fake.NewClientBuilder().WithRuntimeObjects(tt.cm).Build(),
				Log:    logger,
			}
			err := r.updateSecretStatuses(logger, tt.cm, tt.ssw)
			if (err != nil) != tt.wantErr {
				t.Errorf("updateSecretStatuses() error = %v, wantErr %v", err, tt.wantErr)
			}
			if len(tt.ssw.Status.SecretStatus) > 0 && tt.ssw.Status.SecretStatus[0].IsStale != true {
				t.Errorf("Expected the secret to be stale, got %v", tt.ssw.Status.SecretStatus[0].IsStale)
			}
		})
	}
}

func TestStaleSecretWatchReconciler_removeFinalizer(t *testing.T) {
	// Mock request and logger setup
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "test", Namespace: "default"}}
	logger := logr.Discard()
	scheme := runtime.NewScheme()
	_ = securityv1beta1.AddToScheme(scheme)
	staleSecretWatch := &securityv1beta1.StaleSecretWatch{
		ObjectMeta: metav1.ObjectMeta{
			Finalizers: []string{"security.stalesecretwatch.io/finalizer"},
			Name:       "test",
			Namespace:  "default",
		},
	}
	fclient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(staleSecretWatch).Build()

	tests := []struct {
		name    string
		setup   func(r *StaleSecretWatchReconciler)
		want    ctrl.Result
		wantErr bool
	}{
		{
			name: "Successful Finalizer Removal",
			setup: func(r *StaleSecretWatchReconciler) {
				// Mock scenario setup for successful removal
				r.Client = fclient
				r.Scheme = scheme
			},
			want:    ctrl.Result{},
			wantErr: false,
		},
		{
			name: "Failed Finalizer Removal - Fetch Error",
			setup: func(r *StaleSecretWatchReconciler) {
				// Setup to fail on client.Get
				r.Client = fclient
			},
			want:    ctrl.Result{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &StaleSecretWatchReconciler{
				Client:          fclient,
				Log:             logger,
				RequeueInterval: time.Second * 10,
				Scheme:          scheme,
				Recorder:        record.NewFakeRecorder(10),
			}
			tt.setup(r)
			got, err := r.removeFinalizer(context.Background(), logger, staleSecretWatch, &req)
			if (err != nil) != tt.wantErr {
				t.Errorf("removeFinalizer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("removeFinalizer() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStaleSecretWatchReconciler_addFinalizer(t *testing.T) {
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "test", Namespace: "default"}}
	logger := logr.Discard()
	ctx := context.TODO()
	sch := runtime.NewScheme()
	_ = securityv1beta1.AddToScheme(sch)
	staleSecretWatch := &securityv1beta1.StaleSecretWatch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: securityv1beta1.StaleSecretWatchSpec{
			StaleSecretToWatch: securityv1beta1.StaleSecretToWatch{
				Namespace: "all",
				ExcludeList: []securityv1beta1.ExcludeList{
					{Namespace: "ns1", SecretName: "secret2"},
				},
			},
		},
	}
	fclient := fake.NewClientBuilder().WithScheme(sch).WithObjects(staleSecretWatch).Build()

	tests := []struct {
		name    string
		client  client.Client
		want    ctrl.Result
		wantErr bool
	}{
		{
			name:    "Successful Finalizer Addition",
			client:  fclient,
			want:    ctrl.Result{},
			wantErr: true,
		},
		{
			name:    "Failed Finalizer Addition - Fetch Error",
			client:  fake.NewClientBuilder().WithScheme(sch).Build(), // empty client to simulate fetch error
			want:    ctrl.Result{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &StaleSecretWatchReconciler{
				Client:          tt.client,
				Log:             logger,
				RequeueInterval: time.Second * 10,
				Scheme:          sch,
				Recorder:        record.NewFakeRecorder(10),
			}
			//r.Client = tt.client // use the specific client setup for the test case
			got, err := r.addFinalizer(ctx, logger, staleSecretWatch, &req)
			if (err != nil) != tt.wantErr {
				t.Errorf("addFinalizer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("addFinalizer() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStaleSecretWatchReconciler_updateStatusCondition(t *testing.T) {
	ctx := context.Background()
	sch := runtime.NewScheme()
	_ = securityv1beta1.AddToScheme(sch)
	staleSecretWatch := &securityv1beta1.StaleSecretWatch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}
	fclient := fake.NewClientBuilder().WithScheme(sch).WithObjects(staleSecretWatch).Build()

	tests := []struct {
		name    string
		setup   func(r *StaleSecretWatchReconciler)
		wantErr bool
	}{
		{
			name: "Status Update Success",
			setup: func(r *StaleSecretWatchReconciler) {
				r.Client = fclient
			},
			wantErr: true,
		},
		{
			name: "Status Update Conflict Error",
			setup: func(r *StaleSecretWatchReconciler) {
				r.Client = fclient
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &StaleSecretWatchReconciler{}
			tt.setup(r)
			err := r.updateStatusCondition(ctx, staleSecretWatch, "TestCondition", metav1.ConditionTrue, "Testing", "Just a test")
			if (err != nil) != tt.wantErr {
				t.Errorf("updateStatusCondition() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestStaleSecretWatchReconciler_configMapForStaleSecretWatch(t *testing.T) {
	sch := runtime.NewScheme()
	_ = securityv1beta1.AddToScheme(sch)
	staleSecretWatch := &securityv1beta1.StaleSecretWatch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}
	fclient := fake.NewClientBuilder().WithScheme(sch).WithObjects(staleSecretWatch).Build()
	type fields struct {
		Client          client.Client
		Log             logr.Logger
		RequeueInterval time.Duration
		Scheme          *runtime.Scheme
		Recorder        record.EventRecorder
	}
	type args struct {
		ssw *securityv1beta1.StaleSecretWatch
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *corev1.ConfigMap
		wantErr bool
	}{
		{
			name: "Create ConfigMap",
			fields: fields{
				Client:          fclient,
				Log:             logr.Discard(),
				RequeueInterval: time.Second * 10,
				Scheme:          sch,
				Recorder:        record.NewFakeRecorder(10),
			},
			args: args{
				ssw: &securityv1beta1.StaleSecretWatch{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-ssw",
						Namespace: "default",
					},
				},
			},
			want: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "hashed-secrets-stalesecretwatch",
					Namespace: "default",
					Labels: map[string]string{
						"app.kubernetes.io/name":       "StaleSecretWatch",
						"app.kubernetes.io/instance":   "test-ssw",
						"app.kubernetes.io/part-of":    "StaleSecretWatch-operator",
						"app.kubernetes.io/created-by": "controller-manager-stalesecretwatch",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Error - Invalid SSW",
			fields: fields{
				Client:          fclient,
				Log:             logr.Discard(),
				RequeueInterval: time.Second * 10,
				Scheme:          sch,
				Recorder:        record.NewFakeRecorder(10),
			},
			args: args{
				ssw: &securityv1beta1.StaleSecretWatch{}, // Missing name and namespace
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &StaleSecretWatchReconciler{
				Client:          tt.fields.Client,
				Log:             tt.fields.Log,
				RequeueInterval: tt.fields.RequeueInterval,
				Scheme:          tt.fields.Scheme,
				Recorder:        tt.fields.Recorder,
			}
			got, err := r.configMapForStaleSecretWatch(tt.args.ssw)
			if (err != nil) != tt.wantErr {
				t.Errorf("StaleSecretWatchReconciler.configMapForStaleSecretWatch() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got.DeepCopy() == tt.want.DeepCopy() {
				t.Errorf("StaleSecretWatchReconciler.configMapForStaleSecretWatch() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStaleSecretWatchReconciler_calculateAndStoreHashedSecrets(t *testing.T) {
	sch := runtime.NewScheme()
	_ = corev1.AddToScheme(sch)
	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "hashed-secrets-stalesecretwatch", Namespace: "default"},
		BinaryData: cmData,
	}
	existingSecret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "secret1", Namespace: "ns1"}}
	fclient := fake.NewClientBuilder().WithScheme(sch).WithObjects(existingCM, existingSecret).Build()
	secretsToWatch := map[string][]string{
		"ns1": {"secret1"},
	}

	type fields struct {
		Client          client.Client
		Log             logr.Logger
		RequeueInterval time.Duration
		Scheme          *runtime.Scheme
		Recorder        record.EventRecorder
	}
	type args struct {
		ctx            context.Context
		logger         logr.Logger
		secretsToWatch map[string][]string
		cm             *corev1.ConfigMap
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "valid data",
			fields: fields{
				Client:          fclient,
				Log:             logr.Discard(),
				RequeueInterval: time.Second * 10,
				Scheme:          sch,
				Recorder:        record.NewFakeRecorder(10),
			},
			args: args{
				ctx:            context.TODO(),
				logger:         logr.Discard(),
				secretsToWatch: secretsToWatch,
				cm:             existingCM,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &StaleSecretWatchReconciler{
				Client:          tt.fields.Client,
				Log:             tt.fields.Log,
				RequeueInterval: tt.fields.RequeueInterval,
				Scheme:          tt.fields.Scheme,
				Recorder:        tt.fields.Recorder,
			}
			if err := r.calculateAndStoreHashedSecrets(tt.args.ctx, tt.args.logger, tt.args.secretsToWatch, tt.args.cm); (err != nil) != tt.wantErr {
				t.Errorf("StaleSecretWatchReconciler.calculateAndStoreHashedSecrets() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestStaleSecretWatchReconciler_performDailyChecks(t *testing.T) {
	os.Setenv("SLACK_BOT_TOKEN", "your_slack_bot_token")
	os.Setenv("SLACK_CHANNEL_ID", "your_slack_channel_id")
	sch := runtime.NewScheme()
	_ = securityv1beta1.AddToScheme(sch)
	_ = corev1.AddToScheme(sch)
	staleSecretWatch := &securityv1beta1.StaleSecretWatch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		}, Status: securityv1beta1.StaleSecretWatchStatus{
			SecretStatus: []securityv1beta1.SecretStatus{
				{
					Name:         "my-secret",
					Namespace:    "vivid",
					SecretType:   "Opaque",
					Created:      metav1.Now(),
					LastModified: metav1.Now(),
					IsStale:      true,
					Message:      "secret is stale",
				},
			},
			StaleSecretsCount: 1,
		},
	}
	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "hashed-secrets-stalesecretwatch", Namespace: "default"},
		BinaryData: cmData,
	}
	fclient := fake.NewClientBuilder().WithScheme(sch).WithObjects(staleSecretWatch, existingCM).Build()
	type fields struct {
		Client          client.Client
		Log             logr.Logger
		RequeueInterval time.Duration
		Scheme          *runtime.Scheme
		Recorder        record.EventRecorder
	}
	type args struct {
		ctx    context.Context
		logger logr.Logger
		ssw    *securityv1beta1.StaleSecretWatch
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    bool
		want1   ctrl.Result
		wantErr bool
	}{
		{
			name: "valid data",
			fields: fields{
				Client:          fclient,
				Log:             logr.Discard(),
				RequeueInterval: time.Second * 10,
				Scheme:          sch,
				Recorder:        record.NewFakeRecorder(10),
			},
			args: args{
				ctx:    context.TODO(),
				logger: logr.Discard(),
				ssw:    staleSecretWatch,
			},
			want:    true,
			wantErr: true,
			want1:   ctrl.Result{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &StaleSecretWatchReconciler{
				Client:          tt.fields.Client,
				Log:             tt.fields.Log,
				RequeueInterval: tt.fields.RequeueInterval,
				Scheme:          tt.fields.Scheme,
				Recorder:        tt.fields.Recorder,
			}
			got, got1, err := r.performDailyChecks(tt.args.ctx, tt.args.logger, tt.args.ssw)
			if (err != nil) != tt.wantErr {
				t.Errorf("StaleSecretWatchReconciler.performDailyChecks() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("StaleSecretWatchReconciler.performDailyChecks() got = %v, want %v", got, tt.want)
			}
			if !reflect.DeepEqual(got1, tt.want1) {
				t.Errorf("StaleSecretWatchReconciler.performDailyChecks() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func TestStaleSecretWatchReconciler_Reconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "test", Namespace: "default"}}
	sch := runtime.NewScheme()
	_ = securityv1beta1.AddToScheme(sch)
	_ = corev1.AddToScheme(sch)
	staleSecretWatch := &securityv1beta1.StaleSecretWatch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		}, Status: securityv1beta1.StaleSecretWatchStatus{
			SecretStatus: []securityv1beta1.SecretStatus{
				{
					Name:         "my-secret",
					Namespace:    "vivid",
					SecretType:   "Opaque",
					Created:      metav1.Now(),
					LastModified: metav1.Now(),
					IsStale:      true,
					Message:      "secret is stale",
				},
			},
			StaleSecretsCount: 1,
		},
	}
	fclient := fake.NewClientBuilder().WithScheme(sch).WithObjects(staleSecretWatch).Build()

	type fields struct {
		Client          client.Client
		Log             logr.Logger
		RequeueInterval time.Duration
		Scheme          *runtime.Scheme
		Recorder        record.EventRecorder
	}
	type args struct {
		ctx context.Context
		req ctrl.Request
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    ctrl.Result
		wantErr bool
	}{
		{
			name: "valid data",
			fields: fields{
				Client:          fclient,
				Log:             logr.Discard(),
				RequeueInterval: time.Second * 10,
				Scheme:          sch,
				Recorder:        record.NewFakeRecorder(10),
			},
			args: args{
				ctx: context.TODO(),
				req: req,
			},
			want:    ctrl.Result{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &StaleSecretWatchReconciler{
				Client:          tt.fields.Client,
				Log:             tt.fields.Log,
				RequeueInterval: tt.fields.RequeueInterval,
				Scheme:          tt.fields.Scheme,
				Recorder:        tt.fields.Recorder,
			}
			got, err := r.Reconcile(tt.args.ctx, tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("StaleSecretWatchReconciler.Reconcile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("StaleSecretWatchReconciler.Reconcile() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStaleSecretWatchReconciler_updateLastCheckedAnnotation(t *testing.T) {
	sch := runtime.NewScheme()
	_ = securityv1beta1.AddToScheme(sch)
	staleSecretWatch := &securityv1beta1.StaleSecretWatch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		}, Status: securityv1beta1.StaleSecretWatchStatus{
			SecretStatus: []securityv1beta1.SecretStatus{
				{
					Name:         "my-secret",
					Namespace:    "vivid",
					SecretType:   "Opaque",
					Created:      metav1.Now(),
					LastModified: metav1.Now(),
					IsStale:      true,
					Message:      "secret is stale",
				},
			},
			StaleSecretsCount: 1,
		},
	}
	fclient := fake.NewClientBuilder().WithScheme(sch).WithObjects(staleSecretWatch).Build()
	type fields struct {
		Client          client.Client
		Log             logr.Logger
		RequeueInterval time.Duration
		Scheme          *runtime.Scheme
		Recorder        record.EventRecorder
	}
	type args struct {
		ctx context.Context
		ssw *securityv1beta1.StaleSecretWatch
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "valid data",
			fields: fields{
				Client:          fclient,
				Log:             logr.Discard(),
				RequeueInterval: time.Second * 10,
				Scheme:          sch,
				Recorder:        record.NewFakeRecorder(10),
			},
			args: args{
				ctx: context.TODO(),
				ssw: staleSecretWatch,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &StaleSecretWatchReconciler{
				Client:          tt.fields.Client,
				Log:             tt.fields.Log,
				RequeueInterval: tt.fields.RequeueInterval,
				Scheme:          tt.fields.Scheme,
				Recorder:        tt.fields.Recorder,
			}
			if err := r.updateLastCheckedAnnotation(tt.args.ctx, tt.args.ssw); (err != nil) != tt.wantErr {
				t.Errorf("StaleSecretWatchReconciler.updateLastCheckedAnnotation() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
