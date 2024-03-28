package node

import (
	"context"
	"fmt"
	"os"
	"path"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

func (p *NodeHandler) createFakeNodeDeployments(node *v1.Node) error {
	fmt.Printf("Creating fake node deployments for node %s\n", node.Name)
	if !isFakeNode(node) {
		return nil
	}

	deployments, err := p.getFakeNodeDeployments(node)
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

func (p *NodeHandler) getFakeNodeDeployments(node *v1.Node) ([]appsv1.Deployment, error) {
	deploymentsPath := os.Getenv("FAKE_NODE_DEPLOYMENTS_PATH")

	deploymentFiles, err := os.ReadDir(deploymentsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read fake node deployments directory: %w", err)
	}

	var deployments []appsv1.Deployment
	for _, dirEntry := range deploymentFiles {
		if !dirEntry.Type().IsRegular() {
			continue
		}

		deployment, err := readDeploymentFile(path.Join(deploymentsPath, dirEntry.Name()))
		if err != nil {
			return nil, fmt.Errorf("failed to read deployment file %s: %w", dirEntry.Name(), err)
		}

		enrichFakeNodeDeployment(&deployment, node)
		deployments = append(deployments, deployment)
	}

	return deployments, nil
}

func (p *NodeHandler) applyDeployment(deployment appsv1.Deployment) error {
	existingDeployment, err := p.kubeClient.AppsV1().Deployments(deployment.Namespace).Get(context.TODO(), deployment.Name, metav1.GetOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to get deployment %s: %w", deployment.Name, err)
	}

	if errors.IsNotFound(err) {
		_, err := p.kubeClient.AppsV1().Deployments(deployment.Namespace).Create(context.TODO(), &deployment, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create deployment %s: %w", deployment.Name, err)
		}
	} else {
		deployment.ResourceVersion = existingDeployment.ResourceVersion
		_, err := p.kubeClient.AppsV1().Deployments(deployment.Namespace).Update(context.TODO(), &deployment, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update deployment %s: %w", deployment.Name, err)
		}
	}

	return nil
}

func readDeploymentFile(path string) (appsv1.Deployment, error) {
	fileContent, err := os.ReadFile(path)
	if err != nil {
		return appsv1.Deployment{}, fmt.Errorf("failed to read deployment file %s: %w", path, err)
	}

	var deployment appsv1.Deployment
	err = yaml.Unmarshal(fileContent, &deployment)
	if err != nil {
		return appsv1.Deployment{}, fmt.Errorf("failed to unmarshal deployment file %s: %w", path, err)
	}

	return deployment, nil
}

func enrichFakeNodeDeployment(deployment *appsv1.Deployment, node *v1.Node) {
	deployment.Name = fmt.Sprintf("%s-%s", deployment.Name, node.Name)
	deployment.Spec.Template.Spec.Containers[0].Env = append(deployment.Spec.Template.Spec.Containers[0].Env, v1.EnvVar{
		Name:  constants.EnvNodeName,
		Value: node.Name,
	})
}

func isFakeNode(node *v1.Node) bool {
	return node != nil && node.Annotations[constants.KwokNodeAnnotation] == "fake"
}
