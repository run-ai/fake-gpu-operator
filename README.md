# Fake GPU Operator

<div align="center">

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Releases](https://img.shields.io/github/v/release/run-ai/fake-gpu-operator)](https://github.com/run-ai/fake-gpu-operator/releases)
[![CI](https://github.com/run-ai/fake-gpu-operator/workflows/CI/badge.svg)](https://github.com/run-ai/fake-gpu-operator/actions)

🎮 Simulate NVIDIA GPUs in Kubernetes without actual hardware
</div>

## 🚀 Overview

The Fake GPU Operator (GPU Operator Simulator) enables you to simulate NVIDIA GPUs in a Kubernetes cluster without requiring physical GPU hardware. Created by Run:ai, this tool helps developers and testers:

- 💻 Transform CPU-only nodes to simulate one or more virtual GPUs
- 🔄 Replicate all NVIDIA GPU Operator features including feature discovery and NVIDIA MIG
- 📊 Generate Prometheus metrics that mirror real GPU behavior
- 💰 Save costs on GPU hardware for testing and development

Perfect for:
- Development and testing of GPU-dependent applications
- CI/CD pipelines requiring GPU validation
- Large-scale testing environments
- Training and educational purposes

## ✨ Features

- **Full GPU Simulation**: Emulate any NVIDIA GPU topology
- **Metric Generation**: Prometheus metrics that simulate real GPU behavior
- **MIG Support**: Complete NVIDIA Multi-Instance GPU simulation
- **Flexible Configuration**: Customize GPU types and memory configurations
- **nvidia-smi Support**: Simulated nvidia-smi tool for GPU monitoring

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
# Add the Helm repository
helm repo add fake-gpu-operator https://runai.jfrog.io/artifactory/api/helm/fake-gpu-operator-charts-prod --force-update
helm repo update

# Install the operator
helm upgrade -i gpu-operator fake-gpu-operator/fake-gpu-operator \
  --namespace gpu-operator \
  --create-namespace
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

## 🔍 Troubleshooting

### Pod Security Admission

To ensure proper functionality, configure Pod Security Admission for the gpu-operator namespace:

```bash
kubectl label ns gpu-operator pod-security.kubernetes.io/enforce=privileged
```

## 📊 Monitoring

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

- 📚 [Documentation](https://docs.run.ai)
- 🐛 [Issue Tracker](https://github.com/run-ai/fake-gpu-operator/issues)
- 💬 [Community Discussions](https://github.com/run-ai/fake-gpu-operator/discussions)

---

<div align="center">
Created with ❤️ by <a href="https://run.ai">Run:ai</a>
</div>
