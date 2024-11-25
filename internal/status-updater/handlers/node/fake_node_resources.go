package node

import (
	"context"
	"fmt"
	"os"

	"github.com/hashicorp/go-multierror"
	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	kubeutil "github.com/run-ai/fake-gpu-operator/internal/common/util/kubernetes"
	"github.com/spf13/viper"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func (p *NodeHandler) applyFakeNodeResources(node *v1.Node) error {
	if !isFakeNode(node) {
		return nil
	}

	deployments, err := p.generateFakeNodeDeployments(node)
	if err != nil {
		return fmt.Errorf("failed to get fake node deployments: %w", err)
	}

	for _, deployment := range deployments {
		err := kubeutil.ApplyDeployment(p.kubeClient, deployment)
		if err != nil {
			return fmt.Errorf("failed to apply deployment: %w", err)
		}
	}

	dcgmExporterDummyPod := p.generateDcgmExporterDummyPod(node)
	err = kubeutil.ApplyPod(p.kubeClient, dcgmExporterDummyPod)
	if err != nil {
		return fmt.Errorf("failed to apply dcgm-exporter dummy pod: %w", err)
	}

	return nil
}

func (p *NodeHandler) deleteFakeNodeResources(node *v1.Node) error {
	if !isFakeNode(node) {
		return nil
	}

	var multiErr error

	err := p.kubeClient.AppsV1().Deployments(os.Getenv(constants.EnvFakeGpuOperatorNs)).DeleteCollection(context.TODO(), metav1.DeleteOptions{}, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", constants.LabelNodeName, node.Name),
	})
	if err != nil && !errors.IsNotFound(err) {
		multiErr = multierror.Append(multiErr, fmt.Errorf("failed to delete fake node deployments: %w", err))
	}

	err = p.kubeClient.CoreV1().Pods(viper.GetString(constants.EnvFakeGpuOperatorNs)).DeleteCollection(context.TODO(), metav1.DeleteOptions{}, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=true,%s=%s", constants.LabelDcgmExporterDummyPod, constants.LabelNodeName, node.Name),
	})
	if err != nil && !errors.IsNotFound(err) {
		multiErr = multierror.Append(multiErr, fmt.Errorf("failed to delete dcgm-exporter dummy pod: %w", err))
	}

	return multiErr
}

func (p *NodeHandler) generateDcgmExporterDummyPod(node *v1.Node) v1.Pod {
	return v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("nvidia-dcgm-exporter-%s", node.Name),
			Namespace: viper.GetString(constants.EnvFakeGpuOperatorNs),
			Labels: map[string]string{
				constants.LabelDcgmExporterDummyPod: "true",
				constants.LabelNodeName:             node.Name,
			},
		},
		Spec: v1.PodSpec{
			TerminationGracePeriodSeconds: ptr.To(int64(0)),
			NodeName:                      node.Name,
			Containers: []v1.Container{
				{
					Name:  "sleeper",
					Image: "busybox",
					Command: []string{
						"sh",
						"-c",
						"sleep infinity",
					},
					Resources: v1.ResourceRequirements{
						Limits: v1.ResourceList{
							v1.ResourceMemory: resource.MustParse("50Mi"),
							v1.ResourceCPU:    resource.MustParse("10m"),
						},
						Requests: v1.ResourceList{
							v1.ResourceMemory: resource.MustParse("0Mi"),
							v1.ResourceCPU:    resource.MustParse("0m"),
						},
					},
				},
			},
		},
	}
}

func (p *NodeHandler) generateFakeNodeDeployments(node *v1.Node) ([]appsv1.Deployment, error) {
	deploymentTemplates, err := p.kubeClient.AppsV1().Deployments(os.Getenv(constants.EnvFakeGpuOperatorNs)).List(context.TODO(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=true", constants.LabelFakeNodeDeploymentTemplate),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments: %w", err)
	}

	deployments := []appsv1.Deployment{}
	for i := range deploymentTemplates.Items {
		deployments = append(deployments, *generateFakeNodeDeploymentFromTemplate(&deploymentTemplates.Items[i], node))
	}

	return deployments, nil
}

func generateFakeNodeDeploymentFromTemplate(template *appsv1.Deployment, node *v1.Node) *appsv1.Deployment {
	deployment := template.DeepCopy()

	delete(deployment.Labels, constants.LabelFakeNodeDeploymentTemplate)
	deployment.Name = fmt.Sprintf("%s-%s", deployment.Name, node.Name)
	deployment.Labels[constants.LabelNodeName] = node.Name
	deployment.Spec.Replicas = ptr.To(int32(1))
	deployment.Spec.Template.Spec.Containers[0].Env = append(deployment.Spec.Template.Spec.Containers[0].Env, v1.EnvVar{
		Name:  constants.EnvNodeName,
		Value: node.Name,
	}, v1.EnvVar{
		Name:  constants.EnvFakeNode,
		Value: "true",
	})

	deployment.Spec.Template.Spec.Containers[0].Resources.Limits = v1.ResourceList{
		v1.ResourceMemory: resource.MustParse("100Mi"),
		v1.ResourceCPU:    resource.MustParse("50m"),
	}
	deployment.Spec.Template.Spec.Containers[0].Resources.Requests = v1.ResourceList{
		v1.ResourceMemory: resource.MustParse("20Mi"),
		v1.ResourceCPU:    resource.MustParse("10m"),
	}

	return deployment
}

func isFakeNode(node *v1.Node) bool {
	return node != nil && node.Annotations[constants.AnnotationKwokNode] == "fake"
}
