# Fake GPU Operator

The purpose of the _fake GPU Operator_ or GPU Operator Simulator is to simulate the NVIDIA GPU Operator without a GPU. The software has been created by Run:ai in order to save money on actual machines in situations that do not require the GPU itself. This simulator:

* Allows a CPU-only node to be represented as if it has one or more GPUs.
* Simulates all features of the NVIDIA GPU Operator, including feature discovery and NVIDIA MIG.
* Emits metrics to Prometheus, simulating actual GPU behavior.

You can configure the simulator to have any NVIDIA GPU topology, including the type and amount of GPU memory.



## Prerequisites

Ensure that the real Nvidia GPU Operator is not present in the Kubernetes cluster.

## Installation

Assign the nodes you want to simulate GPUs on to a node pool by labeling them with the `run.ai/simulated-gpu-node-pool` label. For example:

```sh
kubectl label node <node-name> run.ai/simulated-gpu-node-pool=default
```

NodePools are used to group nodes that should have the same GPU topology.
These are defined in the `topology.nodePools` section of the Helm `values.yaml` file.
By default, a node pool with 2 Tesla K80 GPUs will be created for all nodes labeled with `run.ai/simulated-gpu-node-pool=default`.
To create a different GPU topology, refer to the __customization__ section below.


To install the operator:


```sh
helm repo add fake-gpu-operator https://fake-gpu-operator.storage.googleapis.com
helm repo update
helm upgrade -i gpu-operator fake-gpu-operator/fake-gpu-operator --namespace gpu-operator --create-namespace
```

## Usage

Submit any workload with a request for an NVIDIA GPU:

```
resources:
  limits:
    nvidia.com/gpu: 1
```

Verify that it has been scheduled on one of the __CPU__ nodes. 

You can also test by running the example deployment YAML under the `example` folder

## Troubleshooting

[Pod Security Admission](https://kubernetes.io/docs/concepts/security/pod-security-admission/) should be disabled on the gpu-operator namespace 

```
kubectl label ns gpu-operator pod-security.kubernetes.io/enforce=privileged
```

## Customization

The GPU topology can be customized by editing the `values.yaml` file on the `topology` section before installing/upgrading the helm chart.

## GPU metrics

By default, the DCGM exporter will report maximum GPU utilization for every pod requesting GPUs.

To customize GPU utilization, add a `run.ai/simulated-gpu-utilization` annotation to the pod with a value representing the desired range of GPU utilization.
For example, add `run.ai/simulated-gpu-utilization: 10-30` to simulate a pod that utilizes between 10% and 30% of the GPU.
