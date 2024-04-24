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
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	admissionv1 "k8s.io/api/admission/v1"
	//+kubebuilder:scaffold:imports
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var ctx context.Context
var cancel context.CancelFunc

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Webhook Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: false,
		WebhookInstallOptions: envtest.WebhookInstallOptions{
			Paths:            []string{filepath.Join("..", "..", "config", "webhook")},
			LocalServingPort: 9443,
			LocalServingHost: "127.0.0.1",
		},
	}

	var err error
	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	scheme := runtime.NewScheme()
	err = AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())

	err = admissionv1.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// start webhook server using Manager
	webhookInstallOptions := &testEnv.WebhookInstallOptions
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
		Metrics: server.Options{
			BindAddress: ":8080",
		},
		LeaderElection: false,
	})
	Expect(err).NotTo(HaveOccurred())

	err = (&StaleSecretWatch{}).SetupWebhookWithManager(mgr)
	Expect(err).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:webhook

	go func() {
		defer GinkgoRecover()
		err = mgr.Start(ctx)
		Expect(err).NotTo(HaveOccurred())
	}()

	// wait for the webhook server to get ready
	dialer := &net.Dialer{Timeout: time.Second}
	addrPort := fmt.Sprintf("%s:%d", webhookInstallOptions.LocalServingHost, webhookInstallOptions.LocalServingPort)
	Eventually(func() error {
		conn, err := tls.DialWithDialer(dialer, "tcp", addrPort, &tls.Config{InsecureSkipVerify: true})
		if err != nil {
			return err
		}
		conn.Close()
		return nil
	}, 10*time.Second, 1*time.Second).Should(Succeed())

})

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

var _ = Describe("StaleSecretWatch Webhook", func() {
	var (
		staleSecretWatch *StaleSecretWatch
		_                context.Context
	)

	BeforeEach(func() {
		// Initialize each test with a fresh instance of StaleSecretWatch
		staleSecretWatch = &StaleSecretWatch{
			Spec: StaleSecretWatchSpec{
				StaleThresholdInDays: 1,
				StaleSecretToWatch: StaleSecretToWatch{
					Namespace: "default",
					ExcludeList: []ExcludeList{
						{
							Namespace:  "kube-system",
							SecretName: "some-secret",
						},
					},
				},
			},
		}
		ctx = context.TODO()
	})

	Describe("Validating Create", func() {
		It("should allow valid config", func() {
			staleSecretWatch.Spec.StaleSecretToWatch.Namespace = "default"
			warnings, err := staleSecretWatch.ValidateCreate()
			Expect(err).ToNot(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("should reject invalid namespace", func() {
			staleSecretWatch.Spec.StaleSecretToWatch.Namespace = ""
			_, err := staleSecretWatch.ValidateCreate()
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Validating Update", func() {
		It("should pass on valid changes", func() {
			oldObject := staleSecretWatch.DeepCopy()
			staleSecretWatch.Spec.StaleThresholdInDays = 2 // an example modification
			_, err := staleSecretWatch.ValidateUpdate(oldObject)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("Default Values", func() {
		It("should set default namespace if not specified", func() {
			staleSecretWatch.Namespace = ""
			staleSecretWatch.Default()
			Expect(staleSecretWatch.Namespace).To(Equal("default"))
		})
	})
})

var _ = Describe("StaleSecretWatch Webhook Validation 01", func() {
	var staleSecretWatch *StaleSecretWatch

	BeforeEach(func() {
		staleSecretWatch = &StaleSecretWatch{
			Spec: StaleSecretWatchSpec{
				StaleThresholdInDays: 1,
				RefreshInterval:      nil, // not setting this to test default behavior
				StaleSecretToWatch: StaleSecretToWatch{
					Namespace: "", // intentionally left blank to test validation logic
					ExcludeList: []ExcludeList{
						{
							SecretName: "",
							Namespace:  "test,namespace",
						},
					},
				},
			},
		}
	})

	Describe("Refresh Interval Defaulting", func() {
		It("should set a default refresh interval if none is provided", func() {
			_ = staleSecretWatch.ValidateStaleSecretWatch()
			Expect(staleSecretWatch.Spec.RefreshInterval.Duration).To(Equal(time.Hour))
		})
	})

	Describe("Namespace Validations", func() {
		Context("when the namespace is empty and not 'all'", func() {
			It("should return an error indicating the namespace cannot be empty", func() {
				err := staleSecretWatch.ValidateStaleSecretWatch()
				Expect(err).To(MatchError("staleSecretToWatch.namespace cannot be empty, please specify 'all' to watch all namespace or existing namespace name"))
			})
		})
	})
})

var _ = Describe("StaleSecretWatch Webhook Validation 02", func() {
	var staleSecretWatch *StaleSecretWatch

	BeforeEach(func() {
		staleSecretWatch = &StaleSecretWatch{
			Spec: StaleSecretWatchSpec{
				StaleThresholdInDays: 1,
				RefreshInterval:      nil, // not setting this to test default behavior
				StaleSecretToWatch: StaleSecretToWatch{
					Namespace: "default",
					ExcludeList: []ExcludeList{
						{
							SecretName: "valid-secret-name", // Set valid name to bypass secret name validation
							Namespace:  "test,namespace",    // Set invalid namespace to test invalid character validation
						},
					},
				},
			},
		}
	})

	Describe("Exclude List Entry Validations", func() {
		Context("when an exclude list entry has an empty secret name", func() {
			BeforeEach(func() {
				// Overriding ExcludeList to generate error for this specific test
				staleSecretWatch.Spec.StaleSecretToWatch.ExcludeList[0].SecretName = ""
			})
			It("should return an error for empty secret name", func() {
				err := staleSecretWatch.ValidateStaleSecretWatch()
				Expect(err).To(MatchError("excludeList.secretName cannot be empty"))
			})
		})

		Context("when an exclude list entry has a namespace with invalid characters", func() {
			It("should return an error for invalid characters in namespace", func() {
				err := staleSecretWatch.ValidateStaleSecretWatch()
				Expect(err).To(MatchError(ContainSubstring("invalid characters in namespace name")))
			})
		})
	})

	Describe("Stale Threshold Validations", func() {
		Context("exclude.Namespace must be a single/existing namespace name", func() {
			It("should return an error stating it must be a positive integer", func() {
				err := staleSecretWatch.ValidateStaleSecretWatch()
				Expect(err).To(MatchError("invalid characters in namespace name: test,namespace, exclude.Namespace must be a single/existing namespace name"))
			})
		})
	})

	Describe("Stale Threshold Validations", func() {
		Context("when staleThresholdInDays is non-positive", func() {
			BeforeEach(func() {
				staleSecretWatch.Spec.StaleSecretToWatch.ExcludeList[0].Namespace = "test"
				staleSecretWatch.Spec.StaleThresholdInDays = 0 // Set to zero to test non-positive validation
			})
			It("should return an error stating it must be a positive integer", func() {
				err := staleSecretWatch.ValidateStaleSecretWatch()
				Expect(err).To(MatchError("staleThresholdInDays must be a positive integer"))
			})
		})
	})

})
