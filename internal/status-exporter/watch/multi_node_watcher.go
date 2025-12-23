package watch

import (
	"context"
	"log"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
)

type MetricsExporter interface {
	SetMetricsForNode(nodeName string, nodeTopology *topology.NodeTopology) error
	DeleteNode(nodeName string) error
}

type LabelsExporter interface {
	SetLabelsForNode(nodeName string, nodeTopology *topology.NodeTopology) error
}

type MultiNodeWatcher struct {
	client.Client
	scheme           *runtime.Scheme
	namespace        string
	topologyCMPrefix string
	metricsExporter  MetricsExporter
	labelsExporter   LabelsExporter
}

func NewMultiNodeWatcher(mgr ctrl.Manager, namespace, topologyCMPrefix string, metricsExporter MetricsExporter, labelsExporter LabelsExporter) *MultiNodeWatcher {
	return &MultiNodeWatcher{
		Client:           mgr.GetClient(),
		scheme:           mgr.GetScheme(),
		namespace:        namespace,
		topologyCMPrefix: topologyCMPrefix,
		metricsExporter:  metricsExporter,
		labelsExporter:   labelsExporter,
	}
}

func (w *MultiNodeWatcher) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var cm corev1.ConfigMap
	if err := w.Get(ctx, req.NamespacedName, &cm); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}

		nodeName := extractNodeNameFromCMName(req.Name, w.topologyCMPrefix)
		if nodeName != "" {
			log.Printf("KWOK node ConfigMap deleted: %s\n", nodeName)
			w.handleNodeDeletion(nodeName)
		}
		return ctrl.Result{}, nil
	}

	nodeTopology, err := topology.FromNodeTopologyCM(&cm)
	if err != nil {
		log.Printf("Failed to parse topology from ConfigMap %s: %v (skipping)\n", req.Name, err)
		return ctrl.Result{}, nil
	}

	nodeName := extractNodeNameFromCMName(req.Name, w.topologyCMPrefix)
	if nodeName == "" {
		log.Printf("Failed to extract node name from ConfigMap: %s\n", req.Name)
		return ctrl.Result{}, nil
	}

	log.Printf("KWOK node topology update: %s\n", nodeName)

	w.handleNodeUpdate(nodeName, nodeTopology)

	return ctrl.Result{}, nil
}

func (w *MultiNodeWatcher) handleNodeUpdate(nodeName string, nodeTopology *topology.NodeTopology) {
	if err := w.metricsExporter.SetMetricsForNode(nodeName, nodeTopology); err != nil {
		log.Printf("Metrics exporter failed for node %s: %v\n", nodeName, err)
	}

	if err := w.labelsExporter.SetLabelsForNode(nodeName, nodeTopology); err != nil {
		log.Printf("Labels exporter failed for node %s: %v\n", nodeName, err)
	}
}

func (w *MultiNodeWatcher) handleNodeDeletion(nodeName string) {
	if err := w.metricsExporter.DeleteNode(nodeName); err != nil {
		log.Printf("Metrics exporter deletion failed for node %s: %v\n", nodeName, err)
	}
}

func extractNodeNameFromCMName(cmName, topologyCMPrefix string) string {
	prefix := topologyCMPrefix + "-"
	if strings.HasPrefix(cmName, prefix) {
		return strings.TrimPrefix(cmName, prefix)
	}
	return cmName
}

func (w *MultiNodeWatcher) SetupWithManager(mgr ctrl.Manager) error {
	kwokNodeCMPredicate := predicate.NewPredicateFuncs(func(obj client.Object) bool {
		cm, ok := obj.(*corev1.ConfigMap)
		if !ok {
			return false
		}
		return isKWOKNodeConfigMap(cm)
	})

	namespacePredicate := predicate.NewPredicateFuncs(func(obj client.Object) bool {
		return obj.GetNamespace() == w.namespace
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.ConfigMap{}).
		WithEventFilter(predicate.And(namespacePredicate, kwokNodeCMPredicate)).
		Complete(w)
}

func isKWOKNodeConfigMap(cm *corev1.ConfigMap) bool {
	if cm == nil || cm.Labels == nil || cm.Annotations == nil {
		return false
	}

	_, foundNodeName := cm.Labels[constants.LabelTopologyCMNodeName]
	if !foundNodeName {
		return false
	}

	return cm.Annotations[constants.AnnotationKwokNode] == "fake"
}

func SetupMultiNodeWatcherWithManager(mgr ctrl.Manager, namespace, topologyCMName string, metricsExporter MetricsExporter, labelsExporter LabelsExporter) (*MultiNodeWatcher, error) {
	watcher := NewMultiNodeWatcher(mgr, namespace, topologyCMName, metricsExporter, labelsExporter)

	if err := watcher.SetupWithManager(mgr); err != nil {
		return nil, err
	}

	log.Println("Multi-node watcher setup complete for KWOK status-exporter")
	return watcher, nil
}
