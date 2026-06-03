# Fake GPU Operator

<div align="center">

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Releases](https://img.shields.io/github/v/release/run-ai/fake-gpu-operator)](https://github.com/run-ai/fake-gpu-operator/releases)
[![CI](https://github.com/run-ai/fake-gpu-operator/actions/workflows/ci.yml/badge.svg)](https://github.com/run-ai/fake-gpu-operator/actions/workflows/ci.yml)

🎮 Simulate NVIDIA GPUs in Kubernetes without actual hardware
</div>

> **Note:** Container images and Helm charts are now available at `ghcr.io/run-ai/fake-gpu-operator`.

## 🚀 Overview

The Fake GPU Operator is a lightweight tool that simulates NVIDIA GPUs in Kubernetes clusters without requiring physical hardware. It provides basic functionality for developers and testers:

- Simulates virtual GPUs on CPU-only nodes
- Supports basic feature discovery and NVIDIA MIG support
- Generates Prometheus metrics for GPU monitoring
- Reduces hardware costs for testing environments

Use cases include:
- Testing GPU-dependent applications
- CI/CD pipeline testing
- Development environments
- Learning and experimentation

## ✨ Features

- Basic GPU topology simulation
- Prometheus metrics generation
- Basic NVIDIA MIG resource scheduling (metrics monitoring not yet supported)
- Configurable GPU types and memory
- Basic nvidia-smi simulation

## 🏃 Quick Start

### Prerequisites

- Kubernetes cluster without NVIDIA GPU Operator
- Helm 3.x
- kubectl CLI tool

### 1. Label Your Nodes

```bash
kubectl label node <node-name> run.ai/simulated-gpu-node-pool=default
```

### 2. Install the Operator

```bash
helm upgrade -i gpu-operator oci://ghcr.io/run-ai/fake-gpu-operator/fake-gpu-operator --namespace gpu-operator --create-namespace --version <VERSION>
```

### 3. Deploy a Test Workload

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: gpu-pod
spec:
  containers:
  - name: gpu-container
    image: nvidia/cuda-vector-add:v0.1
    resources:
      limits:
        nvidia.com/gpu: 1
```

## 🛠️ Configuration

### GPU Topology

Customize GPU configurations in your `values.yaml`:

```yaml
topology:
  nodePools:
    default:
      gpuProduct: Tesla-K80
      gpuCount: 2
      gpuMemory: 11441
```

### GPU Utilization

Control GPU utilization metrics with pod annotations:

```yaml
metadata:
  annotations:
    run.ai/simulated-gpu-utilization: "10-30"  # Simulate 10-30% GPU usage
```

### Knative Inference Workload Integration

The operator provides special handling for **Knative-based inference workloads**, where GPU utilization is dynamically calculated based on actual request traffic rather than static values.

#### How It Works

When a pod is identified as an inference workload (via `workloadKind: "InferenceWorkload"` label or PodGroup `priorityClassName: "inference"`), the operator:

1. **Queries Prometheus** for real-time request metrics using the `revision_app_request_count` metric
2. **Calculates utilization** based on request rate: `rate(revision_app_request_count[1m])`
3. **Updates GPU metrics** to reflect actual inference load

This provides realistic GPU utilization metrics that correlate with inference traffic patterns.

#### Configuration

Configure Prometheus connection in your Helm values:

```yaml
prometheus:
  url: http://prometheus-operated.runai:9090  # Default
```

For local development with port-forwarding:

```yaml
prometheus:
  url: http://localhost:9090
```

#### Example

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: inference-pod
  labels:
    workloadKind: "InferenceWorkload"  # Enables Knative utilization
spec:
  containers:
  - name: model-server
    image: my-inference-server:latest
    resources:
      limits:
        nvidia.com/gpu: 1
```

When requests flow to this inference pod, GPU utilization metrics will reflect the actual request rate from Knative.

**Supported Knative Workload Types:**
- `workloadKind: "InferenceWorkload"` - Single-node inference with Knative metrics
- `workloadKind: "DistributedWorkload"` - Distributed inference with Knative metrics

## Mock Backend (Real NVML)

Most pools use the **`fake`** backend, where FGO's own device-plugin advertises synthetic `nvidia.com/gpu`.

Switch a pool to the **`mock`** backend to:

- Run the **real upstream NVIDIA gpu-operator** components against simulated GPUs.
- Support **apps that call NVML directly**, such as `nvidia-smi` or CUDA device discovery.

It installs NVIDIA's [`nvml-mock`](https://github.com/NVIDIA/k8s-test-infra/tree/main/deployments/nvml-mock) on the pool's nodes. It runs privileged pods and writes host files under `/var/lib/nvml-mock`, so it is opt-in per pool:

```yaml
topology:
  nodePools:
    my-mock-pool:
      gpu:
        backend: mock
        profile: a100        # or h100, b200, gb200, l40s, t4
gpuOperator: { enabled: true }        # device-plugin path; also set gpu-operator.toolkit.enabled
# nvidiaDraDriver: { enabled: true }  # or use this lighter DRA-only path instead
```

See **[docs/mock-backend.md](docs/mock-backend.md)** for profiles, caveats such as the cosmetic `ClusterPolicy NotReady` status, and cleanup notes.

## 🔌 Dynamic Resource Allocation (DRA)

For Kubernetes 1.31+, you can use the DRA plugin instead of the legacy device plugin.

### Prerequisites

Enable DynamicResourceAllocation feature gate and the resource.k8s.io/v1 API on your cluster if needed.

### Enable DRA plugin in Helm chart

```yaml
# values.yaml
draPlugin:
  enabled: true
devicePlugin:
  enabled: false  # Disable legacy plugin
```

### Deploy with DRA

```yaml
apiVersion: resource.k8s.io/v1
kind: ResourceClaimTemplate
metadata:
  name: gpu-claim
spec:
  spec:
    devices:
      requests:
      - name: gpu
        exactly:
          deviceClassName: gpu.nvidia.com
---
apiVersion: v1
kind: Pod
metadata:
  name: gpu-pod
spec:
  containers:
  - name: main
    image: ubuntu:22.04
    resources:
      claims:
      - name: gpu
  resourceClaims:
  - name: gpu
    resourceClaimTemplateName: gpu-claim
```

See [test/e2e/fixtures/manifests/](test/e2e/fixtures/manifests/) for more examples.

## 🔐 Compute Domain DRA (Secure Workload Isolation)

The Fake GPU Operator supports simulating [NVIDIA Compute Domains](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/dra-cds.html) for secure workload isolation without requiring actual NVIDIA hardware. Compute Domains provide IMEX channel simulation for multi-node GPU workloads.

### Prerequisites

- Kubernetes 1.31+ with DynamicResourceAllocation feature gate enabled
- DRA plugin enabled in the Fake GPU Operator

### Enable Compute Domain in Helm chart

```yaml
# values.yaml
computeDomainController:
  enabled: true
computeDomainDraPlugin:
  enabled: true
draPlugin:
  enabled: true
devicePlugin:
  enabled: false  # Disable legacy plugin when using DRA
```

### Deploy with Compute Domain

First, create a ComputeDomain resource:

```yaml
apiVersion: resource.nvidia.com/v1beta1
kind: ComputeDomain
metadata:
  name: my-compute-domain
  namespace: default
spec:
  numNodes: 1
  channel:
    allocationMode: Single  # or "All" for all channels
    resourceClaimTemplate:
      name: my-compute-domain
```

The compute-domain-controller will automatically create a ResourceClaimTemplate for the ComputeDomain.

Then, deploy a pod that uses the compute domain:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: compute-domain-pod
  namespace: default
spec:
  containers:
  - name: main
    image: ubuntu:22.04
    command: ["sleep", "infinity"]
    resources:
      claims:
      - name: compute-domain
  resourceClaims:
  - name: compute-domain
    resourceClaimTemplateName: my-compute-domain
```

### Verify Compute Domain Status

```bash
# Check ComputeDomain status
kubectl get computedomain my-compute-domain -o yaml

# Verify status shows Ready and allocated nodes
# status:
#   status: Ready
#   nodes:
#   - name: <node-name>
#     status: Ready
```

## 🎭 KWOK Integration (Simulated Nodes)

[KWOK](https://kwok.sigs.k8s.io/) (Kubernetes WithOut Kubelet) is a toolkit that allows you to simulate thousands of Kubernetes nodes without running actual kubelet processes. When combined with the Fake GPU Operator, you can create large-scale GPU cluster simulations entirely without hardware - perfect for testing schedulers, autoscalers, and resource management at scale.

### Why KWOK + Fake GPU Operator?

- **Scale Testing**: Simulate hundreds of GPU nodes to test scheduler behavior
- **Cost Efficiency**: No cloud VMs or physical hardware needed
- **Fast Iteration**: Spin up/down simulated clusters in seconds
- **CI/CD**: Run e2e tests against realistic cluster topologies

### Prerequisites

1. Install KWOK controller in your cluster:
   ```bash
   KWOK_VERSION=v0.7.0
   kubectl apply -f "https://github.com/kubernetes-sigs/kwok/releases/download/${KWOK_VERSION}/kwok.yaml"
   kubectl apply -f "https://github.com/kubernetes-sigs/kwok/releases/download/${KWOK_VERSION}/stage-fast.yaml"
   ```

2. Enable the `kwok-dra-plugin` in your Helm values:
   ```yaml
   # values.yaml
   kwokDraPlugin:
     enabled: true
   draPlugin:
     enabled: true
   ```

### Create a Simulated GPU Node

```yaml
apiVersion: v1
kind: Node
metadata:
  annotations:
    kwok.x-k8s.io/node: fake
  labels:
    type: kwok
    run.ai/simulated-gpu-node-pool: default
  name: kwok-gpu-node-1
spec:
  taints:
  - effect: NoSchedule
    key: kwok.x-k8s.io/node
    value: fake
status:
  allocatable:
    cpu: "32"
    memory: 128Gi
    pods: "110"
  capacity:
    cpu: "32"
    memory: 128Gi
    pods: "110"
```

The `status-updater` will automatically create a topology ConfigMap for this node, and the `kwok-dra-plugin` will create a ResourceSlice with the configured GPUs.

### Schedule a GPU Pod on KWOK Node

```yaml
apiVersion: resource.k8s.io/v1
kind: ResourceClaimTemplate
metadata:
  name: gpu-claim
spec:
  spec:
    devices:
      requests:
      - name: gpu
        exactly:
          deviceClassName: gpu.nvidia.com
---
apiVersion: v1
kind: Pod
metadata:
  name: kwok-gpu-pod
spec:
  nodeSelector:
    type: kwok
  tolerations:
  - key: kwok.x-k8s.io/node
    operator: Equal
    value: fake
    effect: NoSchedule
  containers:
  - name: main
    image: ubuntu:22.04
    command: ["sleep", "infinity"]
    resources:
      claims:
      - name: gpu
  resourceClaims:
  - name: gpu
    resourceClaimTemplateName: gpu-claim
```

The pod will be "scheduled" on the KWOK node and appear as Running (KWOK simulates the pod lifecycle). The ResourceClaim will be allocated from the simulated GPU ResourceSlice.

### Verify Setup

```bash
# Check KWOK node is Ready
kubectl get nodes -l type=kwok

# Check ResourceSlice was created
kubectl get resourceslices | grep kwok

# Check pod is running on KWOK node
kubectl get pod kwok-gpu-pod -o wide
```

## 🔀 Mixed Real + Fake GPU Nodes

You can run the Fake GPU Operator **alongside** a real NVIDIA GPU Operator on the same cluster: real-GPU nodes stay fully managed by the real operator, while simulated GPU nodes are added for scale and scheduler testing.

By default the Fake GPU Operator is a drop-in replacement for the real one, so to make them coexist you must (1) install it in its **own namespace** (not `gpu-operator`) and (2) disable the components whose node selectors / cluster-scoped objects collide with the real operator:

```yaml
# mixed-mode-values.yaml — coexist with a real NVIDIA GPU Operator
devicePlugin:   { enabled: false }  # selector collides with the real device-plugin on real-GPU nodes
statusExporter: { enabled: false }  # selector collides with the real dcgm-exporter on real-GPU nodes
runtimeClass:   { enabled: false }  # cluster-scoped RuntimeClass/nvidia is already owned by the real operator
```

Install into a dedicated namespace:

```bash
helm upgrade -i fake-gpu-operator oci://ghcr.io/run-ai/fake-gpu-operator/fake-gpu-operator \
  --namespace fake-gpu-operator --create-namespace \
  --version <VERSION> -f mixed-mode-values.yaml
```

Then:

- **Fake GPUs run on KWOK nodes** (see [KWOK Integration](#-kwok-integration-simulated-nodes) above). Their `nvidia.com/gpu` capacity is published by the central `kwok-gpu-device-plugin`, so the disabled per-node DaemonSets aren't needed, and the KWOK `NoSchedule` taint keeps the real operator's DaemonSets off the simulated nodes.
- **Real-GPU nodes are left untouched** — simply do **not** label them with `run.ai/simulated-gpu-node-pool`. The `status-updater` only acts on nodes carrying that label, so the real operator keeps exclusive ownership of your hardware.

## 🔍 Troubleshooting

### Pod Security Admission

To ensure proper functionality, configure Pod Security Admission for the gpu-operator namespace:

```bash
kubectl label ns gpu-operator pod-security.kubernetes.io/enforce=privileged
```

### nvidia-smi Support

The operator injects a simulated `nvidia-smi` tool into GPU pods. `nvidia-smi` uses the
`NODE_NAME` environment variable to resolve the pod's node topology; both the device-plugin
and the DRA driver inject it automatically at allocation time, so no manual pod configuration
is required.

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## 📝 License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.

## 🙋 Support

- 🐛 [Issue Tracker](https://github.com/run-ai/fake-gpu-operator/issues)

---

<div align="center">
Created with ❤️ by <a href="https://run.ai">Run:ai</a>
</div>
