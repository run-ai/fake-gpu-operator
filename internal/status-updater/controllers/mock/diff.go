package mock

import (
	"reflect"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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
func DiffDaemonSets(desired []*appsv1.DaemonSet, actual []appsv1.DaemonSet) DaemonSetDiff {
	desiredByName := make(map[string]*appsv1.DaemonSet, len(desired))
	for _, ds := range desired {
		desiredByName[ds.Name] = ds
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

// daemonsetNeedsUpdate returns true iff the first container's image,
// command, args, or lifecycle hooks differ; or the config-hash annotation
// differs. Command/Args/Lifecycle are checked because resources.go evolves
// across status-updater versions (e.g., adding a postStart hook for the
// toolkit-ready marker write) — bumping status-updater needs to propagate
// those changes to existing DSes.
func daemonsetNeedsUpdate(want, have *appsv1.DaemonSet) bool {
	if len(want.Spec.Template.Spec.Containers) == 0 || len(have.Spec.Template.Spec.Containers) == 0 {
		return true
	}
	wantC, haveC := want.Spec.Template.Spec.Containers[0], have.Spec.Template.Spec.Containers[0]
	if wantC.Image != haveC.Image {
		return true
	}
	if !reflect.DeepEqual(wantC.Command, haveC.Command) {
		return true
	}
	if !reflect.DeepEqual(wantC.Args, haveC.Args) {
		return true
	}
	if !reflect.DeepEqual(wantC.Lifecycle, haveC.Lifecycle) {
		return true
	}
	return want.Spec.Template.Annotations[ConfigHashAnnotation] != have.Spec.Template.Annotations[ConfigHashAnnotation]
}

// DiffConfigMaps compares desired vs actual ConfigMaps. Update fires when
// data["config.yaml"] differs.
func DiffConfigMaps(desired []*corev1.ConfigMap, actual []corev1.ConfigMap) ConfigMapDiff {
	desiredByName := make(map[string]*corev1.ConfigMap, len(desired))
	for _, cm := range desired {
		desiredByName[cm.Name] = cm
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
