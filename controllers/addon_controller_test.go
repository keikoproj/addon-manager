package controllers

import (
	"fmt"
	"io/ioutil"
	"time"

	wfclientsetfake "github.com/argoproj/argo-workflows/v3/pkg/client/clientset/versioned/fake"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	wfv1api "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	"github.com/keikoproj/addon-manager/api/addon/v1alpha1"
)

var (
	addonNamespace = "default"
	addonName      = "cluster-autoscaler"
	addonKey       = types.NamespacedName{Name: addonName, Namespace: addonNamespace}
)

const timeout = time.Second * 5

var instance *v1alpha1.Addon
var updated *v1alpha1.Addon
var fetchedAddon *v1alpha1.Addon
var addonController *Controller

var _ = Describe("AddonController", func() {

	Describe("Addon CR can be reconciled", func() {
		It("instance should be parsable", func() {
			addonYaml, err := ioutil.ReadFile("./tests/clusterautoscaler.yaml")
			Expect(err).ToNot(HaveOccurred())

			instance, err = parseAddonYaml(addonYaml)
			Expect(err).ToNot(HaveOccurred())
			Expect(instance).To(BeAssignableToTypeOf(&v1alpha1.Addon{}))
			Expect(instance.GetName()).To(Equal(addonName))

		})

		It("instance should be reconciled", func() {
			instance.SetNamespace(addonNamespace)
			addonController = newController(instance)

			processed := addonController.processNextItem(ctx)
			Expect(processed).To(BeTrue())
			var objects []runtime.Object
			wfcli := wfclientsetfake.NewSimpleClientset(objects...)
			err := generateWorkflow(wfcli, testNamespace)
			Expect(err).To(BeNil())

			Eventually(func() error {
				fetchedAddon, err = addonController.addoncli.AddonmgrV1alpha1().Addons(testNamespace).Get(ctx, testAddon, metav1.GetOptions{})
				Expect(err).To(BeNil())
				Expect(fetchedAddon).NotTo(BeNil())

				By("Verify addon has been reconciled by checking for checksum status")
				Expect(fetchedAddon.Status.Checksum).ShouldNot(BeEmpty())

				By("Verify addon has finalizers added which means it's valid")
				Expect(fetchedAddon.ObjectMeta.Finalizers).Should(Equal([]string{"delete.addonmgr.keikoproj.io"}))
				return nil
			}, timeout).Should(Succeed())
			oldCheckSum := instance.Status.Checksum
			//Update instance params for checksum validation
			instance.Spec.Params.Context.ClusterRegion = "us-east-2"
			err = addonController.handleAddonUpdate(ctx, fetchedAddon)
			Expect(err).To(BeNil())

			Eventually(func() error {
				updated, err = addonController.addoncli.AddonmgrV1alpha1().Addons(testNamespace).Get(ctx, fetchedAddon.Name, metav1.GetOptions{})
				Expect(err).To(BeNil())
				Expect(updated).NotTo(BeNil())
				Expect(updated.Status.Checksum).ShouldNot(BeIdenticalTo(oldCheckSum))
				return nil
			}, timeout).Should(Succeed())

			By("Verify changing addon spec generates new checksum")
			Expect(updated.Status.Checksum).ShouldNot(BeIdenticalTo(oldCheckSum))

			By("Verify addon has workflows generated with new checksum name")
			wfName := updated.GetFormattedWorkflowName(v1alpha1.Prereqs)
			var fetchedwf *wfv1api.Workflow
			Eventually(func() error {
				fetchedwf, err = addonController.wfcli.ArgoprojV1alpha1().Workflows(testNamespace).Get(ctx, wfName, metav1.GetOptions{})
				Expect(err).To(BeNil())
				Expect(fetchedwf.GetName()).Should(Equal(wfName))
				return nil
			}, timeout).Should(Succeed())

			By("Verify deleting workflows triggers reconcile and doesn't regenerate workflows again")
			err = addonController.wfcli.ArgoprojV1alpha1().Workflows(testNamespace).Delete(ctx, wfName, metav1.DeleteOptions{})
			Expect(err).To(BeNil())
			wf, err := addonController.wfcli.ArgoprojV1alpha1().Workflows(testNamespace).Get(ctx, wfName, metav1.GetOptions{})
			Expect(err).NotTo(BeNil())
			Expect(wf).To(BeNil())
		})

		It("instance should be deleted w/ deleting state", func() {
			By("Verify deleting instance should set Deleting state")
			Expect(addonController.addoncli.AddonmgrV1alpha1().Addons(testNamespace).Delete(ctx, updated.Name, metav1.DeleteOptions{})).NotTo(HaveOccurred())
			Eventually(func() error {
				_, err := addonController.addoncli.AddonmgrV1alpha1().Addons(testNamespace).Get(ctx, updated.Name, metav1.GetOptions{})
				Expect(err).NotTo(BeNil())

				return nil
			}, timeout).Should(Succeed())

			By("Verify delete workflow was generated")
			wfName := instance.GetFormattedWorkflowName(v1alpha1.Delete)
			Eventually(func() error {
				wf, err := addonController.wfcli.ArgoprojV1alpha1().Workflows(addonNamespace).Get(ctx, wfName, metav1.GetOptions{})
				Expect(err).NotTo(BeNil())
				Expect(wf).To(BeNil())
				return nil
			}, timeout).Should(Succeed())

		})

		It("instance with dependencies should succeed", func() {
			instance = &v1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{Name: "addon-1", Namespace: addonNamespace},
				Spec: v1alpha1.AddonSpec{
					PackageSpec: v1alpha1.PackageSpec{
						PkgType:    v1alpha1.CompositePkg,
						PkgName:    "test/addon-1",
						PkgVersion: "1.0.1",
					},
					Params: v1alpha1.AddonParams{
						Namespace: "addon-test-ns",
					},
				},
			}
			var instance2 = &v1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{Name: "addon-2", Namespace: addonNamespace},
				Spec: v1alpha1.AddonSpec{
					PackageSpec: v1alpha1.PackageSpec{
						PkgType:    v1alpha1.CompositePkg,
						PkgName:    "test/addon-2",
						PkgVersion: "1.0.0",
						PkgDeps: map[string]string{
							"test/addon-1": "*",
						},
					},
					Params: v1alpha1.AddonParams{
						Namespace: "addon-test-ns",
					},
				},
			}

			By("Verify first addon-2 that depends on addon-1 is created and has validation failed state")
			createdInstance2, err := addonController.addoncli.AddonmgrV1alpha1().Addons(testNamespace).Create(ctx, instance2, metav1.CreateOptions{})
			Expect(err).To(BeNil())
			Expect(createdInstance2).NotTo(BeNil())
			err = addonController.handleAddonCreation(ctx, instance2)
			Expect(err).NotTo(BeNil())

			defer addonController.addoncli.AddonmgrV1alpha1().Addons(testNamespace).Delete(ctx, instance2.Name, metav1.DeleteOptions{})
			var fetchedInstance2 *v1alpha1.Addon
			Eventually(func() error {
				fetchedInstance2, err = addonController.addoncli.AddonmgrV1alpha1().Addons(testNamespace).Get(ctx, instance2.Name, metav1.GetOptions{})
				if err != nil || fetchedAddon == nil {
					return err
				}

				if fetchedInstance2.Status.Lifecycle.Installed == v1alpha1.ValidationFailed {
					return nil
				}

				return fmt.Errorf("addon-2 is not in validation failed state")
			}, timeout).Should(Succeed())

			By("Verify addon-1 is submitted and completes successfully")
			createdInstance1, err := addonController.addoncli.AddonmgrV1alpha1().Addons(testNamespace).Create(ctx, instance, metav1.CreateOptions{})
			Expect(err).To(BeNil())
			Expect(createdInstance1).NotTo(BeNil())
			err = addonController.handleAddonCreation(ctx, instance)
			Expect(err).To(BeNil())
			defer addonController.addoncli.AddonmgrV1alpha1().Addons(testNamespace).Delete(ctx, instance.Name, metav1.DeleteOptions{})

			Eventually(func() error {
				fetchedInstance, err := addonController.addoncli.AddonmgrV1alpha1().Addons(testNamespace).Get(ctx, instance.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())

				if fetchedInstance.Status.Lifecycle.Installed == v1alpha1.Succeeded {
					return nil
				}

				return fmt.Errorf("addon-1 is not installed")
			}, timeout).Should(Succeed())

			By("Verify addon-2 succeeds after addon-1 completed")
			err = addonController.handleAddonUpdate(ctx, fetchedInstance2)
			Expect(err).To(BeNil())
			Eventually(func() error {
				updatedInstance2, err := addonController.addoncli.AddonmgrV1alpha1().Addons(testNamespace).Get(ctx, instance2.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(fetchedInstance2).NotTo(BeNil())

				if updatedInstance2.Status.Lifecycle.Installed == v1alpha1.Succeeded {
					return nil
				}
				return fmt.Errorf("addon-2 is not valid after resolving dependencies.")
			}, timeout).Should(Succeed())
		})

	})
})
