package kubernetes

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"
)

// TODO: Replace with a generic apply
func ApplyDeployment(k8s kubernetes.Interface, deployment appsv1.Deployment) error {
	existingDeployment, err := k8s.AppsV1().Deployments(deployment.Namespace).Get(context.TODO(), deployment.Name, metav1.GetOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to get deployment %s: %w", deployment.Name, err)
	}

	if errors.IsNotFound(err) {
		deployment.ResourceVersion = ""
		_, err := k8s.AppsV1().Deployments(deployment.Namespace).Create(context.TODO(), &deployment, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create deployment %s: %w", deployment.Name, err)
		}
	} else {
		deployment.UID = existingDeployment.UID
		deployment.ResourceVersion = existingDeployment.ResourceVersion
		_, err := k8s.AppsV1().Deployments(deployment.Namespace).Update(context.TODO(), &deployment, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update deployment %s: %w", deployment.Name, err)
		}
	}

	return nil
}

// TODO: Replace with a generic apply
func ApplyPod(k8s kubernetes.Interface, pod corev1.Pod) error {
	existingPod, err := k8s.CoreV1().Pods(pod.Namespace).Get(context.TODO(), pod.Name, metav1.GetOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to get pod %s: %w", pod.Name, err)
	}

	if errors.IsNotFound(err) {
		pod.ResourceVersion = ""
		_, err := k8s.CoreV1().Pods(pod.Namespace).Create(context.TODO(), &pod, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create pod %s: %w", pod.Name, err)
		}
	} else {
		pod.UID = existingPod.UID
		pod.ResourceVersion = existingPod.ResourceVersion
		_, err := k8s.CoreV1().Pods(pod.Namespace).Update(context.TODO(), &pod, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update pod %s: %w", pod.Name, err)
		}
	}

	return nil
}
