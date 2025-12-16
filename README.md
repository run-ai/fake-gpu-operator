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
      gpus:
        - type: "Tesla K80"
          memory: "12GB"
          count: 2
```

### GPU Utilization

Control GPU utilization metrics with pod annotations:

```yaml
metadata:
  annotations:
    run.ai/simulated-gpu-utilization: "10-30"  # Simulate 10-30% GPU usage
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

See [test/integration/manifests/](test/integration/manifests/) for more examples.

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
