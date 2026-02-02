package integration_test

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	computedomainv1beta1 "github.com/NVIDIA/k8s-dra-driver-gpu/api/nvidia.com/resource/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/run-ai/fake-gpu-operator/pkg/compute-domain/consts"
	resourceapi "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Compute Domain Controller Integration Tests", func() {
	var testNamespaces []string

	AfterEach(func() {
		for _, ns := range testNamespaces {
			deleteNamespace(ns)
		}

		for _, ns := range testNamespaces {
			Eventually(func() error {
				_, err := kubeClient.CoreV1().Namespaces().Get(context.Background(), ns, metav1.GetOptions{})
				if err != nil {
					// If namespace is not found, it means it was successfully deleted
					return err
				}
				return nil
			}).WithTimeout(60*time.Second).ShouldNot(Succeed(), "Namespace %s should be deleted", ns)
		}

		testNamespaces = nil
	})

	Describe("Basic ComputeDomain Creation", func() {
		It("should create ResourceClaimTemplate for ComputeDomain", func() {
			manifestPath := "manifests/compute-domain-basic.yaml"
			namespace := "compute-domain-test-compute-domain-basic"
			computeDomainName := "test-domain"
			templateName := "test-domain-template"

			setupTest(manifestPath, namespace, &testNamespaces)

			// Wait for ComputeDomain to be created
			Eventually(func() error {
				_, err := nvidiaClient.ResourceV1beta1().ComputeDomains(namespace).Get(
					context.Background(), computeDomainName, metav1.GetOptions{})
				return err
			}).WithTimeout(30 * time.Second).Should(Succeed())

			// Wait for ResourceClaimTemplate to be created by the controller
			Eventually(func() error {
				_, err := kubeClient.ResourceV1().ResourceClaimTemplates(namespace).Get(
					context.Background(), templateName, metav1.GetOptions{})
				return err
			}).WithTimeout(30 * time.Second).Should(Succeed())

			// Verify ResourceClaimTemplate details
			rct, err := kubeClient.ResourceV1().ResourceClaimTemplates(namespace).Get(
				context.Background(), templateName, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			// Check that it has the correct labels
			Expect(rct.Labels).To(HaveKeyWithValue(consts.ComputeDomainTemplateLabel, computeDomainName))
			Expect(rct.Labels).To(HaveKeyWithValue(consts.ComputeDomainTemplateTargetLabel, "workload"))

			// Check ResourceClaimTemplate spec
			Expect(rct.Spec.Spec.Devices.Requests).To(HaveLen(1))
			request := rct.Spec.Spec.Devices.Requests[0]
			Expect(request.Name).To(Equal("channel"))
			Expect(request.Exactly).NotTo(BeNil())
			Expect(request.Exactly.AllocationMode).To(Equal(resourceapi.DeviceAllocationModeExactCount))
			Expect(request.Exactly.Count).To(Equal(int64(1)))
		})
	})

	Describe("ComputeDomain with Pod", func() {
		It("should allocate ComputeDomain to pod using ComputeDomain ResourceClaimTemplate", func() {
			manifestPath := "manifests/compute-domain-with-pod.yaml"
			namespace := "compute-domain-test-compute-domain-pod"
			computeDomainName := "test-domain-pod"
			podName := "pod0"
			templateName := "test-domain-template"

			setupTest(manifestPath, namespace, &testNamespaces)

			// Wait for ComputeDomain to be created
			Eventually(func() error {
				_, err := nvidiaClient.ResourceV1beta1().ComputeDomains(namespace).Get(
					context.Background(), computeDomainName, metav1.GetOptions{})
				return err
			}).WithTimeout(30 * time.Second).Should(Succeed())

			// Wait for ResourceClaimTemplate to be created by the controller
			Eventually(func() error {
				_, err := kubeClient.ResourceV1().ResourceClaimTemplates(namespace).Get(
					context.Background(), templateName, metav1.GetOptions{})
				return err
			}).WithTimeout(30 * time.Second).Should(Succeed())

			// Wait for pod to be created
			Eventually(func() error {
				_, err := kubeClient.CoreV1().Pods(namespace).Get(
					context.Background(), podName, metav1.GetOptions{})
				return err
			}).WithTimeout(30 * time.Second).Should(Succeed())

			// Wait for ResourceClaim to be created for the pod
			var claimName string
			Eventually(func() error {
				claimNames, err := getResourceClaimNameFromPod(namespace, podName)
				if err != nil {
					return err
				}
				if len(claimNames) == 0 {
					return fmt.Errorf("no ResourceClaims found for pod")
				}
				claimName = claimNames[0]
				return nil
			}).WithTimeout(30 * time.Second).Should(Succeed())

			// Wait for ResourceClaim to be allocated
			Eventually(func() error {
				return waitForResourceClaimAllocated(namespace, claimName, 30*time.Second)
			}).WithTimeout(60 * time.Second).Should(Succeed())

			// Wait for pod to be ready
			waitForPodReady(namespace, podName, podReadyTimeout)

			// Verify ComputeDomain status shows the allocated node
			Eventually(func() error {
				cd, err := nvidiaClient.ResourceV1beta1().ComputeDomains(namespace).Get(
					context.Background(), computeDomainName, metav1.GetOptions{})
				if err != nil {
					return err
				}
				if len(cd.Status.Nodes) == 0 {
					return fmt.Errorf("no nodes in ComputeDomain status")
				}
				if cd.Status.Status != "Ready" {
					return fmt.Errorf("ComputeDomain status is %s, expected Ready", cd.Status.Status)
				}
				return nil
			}).WithTimeout(30 * time.Second).Should(Succeed())
		})
	})

	Describe("ComputeDomain Deletion", func() {
		It("should clean up ResourceClaimTemplate when ComputeDomain is deleted", func() {
			manifestPath := "manifests/compute-domain-basic.yaml"
			namespace := "compute-domain-test-compute-domain-basic"
			computeDomainName := "test-domain"
			templateName := "test-domain-template"

			setupTest(manifestPath, namespace, &testNamespaces)

			// Wait for ComputeDomain to be created
			Eventually(func() error {
				_, err := nvidiaClient.ResourceV1beta1().ComputeDomains(namespace).Get(
					context.Background(), computeDomainName, metav1.GetOptions{})
				return err
			}).WithTimeout(30 * time.Second).Should(Succeed())

			// Wait for ResourceClaimTemplate to be created by the controller
			Eventually(func() error {
				_, err := kubeClient.ResourceV1().ResourceClaimTemplates(namespace).Get(
					context.Background(), templateName, metav1.GetOptions{})
				return err
			}).WithTimeout(30 * time.Second).Should(Succeed())

			// Delete the ComputeDomain
			err := nvidiaClient.ResourceV1beta1().ComputeDomains(namespace).Delete(
				context.Background(), computeDomainName, metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())

			// Wait for ResourceClaimTemplate to be deleted
			Eventually(func() error {
				_, err := kubeClient.ResourceV1().ResourceClaimTemplates(namespace).Get(
					context.Background(), templateName, metav1.GetOptions{})
				return err
			}).WithTimeout(30 * time.Second).ShouldNot(Succeed())
		})
	})

	Describe("ComputeDomain with All Allocation Mode", func() {
		It("should create ResourceClaimTemplate with All allocation mode", func() {
			// Create a ComputeDomain with All allocation mode directly using API
			namespace := "compute-domain-test-compute-domain-basic"
			computeDomainName := "test-domain-all"
			templateName := "test-domain-template"

			// Create namespace using our helper
			nsManifest := fmt.Sprintf(`
apiVersion: v1
kind: Namespace
metadata:
  name: %s
`, namespace)

			applyManifestFromString(nsManifest)
			testNamespaces = append(testNamespaces, namespace)

			// Create ComputeDomain with All allocation mode
			computeDomain := &computedomainv1beta1.ComputeDomain{
				ObjectMeta: metav1.ObjectMeta{
					Name:      computeDomainName,
					Namespace: namespace,
				},
				Spec: computedomainv1beta1.ComputeDomainSpec{
					NumNodes: 0,
					Channel: &computedomainv1beta1.ComputeDomainChannelSpec{
						AllocationMode: "All",
						ResourceClaimTemplate: computedomainv1beta1.ComputeDomainResourceClaimTemplate{
							Name: templateName,
						},
					},
				},
			}

			_, err := nvidiaClient.ResourceV1beta1().ComputeDomains(namespace).Create(
				context.Background(), computeDomain, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			// Wait for ResourceClaimTemplate to be created by the controller
			Eventually(func() error {
				_, err := kubeClient.ResourceV1().ResourceClaimTemplates(namespace).Get(
					context.Background(), templateName, metav1.GetOptions{})
				return err
			}).WithTimeout(30 * time.Second).Should(Succeed())

			// Verify ResourceClaimTemplate details
			rct, err := kubeClient.ResourceV1().ResourceClaimTemplates(namespace).Get(
				context.Background(), templateName, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			// Check that the opaque parameters contain the correct allocation mode
			Expect(rct.Spec.Spec.Devices.Config).To(HaveLen(1))
			config := rct.Spec.Spec.Devices.Config[0]
			Expect(config.Opaque).NotTo(BeNil())
			// Check for allocationMode All (allowing for whitespace variations)
			Expect(string(config.Opaque.Parameters.Raw)).To(ContainSubstring(`"allocationMode"`))
			Expect(string(config.Opaque.Parameters.Raw)).To(ContainSubstring(`"All"`))
		})
	})
})

// Helper function to apply a manifest from string
func applyManifestFromString(manifest string) {
	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	output, err := cmd.CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "Failed to apply manifest: %s", string(output))
}
