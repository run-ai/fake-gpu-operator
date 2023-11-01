# Fake GPU Operator

The purpose of the _fake GPU Operator_ or GPU Operator Simulator is to simulate the NVIDIA GPU Operator without a GPU. The software has been created by Run:ai in order to save money on actual machines in situations that do not require the GPU itself. This simulator:

* Allows you to take a CPU-only node and externalize it as if it has 1 or more GPUs. 
* Simulates all aspects of the NVIDIA GPU Operator including feature discovery, NVIDIA MIG and more. 
* Emits metrics to Prometheus simulating actual GPUs

You can configure the simulator to have any NVIDIA GPU topology, including type and amount of GPU memory. 



## Prerequisites

The real Nvidia GPU Operator should not exist in the Kubernetes cluster

## Installation

Label the nodes you wish to have fake GPUs with the following labels:

```
kubectl label node <node-name> nvidia.com/gpu.deploy.device-plugin=true nvidia.com/gpu.deploy.dcgm-exporter=true --overwrite
```

By default, the operator creates a GPU topology of 2 Tesla K80 GPUs for each node. To create a different GPU topology, see the __customization__ section below.


Install the operator:

```
helm repo add fake-gpu-operator https://fake-gpu-operator.storage.googleapis.com
helm repo update
helm upgrade -i gpu-operator fake-gpu-operator/fake-gpu-operator --namespace gpu-operator --create-namespace
```

## Usage

Submit any workload that requests an NVIDIA GPU 

```
resources:
  limits:
    nvidia.com/gpu: 1
```

Verify that it schedules on one of the CPU nodes 

You can also test by running the example deployment under the `example` folder

## Troubleshooting

[Pod Security Admission](https://kubernetes.io/docs/concepts/security/pod-security-admission/) should be disabled on the gpu-operator namespace 

```
kubectl label ns gpu-operator pod-security.kubernetes.io/enforce=privileged
```

## Customization

The base GPU topology is defined using a Kubernetes configmap named `topology`.

To customize the GPU topology, edit the configmap:

```
kubectl edit cm topology -n gpu-operator
```

The configmap should look like this:

```
apiVersion: v1
data:
  topology.yml: |
    config:
      node-autofill:
        gpu-count: 16
        gpu-memory: 11441
        gpu-product: Tesla-K80
    mig-strategy: mixed
```

This configmap defines the GPU topology for all nodes.

* __gpu-count__ - number of GPUs per node.
* __gpu-memory__ - amount of GPU memory per GPU.
* __gpu-product__ - GPU type. For example: `Tesla-K80`, `Tesla-V100`, etc.
* __mig-strategy__ - MIG strategy. Can be `none`, `mixed` or `single`.

### Node specific customization

Each node can have a different GPU topology. To customize a specific node, edit the configmap named `<node-name>-topology` in the `gpu-operator` namespace.


### GPU metrics

By default, dcgm exporter will export maximum GPU utilization for every pod that requests GPUs.

If you want to customize the GPU utilization, add a `run.ai/simulated-gpu-utilization` annotation to the pod with a value that represents the range of the GPU utilization that should be simulated.
For example, add `run.ai/simulated-gpu-utilization: 10-30` annotation to simulate a pod that utilizes the GPU between 10% to 30%.