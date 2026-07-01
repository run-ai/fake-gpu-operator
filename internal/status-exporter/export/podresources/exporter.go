package podresources

import (
	"context"
	"fmt"
	"log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"github.com/spf13/viper"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/export"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/export/numazones"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/watch"
)

const (
	SocketPath = "/var/lib/fake-gpu-operator/pod-resources/kubelet.sock"
	SysfsRoot  = "/var/lib/fake-gpu-operator/sys"
)

var _ export.Interface = &Exporter{}

type Exporter struct {
	watcher    watch.Interface
	kubeClient kubernetes.Interface
	server     *Server
	sysfsRoot  string
	nodeName   string
}

func NewExporter(watcher watch.Interface, kubeClient kubernetes.Interface) *Exporter {
	return &Exporter{
		watcher:    watcher,
		kubeClient: kubeClient,
		server:     NewServer(SocketPath),
		sysfsRoot:  SysfsRoot,
		nodeName:   viper.GetString(constants.EnvNodeName),
	}
}

func (e *Exporter) Run(stopCh <-chan struct{}) {
	topologyChan := make(chan *topology.NodeTopology)
	e.watcher.Subscribe(topologyChan)

	if err := e.server.Start(); err != nil {
		log.Printf("podresources: failed to start server: %v", err)
	}
	defer e.server.Stop()

	for {
		select {
		case nt := <-topologyChan:
			if err := e.reconcile(nt); err != nil {
				log.Printf("podresources: reconcile error (keeping last-good): %v", err)
			}
		case <-stopCh:
			return
		}
	}
}

func (e *Exporter) reconcile(nt *topology.NodeTopology) error {
	ctx := context.Background()
	clusterConfig, err := topology.GetClusterConfigFromCM(e.kubeClient)
	if err != nil {
		return fmt.Errorf("get cluster config: %w", err)
	}
	node, err := e.kubeClient.CoreV1().Nodes().Get(ctx, e.nodeName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get node %s: %w", e.nodeName, err)
	}
	poolName, ok := node.Labels[clusterConfig.NodePoolLabelKey]
	if !ok {
		e.server.SetSnapshot(nil)
		return nil
	}
	poolConfig, ok := clusterConfig.NodePools[poolName]
	if !ok || poolConfig.Numa == nil {
		e.server.SetSnapshot(nil)
		return nil
	}

	layout, err := numazones.ResolveZoneLayout(*poolConfig.Numa, len(nt.Gpus), node.Status.Allocatable)
	if err != nil {
		return fmt.Errorf("resolve zone layout: %w", err)
	}
	if layout == nil {
		e.server.SetSnapshot(nil)
		return nil
	}

	reqs, err := e.podRequests(ctx)
	if err != nil {
		return fmt.Errorf("list pod requests: %w", err)
	}

	if err := RenderCpulist(e.sysfsRoot, layout); err != nil {
		return fmt.Errorf("render cpulist: %w", err)
	}
	e.server.SetSnapshot(BuildPodResources(nt, layout, reqs))
	return nil
}

func (e *Exporter) podRequests(ctx context.Context) (map[string]PodRequest, error) {
	pods, err := e.kubeClient.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("spec.nodeName", e.nodeName).String(),
	})
	if err != nil {
		return nil, err
	}
	out := make(map[string]PodRequest, len(pods.Items))
	for i := range pods.Items {
		p := &pods.Items[i]
		var cpu, mem resource.Quantity
		for _, c := range p.Spec.Containers {
			if q := c.Resources.Requests.Cpu(); q != nil {
				cpu.Add(*q)
			}
			if q := c.Resources.Requests.Memory(); q != nil {
				mem.Add(*q)
			}
		}
		out[p.Namespace+"/"+p.Name] = PodRequest{CPU: cpu, Memory: mem}
	}
	return out, nil
}
