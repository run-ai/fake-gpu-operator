package node

import (
	"context"
	"fmt"
	"time"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/spf13/viper"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/utils/ptr"
)

const (
	dummyDcgmExporterPodTimeout = 20 * time.Second
)

func (p *NodeHandler) applyFakeNodeDeployments(node *v1.Node) error {
	if !isFakeNode(node) {
		return nil
	}

	deployments, err := p.generateFakeNodeDeployments(node)
	if err != nil {
		return fmt.Errorf("failed to get fake node deployments: %w", err)
	}

	for _, deployment := range deployments {
		err := p.applyDeployment(deployment)
		if err != nil {
			return fmt.Errorf("failed to apply deployment: %w", err)
		}
	}

	return nil
}

func (p *NodeHandler) deleteFakeNodeDeployments(node *v1.Node) error {
	if !isFakeNode(node) {
		return nil
	}

	deployments, err := p.generateFakeNodeDeployments(node)
	if err != nil {
		return fmt.Errorf("failed to get fake node deployments: %w", err)
	}

	for _, deployment := range deployments {
		err := p.kubeClient.AppsV1().Deployments(deployment.Namespace).Delete(context.TODO(), deployment.Name, metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete deployment %s: %w", deployment.Name, err)
		}
	}

	return nil
}

func (p *NodeHandler) generateFakeNodeDeployments(node *v1.Node) ([]appsv1.Deployment, error) {
	deploymentTemplates, err := p.kubeClient.AppsV1().Deployments(viper.GetString(constants.EnvFakeGpuOperatorNs)).List(context.TODO(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=true", constants.LabelFakeNodeDeploymentTemplate),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments: %w", err)
	}

	deployments := []appsv1.Deployment{}
	for i := range deploymentTemplates.Items {
		generatedDeployment, err := p.generateFakeNodeDeploymentFromTemplate(&deploymentTemplates.Items[i], node)
		if err != nil {
			return nil, fmt.Errorf("failed to generate fake node deployment from template: %w", err)
		}
		deployments = append(deployments, *generatedDeployment)
	}

	return deployments, nil
}

func (p *NodeHandler) applyDeployment(deployment appsv1.Deployment) error {
	existingDeployment, err := p.kubeClient.AppsV1().Deployments(deployment.Namespace).Get(context.TODO(), deployment.Name, metav1.GetOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to get deployment %s: %w", deployment.Name, err)
	}

	if errors.IsNotFound(err) {
		deployment.ResourceVersion = ""
		_, err := p.kubeClient.AppsV1().Deployments(deployment.Namespace).Create(context.TODO(), &deployment, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create deployment %s: %w", deployment.Name, err)
		}
	} else {
		deployment.UID = existingDeployment.UID
		deployment.ResourceVersion = existingDeployment.ResourceVersion
		_, err := p.kubeClient.AppsV1().Deployments(deployment.Namespace).Update(context.TODO(), &deployment, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update deployment %s: %w", deployment.Name, err)
		}
	}

	return nil
}

func (p *NodeHandler) generateFakeNodeDeploymentFromTemplate(template *appsv1.Deployment, node *v1.Node) (*appsv1.Deployment, error) {
	dummyDcgmExporterPod, err := p.getDummyDcgmExporterPod(node.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get dummy dcgm exporter: %w", err)
	}

	deployment := template.DeepCopy()

	delete(deployment.Labels, constants.LabelFakeNodeDeploymentTemplate)
	deployment.Name = fmt.Sprintf("%s-%s", deployment.Name, node.Name)
	deployment.Labels[constants.LabelFakeNodeDeployment] = "true"
	deployment.Spec.Replicas = ptr.To(int32(1))

	deployment.Spec.Selector.MatchLabels[constants.LabelApp] = constants.KwokDCGMExporterApp
	deployment.Spec.Template.Labels[constants.LabelApp] = constants.KwokDCGMExporterApp
	deployment.Spec.Template.Spec.Containers[0].Env = append(deployment.Spec.Template.Spec.Containers[0].Env, v1.EnvVar{
		Name:  constants.EnvNodeName,
		Value: node.Name,
	}, v1.EnvVar{
		Name:  constants.EnvFakeNode,
		Value: "true",
	}, v1.EnvVar{
		Name:  constants.EnvImpersonatePodIP,
		Value: dummyDcgmExporterPod.Status.PodIP,
	}, v1.EnvVar{
		Name:  constants.EnvImpersonatePodName,
		Value: dummyDcgmExporterPod.Name,
	}, v1.EnvVar{
		Name:  constants.EnvExportPrometheusLabelEnrichments,
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

	return deployment, nil
}

func (p *NodeHandler) getDummyDcgmExporterPod(nodeName string) (*v1.Pod, error) {
	labelSelector := fmt.Sprintf("%s=%s", constants.LabelApp, constants.DCGMExporterApp)
	fieldSelector := fields.OneTermEqualSelector("spec.nodeName", nodeName).String()

	ctx, cancel := context.WithTimeout(context.Background(), dummyDcgmExporterPodTimeout)
	defer cancel()

	watcher, err := p.kubeClient.CoreV1().Pods(viper.GetString(constants.EnvFakeGpuOperatorNs)).Watch(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
		FieldSelector: fieldSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create pod watcher: %w", err)
	}
	defer watcher.Stop()

	// Wait for the pod to be ready
	for {
		select {
		case event := <-watcher.ResultChan():
			pod, ok := event.Object.(*v1.Pod)
			if !ok {
				return nil, fmt.Errorf("unexpected object type: %T", event.Object)
			}
			if event.Type == watch.Added || event.Type == watch.Modified {
				if pod.Status.Phase == v1.PodRunning {
					return pod, nil
				}
			}
		case <-ctx.Done():
			return nil, fmt.Errorf("timeout waiting for pod to be ready")
		}
	}
}
