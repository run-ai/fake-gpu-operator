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

Run any image that does not require a GPU. For example, the Run:ai quickstart fake image - `gcr.io/run-ai-demo/quickstart-demo`

## Troubleshooting

Pod Security Admissions should be disabled on the gpu-operator namespace 

```
kubectl label ns gpu-operator pod-security.kubernetes.io/enforce=privileged
```

## Customization

The GPU topology is defined using a Kubernetes configmap named `topology`. The configmap is defined in the values file of the chart under the initialTopology section.

To customize, create a file `values.yaml` and use it when installing the helm chart by adding as with `-f values.yaml`

### Node specific customization

To control the GPUs for each node, use the following values file:

```
initialTopology:
  nodes:
    __node_name__:
      gpu-memory: 11441
      gpu-product: Tesla-K80
      gpu-count: 16
```

* Replace __node_name__ with the actual node name that you want to add fake GPUs to
* Change the other values as required. 

### Same GPU Configuration for all nodes



if you want all nodes the same, then you can use:

 config: node-autofill: enabled: true

if you want custom topology per-node, make sure that:

 config: node-autofill: enabled: false

under “data: topology.yml: | nodes:”, it should look as described above (__node_name__:, etc..)




### Customizate an existing installation

If you already have fake-gpu-operator installed, run:

```
kubectl edit cm topology -n gpu-operator
```

Change the configmap and save. Then run:

```
kubectl delete pods -n gpu-operator --all --force
```

