package controllers

import (
	"fmt"
	"io/ioutil"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v2"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	//. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/keikoproj/addon-manager/api/addon/v1alpha1"
	"github.com/keikoproj/addon-manager/pkg/client/clientset/versioned/scheme"
)

const (
	addonName     = "cluster-autoscaler"
	testNamespace = "default"
)

var fetched *v1alpha1.Addon

const timeout = time.Second * 5

var _ = Describe("AddonController", func() {

	var (
		instance *v1alpha1.Addon
	)

	var _ = BeforeSuite(func() {
		logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

		addonController = getTestController()
	})

	var _ = AfterSuite(func() {
		<-stopCh

		By("tearing down the test environment")
		//close(stopCh)
		//err := testEnv.Stop()
		//Expect(err).ToNot(HaveOccurred())
	})

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
			created, err := addonController.addoncli.AddonmgrV1alpha1().Addons(testNamespace).Create(ctx, instance, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(created).NotTo(BeNil())

			fetchedAddon, err := addonController.addoncli.AddonmgrV1alpha1().Addons(testNamespace).Get(ctx, instance.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(err).NotTo(HaveOccurred())

			By("Verify addon has been reconciled by checking for checksum status")
			Expect(fetchedAddon.Status.Checksum).ShouldNot(BeEmpty())

			By("Verify addon has finalizers added which means it's valid")
			Expect(fetchedAddon.ObjectMeta.Finalizers).Should(Equal([]string{"delete.addonmgr.keikoproj.io"}))

			oldCheckSum := instance.Status.Checksum
			//Update instance params for checksum validation
			instance.Spec.Params.Context.ClusterRegion = "us-east-2"
			//err = k8sClient.Update(context.TODO(), instance)
			_, err = addonController.addoncli.AddonmgrV1alpha1().Addons(testNamespace).Update(ctx, fetchedAddon, metav1.UpdateOptions{})
			if apierrors.IsInvalid(err) {
				addonController.logger.Error(err, "failed to update object, got an invalid object error")
				return
			}
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() error {
				fetched, err = addonController.addoncli.AddonmgrV1alpha1().Addons(testNamespace).Get(ctx, instance.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}

				if len(fetched.ObjectMeta.Finalizers) > 0 {
					return nil
				}
				return fmt.Errorf("addon is not valid")
			}, timeout).Should(Succeed())

			By("Verify changing addon spec generates new checksum")
			Expect(fetched.Status.Checksum).ShouldNot(BeIdenticalTo(oldCheckSum))

			By("Verify addon has workflows generated with new checksum name")
			wfName := fetched.GetFormattedWorkflowName(v1alpha1.Prereqs)
			Eventually(func() error {
				wfv1, err := addonController.wfcli.ArgoprojV1alpha1().Workflows(testNamespace).Get(ctx, wfName, metav1.GetOptions{})
				if err != nil {
					return err
				}
				Expect(wfv1.GetName()).Should(Equal(wfName))
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
			fetched.ObjectMeta.DeletionTimestamp = &metav1.Time{Time: metav1.Now().Time}
			err := addonController.addoncli.AddonmgrV1alpha1().Addons(testNamespace).Delete(ctx, fetched.Name, metav1.DeleteOptions{})
			Expect(err).To(BeNil())
			Eventually(func() error {
				updated, err := addonController.addoncli.AddonmgrV1alpha1().Addons(testNamespace).Get(ctx, fetched.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}

				if updated.Status.Lifecycle.Installed == v1alpha1.Deleting {
					return nil
				}
				return fmt.Errorf("addon is not being deleted")
			}, timeout).Should(Succeed())

			By("Verify delete workflow was generated")
			wfName := fetched.GetFormattedWorkflowName(v1alpha1.Delete)
			Eventually(func() error {
				wfv1, err := addonController.wfcli.ArgoprojV1alpha1().Workflows(testNamespace).Get(ctx, wfName, metav1.GetOptions{})
				if err != nil {
					return err
				}
				Expect(wfv1.GetName()).Should(Equal(wfName))
				return nil
			}, timeout).Should(Succeed())
		})

		It("instance with dependencies should succeed", func() {
			instance = &v1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{Name: "addon-1", Namespace: testNamespace},
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
				ObjectMeta: metav1.ObjectMeta{Name: "addon-2", Namespace: testNamespace},
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
			_, err := addonController.addoncli.AddonmgrV1alpha1().Addons(testNamespace).Create(ctx, instance2, metav1.CreateOptions{})
			Expect(err).To(BeNil())
			err = addonController.handleAddonCreation(ctx, instance2)
			Expect(err).NotTo(BeNil())
			var fetchedInstance2 *v1alpha1.Addon
			Eventually(func() error {
				fetchedInstance2, err = addonController.addoncli.AddonmgrV1alpha1().Addons(testNamespace).Get(ctx, instance2.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}

				if fetchedInstance2.Status.Lifecycle.Installed == v1alpha1.DepNotInstalled {
					return nil
				}
				return fmt.Errorf("addon-2 is not in validation failed state")
			}, timeout).Should(Succeed())

			By("Verify addon-1 is submitted and completes successfully")

			_, err = addonController.addoncli.AddonmgrV1alpha1().Addons(testNamespace).Create(ctx, instance, metav1.CreateOptions{})
			Expect(err).To(BeNil())
			err = addonController.handleAddonCreation(ctx, instance)
			Expect(err).To(BeNil())
			defer addonController.addoncli.AddonmgrV1alpha1().Addons(testNamespace).Delete(ctx, instance.Name, metav1.DeleteOptions{})
			Eventually(func() error {
				instance, err := addonController.addoncli.AddonmgrV1alpha1().Addons(testNamespace).Get(ctx, instance.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}

				if instance.Status.Lifecycle.Installed == v1alpha1.Succeeded {
					return nil
				}

				return fmt.Errorf("addon-1 is not installed")
			}, timeout).Should(Succeed())

			By("Verify addon-2 succeeds after addon-1 completed")
			err = addonController.handleAddonUpdate(ctx, fetchedInstance2)
			Expect(err).To(BeNil())
			Eventually(func() error {
				instance2, err := addonController.addoncli.AddonmgrV1alpha1().Addons(testNamespace).Get(ctx, instance2.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}

				if instance2.Status.Lifecycle.Installed == v1alpha1.Succeeded {
					return nil
				}

				return fmt.Errorf("addon-2 is not valid")
			}, timeout*10).Should(Succeed())
		})

	})
})

func parseAddonYaml(data []byte) (*v1alpha1.Addon, error) {
	var err error
	o := &unstructured.Unstructured{}
	err = yaml.Unmarshal(data, &o.Object)
	if err != nil {
		return nil, err
	}
	a := &v1alpha1.Addon{}
	err = scheme.Scheme.Convert(o, a, 0)
	if err != nil {
		return nil, err
	}

	return a, nil
}
