package mock

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DaemonSetDiff partitions name-keyed DaemonSets into Create/Update/Delete.
type DaemonSetDiff struct {
	ToCreate []*appsv1.DaemonSet
	ToUpdate []*appsv1.DaemonSet
	ToDelete []appsv1.DaemonSet
}

// ConfigMapDiff partitions name-keyed ConfigMaps. ToUpdate carries
// ResourceVersion-stamped objects ready to send to Update.
type ConfigMapDiff struct {
	ToCreate []*corev1.ConfigMap
	ToUpdate []*corev1.ConfigMap
	ToDelete []corev1.ConfigMap
}

// DiffDaemonSets compares desired vs actual. A DaemonSet that exists in both
// triggers an Update iff its first container image differs OR its config-hash
// annotation differs. ResourceVersion is copied from actual into desired
// before Update (optimistic concurrency requirement).
func DiffDaemonSets(desired []runtime.Object, actual []appsv1.DaemonSet) DaemonSetDiff {
	desiredByName := make(map[string]*appsv1.DaemonSet)
	for _, obj := range desired {
		if ds, ok := obj.(*appsv1.DaemonSet); ok {
			desiredByName[ds.Name] = ds
		}
	}
	actualByName := make(map[string]appsv1.DaemonSet, len(actual))
	for _, ds := range actual {
		actualByName[ds.Name] = ds
	}

	var d DaemonSetDiff
	for name, want := range desiredByName {
		have, exists := actualByName[name]
		if !exists {
			d.ToCreate = append(d.ToCreate, want)
			continue
		}
		if daemonsetNeedsUpdate(want, &have) {
			want.ResourceVersion = have.ResourceVersion
			d.ToUpdate = append(d.ToUpdate, want)
		}
	}
	for _, have := range actual {
		if _, stillDesired := desiredByName[have.Name]; !stillDesired {
			d.ToDelete = append(d.ToDelete, have)
		}
	}
	return d
}

// daemonsetNeedsUpdate returns true iff the first container image OR the
// config-hash annotation differs. Other fields are sourced from the
// status-updater pod's env/values or hardcoded in resources.go and only
// change across a status-updater rollout — so a CM-driven reconcile cannot
// observe a difference there.
func daemonsetNeedsUpdate(want, have *appsv1.DaemonSet) bool {
	if len(want.Spec.Template.Spec.Containers) == 0 || len(have.Spec.Template.Spec.Containers) == 0 {
		return true
	}
	if want.Spec.Template.Spec.Containers[0].Image != have.Spec.Template.Spec.Containers[0].Image {
		return true
	}
	return want.Spec.Template.Annotations[ConfigHashAnnotation] != have.Spec.Template.Annotations[ConfigHashAnnotation]
}

// DiffConfigMaps compares desired vs actual ConfigMaps. Update fires when
// data["config.yaml"] differs.
func DiffConfigMaps(desired []runtime.Object, actual []corev1.ConfigMap) ConfigMapDiff {
	desiredByName := make(map[string]*corev1.ConfigMap)
	for _, obj := range desired {
		if cm, ok := obj.(*corev1.ConfigMap); ok {
			desiredByName[cm.Name] = cm
		}
	}
	actualByName := make(map[string]corev1.ConfigMap, len(actual))
	for _, cm := range actual {
		actualByName[cm.Name] = cm
	}

	var d ConfigMapDiff
	for name, want := range desiredByName {
		have, exists := actualByName[name]
		if !exists {
			d.ToCreate = append(d.ToCreate, want)
			continue
		}
		if want.Data["config.yaml"] != have.Data["config.yaml"] {
			want.ResourceVersion = have.ResourceVersion
			d.ToUpdate = append(d.ToUpdate, want)
		}
	}
	for _, have := range actual {
		if _, stillDesired := desiredByName[have.Name]; !stillDesired {
			d.ToDelete = append(d.ToDelete, have)
		}
	}
	return d
}
