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
    env:
      - name: NODE_NAME
        valueFrom:
          fieldRef:
            fieldPath: spec.nodeName
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

## 🧩 NVIDIA MIG (Multi-Instance GPU)

The Fake GPU Operator can simulate [MIG](https://docs.nvidia.com/datacenter/tesla/mig-user-guide/) so that a single simulated GPU is published to the scheduler as its individual MIG slices (e.g. `nvidia.com/mig-1g.5gb`). This runs on the **legacy device-plugin path** (not DRA), driven by the `mig-faker` component.

> **Note:** MIG resource _scheduling_ is supported; per-slice metrics monitoring is not yet supported.

### 1. Enable the device plugin and the `mixed` MIG strategy

MIG uses the device plugin, so make sure the DRA plugin is disabled. Set `migStrategy: mixed` so MIG-enabled GPUs are advertised as their individual profiles:

```yaml
# values.yaml
devicePlugin:
  enabled: true
migFaker:
  enabled: true
draPlugin:
  enabled: false  # device-plugin and DRA are mutually exclusive
topology:
  migStrategy: mixed
  nodePools:
    mig-pool:
      gpuProduct: NVIDIA-A100-SXM4-40GB
      gpuCount: 1
      gpuMemory: 40960
```

### 2. Label the node

In addition to the node-pool label, label the node so `mig-faker` runs on it:

```bash
kubectl label node <node-name> run.ai/simulated-gpu-node-pool=mig-pool --overwrite
kubectl label node <node-name> node-role.kubernetes.io/runai-dynamic-mig=true --overwrite
```

### 3. Apply a MIG config annotation

`mig-faker` watches the `run.ai/mig.config` node annotation. Its value is a YAML document that selects which GPU indices to slice and into which profiles. For example, to slice GPU `0` into seven `1g.5gb` instances:

```bash
kubectl annotate node <node-name> run.ai/mig.config='
version: v1
mig-configs:
  selected:
  - devices: ["0"]          # GPU indices to enable MIG on; use ["all"] for every GPU
    mig-enabled: true
    mig-devices:
    - {name: 1g.5gb, position: 0, size: 1}
    - {name: 1g.5gb, position: 1, size: 1}
    - {name: 1g.5gb, position: 2, size: 1}
    - {name: 1g.5gb, position: 3, size: 1}
    - {name: 1g.5gb, position: 4, size: 1}
    - {name: 1g.5gb, position: 5, size: 1}
    - {name: 1g.5gb, position: 6, size: 1}
' --overwrite
```

`devices` entries are GPU indices as strings (or `["all"]`). Valid profile `name`s depend on the GPU product (e.g. `1g.5gb`, `2g.10gb`, `3g.20gb` for A100-40GB).

`mig-faker` then marks the node with `nvidia.com/mig.config.state: success`, records the slices in the node's topology ConfigMap, and restarts the device-plugin pod so kubelet picks up the new resources.

### 4. Verify and schedule

```bash
kubectl get node <node-name> -o jsonpath='{.status.allocatable}' | tr ',' '\n' | grep nvidia
# nvidia.com/mig-1g.5gb":"7"
```

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: mig-pod
spec:
  containers:
  - name: main
    image: ubuntu:22.04
    command: ["sleep", "infinity"]
    resources:
      limits:
        nvidia.com/mig-1g.5gb: 1
```

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

## 🔍 Troubleshooting

### Pod Security Admission

To ensure proper functionality, configure Pod Security Admission for the gpu-operator namespace:

```bash
kubectl label ns gpu-operator pod-security.kubernetes.io/enforce=privileged
```

### nvidia-smi Support

The operator injects a simulated `nvidia-smi` tool into GPU pods. Ensure your pods include the required environment variable:

```yaml
env:
  - name: NODE_NAME
    valueFrom:
      fieldRef:
        fieldPath: spec.nodeName
```

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
