/*
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
package controllers

import (
	"context"
	"fmt"
	"os"
	"time"

	addonapiv1 "github.com/keikoproj/addon-manager/api/addon"
	addonmgrv1alpha1 "github.com/keikoproj/addon-manager/api/addon/v1alpha1"
	"github.com/keikoproj/addon-manager/pkg/addon"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
)

var (
	addonNamespace = "default"
)

const timeout = time.Second * 5

var _ = Describe("AddonController", func() {

	Describe("Addon CR can be reconciled", func() {
		var instance *addonmgrv1alpha1.Addon
		var wfv1 = &unstructured.Unstructured{}
		var addonName = "cluster-autoscaler"
		var addonKey = types.NamespacedName{Name: addonName, Namespace: addonNamespace}

		wfv1.SetGroupVersionKind(schema.GroupVersionKind{
			Kind:    "Workflow",
			Group:   "argoproj.io",
			Version: "v1alpha1",
		})

		Context("Addon CR is created", func() {
			It("Creating a new Addon instance", func() {
				addonYaml, err := os.ReadFile("../docs/examples/clusterautoscaler.yaml")
				Expect(err).ToNot(HaveOccurred())

				instance, err = parseAddonYaml(addonYaml)
				Expect(err).ToNot(HaveOccurred())
				Expect(instance).To(BeAssignableToTypeOf(&addonmgrv1alpha1.Addon{}))
				Expect(instance.GetName()).To(Equal(addonName))

				instance.SetNamespace(addonNamespace)
			})

			It("instance should be reconciled", func() {
				defer k8sClient.Delete(context.Background(), instance)
				err := k8sClient.Create(context.TODO(), instance)
				Expect(err).NotTo(HaveOccurred())
				Eventually(func() error {
					if err := k8sClient.Get(context.TODO(), addonKey, instance); err != nil {
						return err
					}

					if len(instance.ObjectMeta.Finalizers) > 0 {
						return nil
					}
					return fmt.Errorf("addon is not valid")
				}, timeout).Should(Succeed())

				By("Verify addon has been reconciled by checking for checksum status")
				Expect(instance.Status.Checksum).ShouldNot(BeEmpty())

				By("Verify addon has finalizers added which means it's valid")
				Expect(instance.ObjectMeta.Finalizers).Should(Equal([]string{addonapiv1.FinalizerName}))

				By("Verify addon has pending status")
				Expect(instance.Status.Lifecycle.Installed).Should(Equal(addonmgrv1alpha1.Pending))

				By("Verify addon has prereqs workflow generated with checksum name")
				wfName := instance.GetFormattedWorkflowName(addonmgrv1alpha1.Prereqs)
				var wfv1Key = types.NamespacedName{Name: wfName, Namespace: addonNamespace}
				Eventually(func() error {
					return k8sClient.Get(context.TODO(), wfv1Key, wfv1)
				}, timeout).Should(Succeed())
				Expect(wfv1.GetName()).Should(Equal(wfName))

				oldCheckSum := instance.Status.Checksum

				//Update instance params for checksum validation
				instance.Spec.Params.Context.ClusterRegion = "us-east-2"
				err = k8sClient.Update(context.TODO(), instance)
				Expect(err).NotTo(HaveOccurred())
				Consistently(func() error {
					if err := k8sClient.Get(context.TODO(), addonKey, instance); err != nil {
						return err
					}

					if len(instance.ObjectMeta.Finalizers) > 0 {
						return nil
					}
					return fmt.Errorf("addon is not valid")
				}, timeout).Should(Succeed())

				By("Verify changing addon spec generates new checksum")
				Expect(instance.Status.Checksum).ShouldNot(BeIdenticalTo(oldCheckSum))

				By("Verify old prereqs workflow generated with old checksum name has been deleted")
				Consistently(func() error {
					if apierrors.IsNotFound(k8sClient.Get(context.TODO(), wfv1Key, wfv1)) {
						return nil
					}

					return fmt.Errorf("old workflow was not deleted")
				}, timeout).Should(Succeed())

				By("Verify addon has prereqs workflow generated with new checksum name")
				wfName = instance.GetFormattedWorkflowName(addonmgrv1alpha1.Prereqs)
				wfv1Key = types.NamespacedName{Name: wfName, Namespace: addonNamespace}
				Eventually(func() error {
					return k8sClient.Get(context.TODO(), wfv1Key, wfv1)
				}, timeout).Should(Succeed())
				Expect(wfv1.GetName()).Should(Equal(wfName))

				By("Verify addon still has pending status")
				err = k8sClient.Get(context.TODO(), addonKey, instance)
				Expect(err).NotTo(HaveOccurred())
				Expect(instance.Status.Lifecycle.Installed).Should(Equal(addonmgrv1alpha1.Pending))

				By("Verify addon prereqs status completed after prereqs workflow is completed")
				wfv1.UnstructuredContent()["status"] = map[string]interface{}{
					"phase": "Succeeded",
				}
				err = k8sClient.Update(context.TODO(), wfv1)
				Expect(err).NotTo(HaveOccurred())
				Eventually(func() error {
					if err := k8sClient.Get(context.TODO(), addonKey, instance); err != nil {
						return err
					}
					if instance.Status.Lifecycle.Prereqs == addonmgrv1alpha1.Succeeded && instance.Status.Lifecycle.Installed == addonmgrv1alpha1.Pending {
						return nil
					}
					return fmt.Errorf("addon prereqs|install status(%s|%s) is not succeeded|pending", instance.Status.Lifecycle.Prereqs, instance.Status.Lifecycle.Installed)
				}, timeout).Should(Succeed())

				By("Verify addon has install workflow generated with new checksum name")
				wfName = instance.GetFormattedWorkflowName(addonmgrv1alpha1.Install)
				wfv1Key = types.NamespacedName{Name: wfName, Namespace: addonNamespace}
				Eventually(func() error {
					return k8sClient.Get(context.TODO(), wfv1Key, wfv1)
				}, timeout).Should(Succeed())
				Expect(wfv1.GetName()).Should(Equal(wfName))

				By("Verify addon install status completed after install workflow is completed")
				wfv1.UnstructuredContent()["status"] = map[string]interface{}{
					"phase": "Succeeded",
				}
				err = k8sClient.Update(context.TODO(), wfv1)
				Expect(err).NotTo(HaveOccurred())
				Eventually(func() error {
					if err := k8sClient.Get(context.TODO(), addonKey, instance); err != nil {
						return err
					}
					if instance.Status.Lifecycle.Installed == addonmgrv1alpha1.Succeeded {
						return nil
					}
					return fmt.Errorf("addon install status(%s) is not succeeded", instance.Status.Lifecycle.Installed)
				}, timeout).Should(Succeed())

				By("Verify deleting workflows triggers reconcile but doesn't regenerate workflows again after completed")
				Expect(k8sClient.Delete(context.TODO(), wfv1)).To(Succeed())
				Consistently(func() error {
					if apierrors.IsNotFound(k8sClient.Get(context.TODO(), wfv1Key, wfv1)) {
						return nil
					}

					return fmt.Errorf("workflow was regenerated")
				}, timeout).Should(Succeed())
			})
		})

		Context("Addon CR can be successfully deleted", func() {
			It("instance should be deleted w/ deleting state", func() {
				By("Verify deleting instance should set Deleting state")
				Expect(k8sClient.Delete(context.TODO(), instance)).NotTo(HaveOccurred())
				Eventually(func() error {
					if err := k8sClient.Get(context.TODO(), addonKey, instance); err != nil {
						return err
					}

					if instance.ObjectMeta.DeletionTimestamp != nil && instance.Status.Lifecycle.Installed == addonmgrv1alpha1.Deleting {
						return nil
					}
					return fmt.Errorf("addon is not being deleted")
				}, timeout).Should(Succeed())

				By("Verify delete workflow was generated")
				wfName := instance.GetFormattedWorkflowName(addonmgrv1alpha1.Delete)
				var wfv1Key = types.NamespacedName{Name: wfName, Namespace: addonNamespace}
				Eventually(func() error {
					return k8sClient.Get(context.TODO(), wfv1Key, wfv1)
				}, timeout).Should(Succeed())
				Expect(wfv1.GetName()).Should(Equal(wfName))

				By("Verify addon remains in deleting state while delete workflow is running")
				Eventually(func() error {
					if err := k8sClient.Get(context.TODO(), addonKey, instance); err != nil {
						return err
					}

					if instance.Status.Lifecycle.Installed == addonmgrv1alpha1.Deleting {
						return nil
					}
					return fmt.Errorf("addon is not being deleted. Status: %v", instance.Status.Lifecycle.Installed)
				}, timeout).Should(Succeed())
			})

			It("instance should be deleted when delete workflow is successful", func() {
				By("Verify addon is deleted after delete workflow is completed")
				wfv1.UnstructuredContent()["status"] = map[string]interface{}{
					"phase": "Succeeded",
				}
				err := k8sClient.Update(context.TODO(), wfv1)
				Expect(err).NotTo(HaveOccurred())
				Eventually(func() error {
					if apierrors.IsNotFound(k8sClient.Get(context.TODO(), addonKey, instance)) {
						return nil
					}

					return fmt.Errorf("addon is not deleted")
				}, timeout).Should(Succeed())
			})

		})
	})

	Describe("Addon CR should reconcile delete failures", func() {
		var instance *addonmgrv1alpha1.Addon
		var wfv1 = &unstructured.Unstructured{}
		var addonName = "event-router"
		var addonKey = types.NamespacedName{Name: addonName, Namespace: addonNamespace}
		wfv1.SetGroupVersionKind(schema.GroupVersionKind{
			Kind:    "Workflow",
			Group:   "argoproj.io",
			Version: "v1alpha1",
		})

		It("Creating a new Addon instance", func() {
			addonYaml, err := os.ReadFile("../docs/examples/eventrouter.yaml")
			Expect(err).ToNot(HaveOccurred())

			instance, err = parseAddonYaml(addonYaml)
			Expect(err).ToNot(HaveOccurred())
			Expect(instance).To(BeAssignableToTypeOf(&addonmgrv1alpha1.Addon{}))
			Expect(instance.GetName()).To(Equal(addonName))

			instance.SetNamespace(addonNamespace)

			err = k8sClient.Create(context.TODO(), instance)
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() error {
				if err := k8sClient.Get(context.TODO(), addonKey, instance); err != nil {
					return err
				}

				if len(instance.ObjectMeta.Finalizers) > 0 {
					return nil
				}
				return fmt.Errorf("addon is not valid")
			}, timeout).Should(Succeed())

			By("Verify addon has been reconciled by checking for checksum status")
			Expect(instance.Status.Checksum).ShouldNot(BeEmpty())

			By("Verify addon has finalizers added which means it's valid")
			Expect(instance.ObjectMeta.Finalizers).Should(Equal([]string{addonapiv1.FinalizerName}))

			By("Verify addon has pending status")
			Expect(instance.Status.Lifecycle.Installed).Should(Equal(addonmgrv1alpha1.Pending))

			By("Verify deleting instance should set Deleting state")
			Expect(k8sClient.Delete(context.TODO(), instance)).NotTo(HaveOccurred())
			Eventually(func() error {
				if err := k8sClient.Get(context.TODO(), addonKey, instance); err != nil {
					return err
				}

				if instance.ObjectMeta.DeletionTimestamp != nil && instance.Status.Lifecycle.Installed == addonmgrv1alpha1.Deleting {
					return nil
				}
				return fmt.Errorf("addon is not being deleted")
			}, timeout).Should(Succeed())

			By("Verify delete workflow was generated")
			wfName := instance.GetFormattedWorkflowName(addonmgrv1alpha1.Delete)
			wfv1Key := types.NamespacedName{Name: wfName, Namespace: addonNamespace}
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), wfv1Key, wfv1)
			}, timeout).Should(Succeed())
			Expect(wfv1.GetName()).Should(Equal(wfName))

			By("Verify addon remains in deleting state while delete workflow is running")
			Eventually(func() error {
				if err := k8sClient.Get(context.TODO(), addonKey, instance); err != nil {
					return err
				}

				if instance.Status.Lifecycle.Installed == addonmgrv1alpha1.Deleting {
					return nil
				}
				return fmt.Errorf("addon is not being deleted. Status: %v", instance.Status.Lifecycle.Installed)
			}, timeout).Should(Succeed())

			By("Verify addon remains in DeleteFailed state after delete workflow fails")
			wfv1.UnstructuredContent()["status"] = map[string]interface{}{
				"phase": "Failed",
			}
			err = k8sClient.Update(context.TODO(), wfv1)
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() error {
				if err := k8sClient.Get(context.TODO(), addonKey, instance); err != nil {
					return err
				}

				if instance.Status.Lifecycle.Installed == addonmgrv1alpha1.DeleteFailed {
					return nil
				}
				return fmt.Errorf("addon is not in a delete failed state. Status: %v", instance.Status.Lifecycle.Installed)
			}, timeout).Should(Succeed())
		})
	})

	Describe("Addon CR should reconcile dependencies", func() {
		It("instance with dependencies should succeed", func() {
			instance := &addonmgrv1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{Name: "addon-1", Namespace: addonNamespace},
				Spec: addonmgrv1alpha1.AddonSpec{
					PackageSpec: addonmgrv1alpha1.PackageSpec{
						PkgType:    addonmgrv1alpha1.CompositePkg,
						PkgName:    "test/addon-1",
						PkgVersion: "1.0.1",
					},
					Params: addonmgrv1alpha1.AddonParams{
						Namespace: "addon-test-ns",
					},
				},
			}
			var instanceKey = types.NamespacedName{Namespace: instance.Namespace, Name: instance.Name}
			var instance2 = &addonmgrv1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{Name: "addon-2", Namespace: addonNamespace},
				Spec: addonmgrv1alpha1.AddonSpec{
					PackageSpec: addonmgrv1alpha1.PackageSpec{
						PkgType:    addonmgrv1alpha1.CompositePkg,
						PkgName:    "test/addon-2",
						PkgVersion: "1.0.0",
						PkgDeps: map[string]string{
							"test/addon-1": "*",
						},
					},
					Params: addonmgrv1alpha1.AddonParams{
						Namespace: "addon-test-ns",
					},
				},
			}
			var instanceKey2 = types.NamespacedName{Namespace: instance2.Namespace, Name: instance2.Name}

			By("Verify first addon-2 that depends on addon-1 is created and has validation failed state")
			Expect(k8sClient.Create(context.TODO(), instance2)).NotTo(HaveOccurred())
			Eventually(func() error {
				if err := k8sClient.Get(context.TODO(), instanceKey2, instance2); err != nil {
					return err
				}

				if instance2.Status.Lifecycle.Installed == addonmgrv1alpha1.ValidationFailed {
					return nil
				}

				return fmt.Errorf("addon-2 is not in validation failed state")
			}, timeout).Should(Succeed())

			By("Verify addon-1 is submitted and completes successfully")
			Expect(k8sClient.Create(context.TODO(), instance)).NotTo(HaveOccurred())
			defer k8sClient.Delete(context.TODO(), instance)
			Eventually(func() error {
				if err := k8sClient.Get(context.TODO(), instanceKey, instance); err != nil {
					return err
				}

				if instance.Status.Lifecycle.Installed == addonmgrv1alpha1.Succeeded {
					return nil
				}

				return fmt.Errorf("addon-1 is not installed")
			}, timeout).Should(Succeed())

			By("Verify addon-2 succeeds after addon-1 completed")
			Eventually(func() error {
				if err := k8sClient.Get(context.TODO(), instanceKey2, instance2); err != nil {
					return err
				}

				if instance2.Status.Lifecycle.Installed == addonmgrv1alpha1.Succeeded {
					return nil
				}

				return fmt.Errorf("addon-2 is not valid")
			}, timeout*10).Should(Succeed())
		})
	})
})

// Unit tests for the mapper functions used by the Watch mechanism
var _ = Describe("AddonController Map Functions", func() {
	var r *AddonReconciler
	var versionCache *mockVersionCache
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
		versionCache = &mockVersionCache{
			addonVersions: make(map[string]*addon.Version),
		}
		r = &AddonReconciler{
			versionCache: versionCache,
		}
	})

	Describe("getAddonRequestsFromLabels", func() {
		Context("with addon label", func() {
			It("should return a reconcile request for the matching addon", func() {
				// Setup mock
				testVersion := &addon.Version{
					Name:      "test-addon",
					Namespace: "default",
				}
				versionCache.addonVersions["test-addon-version"] = testVersion

				// Test the function
				labels := map[string]string{
					addonapiv1.ResourceDefaultOwnLabel: "test-addon-version",
				}

				requests := r.getAddonRequestsFromLabels(labels)

				// Verify results
				Expect(requests).To(HaveLen(1))
				Expect(requests[0].Name).To(Equal("test-addon"))
				Expect(requests[0].Namespace).To(Equal("default"))
			})
		})

		Context("without addon label", func() {
			It("should return empty requests", func() {
				// No addon label
				labels := map[string]string{
					"app": "some-app",
				}

				requests := r.getAddonRequestsFromLabels(labels)

				// Verify results
				Expect(requests).To(HaveLen(0))
			})
		})

		Context("with addon label but addon not found in cache", func() {
			It("should return empty requests", func() {
				// With addon label but no matching version in cache
				labels := map[string]string{
					addonapiv1.ResourceDefaultOwnLabel: "missing-addon",
				}

				requests := r.getAddonRequestsFromLabels(labels)

				// Verify results
				Expect(requests).To(HaveLen(0))
			})
		})
	})

	Describe("Resource mapper functions", func() {
		Context("with deployment resource", func() {
			It("should map deployment to addon reconcile requests", func() {
				// Setup mock
				testVersion := &addon.Version{
					Name:      "test-addon",
					Namespace: "default",
				}
				versionCache.addonVersions["test-addon-version"] = testVersion

				// Create a deployment with addon label
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-deployment",
						Namespace: "default",
						Labels: map[string]string{
							addonapiv1.ResourceDefaultOwnLabel: "test-addon-version",
						},
					},
				}

				// Test the function
				requests := r.mapDeploymentToAddonRequests(ctx, deployment)

				// Verify results
				Expect(requests).To(HaveLen(1))
				Expect(requests[0].Name).To(Equal("test-addon"))
				Expect(requests[0].Namespace).To(Equal("default"))
			})
		})

		Context("with service resource", func() {
			It("should map service to addon reconcile requests", func() {
				// Setup mock
				testVersion := &addon.Version{
					Name:      "test-addon",
					Namespace: "default",
				}
				versionCache.addonVersions["test-addon-version"] = testVersion

				// Create a service with addon label
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-service",
						Namespace: "default",
						Labels: map[string]string{
							addonapiv1.ResourceDefaultOwnLabel: "test-addon-version",
						},
					},
				}

				// Test the function
				requests := r.mapServiceToAddonRequests(ctx, service)

				// Verify results
				Expect(requests).To(HaveLen(1))
				Expect(requests[0].Name).To(Equal("test-addon"))
				Expect(requests[0].Namespace).To(Equal("default"))
			})
		})

		Context("with daemonset resource", func() {
			It("should map daemonset to addon reconcile requests", func() {
				// Setup mock
				testVersion := &addon.Version{
					Name:      "test-addon",
					Namespace: "default",
				}
				versionCache.addonVersions["test-addon-version"] = testVersion

				// Create a daemonset with addon label
				daemonset := &appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-daemonset",
						Namespace: "default",
						Labels: map[string]string{
							addonapiv1.ResourceDefaultOwnLabel: "test-addon-version",
						},
					},
				}

				// Test the function
				requests := r.mapDaemonSetToAddonRequests(ctx, daemonset)

				// Verify results
				Expect(requests).To(HaveLen(1))
				Expect(requests[0].Name).To(Equal("test-addon"))
				Expect(requests[0].Namespace).To(Equal("default"))
			})
		})

		Context("with statefulset resource", func() {
			It("should map statefulset to addon reconcile requests", func() {
				// Setup mock
				testVersion := &addon.Version{
					Name:      "test-addon",
					Namespace: "default",
				}
				versionCache.addonVersions["test-addon-version"] = testVersion

				// Create a statefulset with addon label
				statefulset := &appsv1.StatefulSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-statefulset",
						Namespace: "default",
						Labels: map[string]string{
							addonapiv1.ResourceDefaultOwnLabel: "test-addon-version",
						},
					},
				}

				// Test the function
				requests := r.mapStatefulSetToAddonRequests(ctx, statefulset)

				// Verify results
				Expect(requests).To(HaveLen(1))
				Expect(requests[0].Name).To(Equal("test-addon"))
				Expect(requests[0].Namespace).To(Equal("default"))
			})
		})

		Context("with job resource", func() {
			It("should map job to addon reconcile requests", func() {
				// Setup mock
				testVersion := &addon.Version{
					Name:      "test-addon",
					Namespace: "default",
				}
				versionCache.addonVersions["test-addon-version"] = testVersion

				// Create a job with addon label
				job := &batchv1.Job{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-job",
						Namespace: "default",
						Labels: map[string]string{
							addonapiv1.ResourceDefaultOwnLabel: "test-addon-version",
						},
					},
				}

				// Test the function
				requests := r.mapJobToAddonRequests(ctx, job)

				// Verify results
				Expect(requests).To(HaveLen(1))
				Expect(requests[0].Name).To(Equal("test-addon"))
				Expect(requests[0].Namespace).To(Equal("default"))
			})
		})
	})
})

// Mock implementation of VersionCacheClient for testing
type mockVersionCache struct {
	addonVersions map[string]*addon.Version
}

func (m *mockVersionCache) HasVersionName(name string) (bool, *addon.Version) {
	version, ok := m.addonVersions[name]
	return ok, version
}

// Additional required methods to satisfy the VersionCacheClient interface
func (m *mockVersionCache) AddVersion(version addon.Version) {
	// Not needed for this test
}

func (m *mockVersionCache) GetVersions(pkgName string) map[string]addon.Version {
	// Not needed for this test
	return make(map[string]addon.Version)
}

func (m *mockVersionCache) GetVersion(pkgName, pkgVersion string) *addon.Version {
	// Not needed for this test
	return nil
}

func (m *mockVersionCache) RemoveVersion(pkgName, pkgVersion string) {
	// Not needed for this test
}

func (m *mockVersionCache) RemoveVersions(pkgName string) {
	// Not needed for this test
}

func (m *mockVersionCache) GetAllVersions() map[string]map[string]addon.Version {
	// Not needed for this test
	return make(map[string]map[string]addon.Version)
}

func parseAddonYaml(data []byte) (*addonmgrv1alpha1.Addon, error) {
	var err error
	o := &unstructured.Unstructured{}
	err = yaml.Unmarshal(data, &o.Object)
	if err != nil {
		return nil, err
	}
	a := &addonmgrv1alpha1.Addon{}
	err = scheme.Scheme.Convert(o, a, 0)
	if err != nil {
		return nil, err
	}

	return a, nil
}
