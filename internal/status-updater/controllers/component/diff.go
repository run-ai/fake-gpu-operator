package component

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DeploymentDiff holds the result of comparing desired vs actual deployments.
type DeploymentDiff struct {
	ToCreate []*appsv1.Deployment
	ToUpdate []*appsv1.Deployment
	ToDelete []appsv1.Deployment
}

// ServiceDiff holds the result of comparing desired vs actual services.
type ServiceDiff struct {
	ToCreate []*corev1.Service
	ToUpdate []*corev1.Service
	ToDelete []corev1.Service
}

// DiffDeployments compares desired deployments (from runtime.Object list) against actual deployments in the cluster.
func DiffDeployments(desired []runtime.Object, actual []appsv1.Deployment) DeploymentDiff {
	desiredMap := make(map[string]*appsv1.Deployment)
	for _, obj := range desired {
		if dep, ok := obj.(*appsv1.Deployment); ok {
			desiredMap[dep.Name] = dep
		}
	}

	actualMap := make(map[string]appsv1.Deployment)
	for _, dep := range actual {
		actualMap[dep.Name] = dep
	}

	var diff DeploymentDiff

	for name, d := range desiredMap {
		existing, exists := actualMap[name]
		if !exists {
			diff.ToCreate = append(diff.ToCreate, d)
			continue
		}
		if deploymentNeedsUpdate(d, &existing) {
			d.ResourceVersion = existing.ResourceVersion
			diff.ToUpdate = append(diff.ToUpdate, d)
		}
	}

	for _, existing := range actual {
		if _, stillDesired := desiredMap[existing.Name]; !stillDesired {
			diff.ToDelete = append(diff.ToDelete, existing)
		}
	}

	return diff
}

func deploymentNeedsUpdate(desired, actual *appsv1.Deployment) bool {
	if len(desired.Spec.Template.Spec.Containers) == 0 || len(actual.Spec.Template.Spec.Containers) == 0 {
		return true
	}
	return desired.Spec.Template.Spec.Containers[0].Image != actual.Spec.Template.Spec.Containers[0].Image
}

// DiffServices compares desired services against actual services in the cluster.
func DiffServices(desired []runtime.Object, actual []corev1.Service) ServiceDiff {
	desiredMap := make(map[string]*corev1.Service)
	for _, obj := range desired {
		if svc, ok := obj.(*corev1.Service); ok {
			desiredMap[svc.Name] = svc
		}
	}

	var diff ServiceDiff

	for name, d := range desiredMap {
		found := false
		for _, existing := range actual {
			if existing.Name == name {
				found = true
				break
			}
		}
		if !found {
			diff.ToCreate = append(diff.ToCreate, d)
		}
	}

	for _, existing := range actual {
		if _, stillDesired := desiredMap[existing.Name]; !stillDesired {
			diff.ToDelete = append(diff.ToDelete, existing)
		}
	}

	return diff
}
