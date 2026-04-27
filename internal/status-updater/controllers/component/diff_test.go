package component

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ = Describe("DiffDeployments", func() {
	It("should create resources that don't exist yet", func() {
		desired := []runtime.Object{
			&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "dep-a", Namespace: "ns"}},
		}
		actual := []appsv1.Deployment{}

		diff := DiffDeployments(desired, actual)
		Expect(diff.ToCreate).To(HaveLen(1))
		Expect(diff.ToUpdate).To(BeEmpty())
		Expect(diff.ToDelete).To(BeEmpty())
	})

	It("should delete resources that are no longer desired", func() {
		desired := []runtime.Object{}
		actual := []appsv1.Deployment{
			{ObjectMeta: metav1.ObjectMeta{
				Name:      "dep-old",
				Namespace: "ns",
				Labels: map[string]string{
					constants.LabelManagedBy: constants.LabelManagedByValue,
				},
			}},
		}

		diff := DiffDeployments(desired, actual)
		Expect(diff.ToCreate).To(BeEmpty())
		Expect(diff.ToDelete).To(HaveLen(1))
		Expect(diff.ToDelete[0].Name).To(Equal("dep-old"))
	})

	It("should update resources that exist but have changed images", func() {
		desired := []runtime.Object{
			&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "dep-a", Namespace: "ns"},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{Image: "new:v2"}},
						},
					},
				},
			},
		}
		actual := []appsv1.Deployment{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "dep-a", Namespace: "ns"},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{Image: "old:v1"}},
						},
					},
				},
			},
		}

		diff := DiffDeployments(desired, actual)
		Expect(diff.ToCreate).To(BeEmpty())
		Expect(diff.ToDelete).To(BeEmpty())
		Expect(diff.ToUpdate).To(HaveLen(1))
	})

	It("should not update resources that haven't changed", func() {
		desired := []runtime.Object{
			&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "dep-a", Namespace: "ns"},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{Image: "same:v1"}},
						},
					},
				},
			},
		}
		actual := []appsv1.Deployment{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "dep-a", Namespace: "ns"},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{Image: "same:v1"}},
						},
					},
				},
			},
		}

		diff := DiffDeployments(desired, actual)
		Expect(diff.ToCreate).To(BeEmpty())
		Expect(diff.ToDelete).To(BeEmpty())
		Expect(diff.ToUpdate).To(BeEmpty())
	})
})

var _ = Describe("DiffServices", func() {
	It("should create services that don't exist", func() {
		desired := []runtime.Object{
			&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "svc-a", Namespace: "ns"}},
		}
		diff := DiffServices(desired, []corev1.Service{})
		Expect(diff.ToCreate).To(HaveLen(1))
		Expect(diff.ToDelete).To(BeEmpty())
	})

	It("should delete services no longer desired", func() {
		actual := []corev1.Service{
			{ObjectMeta: metav1.ObjectMeta{Name: "svc-old", Namespace: "ns"}},
		}
		diff := DiffServices([]runtime.Object{}, actual)
		Expect(diff.ToCreate).To(BeEmpty())
		Expect(diff.ToDelete).To(HaveLen(1))
	})
})
