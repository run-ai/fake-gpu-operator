# Deploying fake-gpu-operator on Openshift

This document will guide you through deploying fake-gpu-operator on an OpenShift cluster with some basic validation. Unlike most Kubernetes based clusters, Openshift has tighter security controls on what pods or workloads can do. Since there is this delta, we think it would be helpful to have this document here for others looking to deploy it on Openshift.

It must be noted that this operator has limitations on what can be simulated. Do NOT expect to be able to plug in and have it play nice for say something like vllm or llm-d gpu required workloads/requests. As of this document, the simulated GPUs do not support simulated inferencing. If you need that functionality, consider looking at [llm-d/llm-d simulated-accelerators](https://github.com/llm-d/llm-d/tree/main/guides/simulated-accelerators) and/or [llm-d/llm-d-inference-sim](https://github.com/llm-d/llm-d-inference-sim).

Note: Comparable Openshift infrastructure and/or older OCP versions may work but have not been tested.
Note: The oc cli is used for this guide but kubectl should work as well.

## Prerequisites

Apart from what is already mentioned on the main README page, below are some further points to keep in mind.

### Infrastructure Setup

- AWS - An [ec2](https://aws.amazon.com/ec2/instance-types/m6a/) instance of m6a.4xlarge was used for the Openshift control plane nodes. The cluster was scaled up with 2 extra worker nodes for demonstration purposes. If your a Red Hat associate, partner, or customer you can provision through the demo redhat system. 

### Platform Setup
- OpenShift - This guide was tested on OpenShift 4.20.
- Cluster administrator privileges are required to grant appropriate privileges for some of the fake-gpu-operator component service accounts.

## Deployment Steps

1. After logging into your Openshift cluster, get the cluster node names and focus on the ones you want to be your simulated GPUs. I will select the workers.
    ```
    $ oc get nodes
    NAME                                        STATUS   ROLES                         AGE   VERSION
    ip-10-0-13-14.us-east-2.compute.internal    Ready    worker                        21h   v1.33.5
    ip-10-0-28-20.us-east-2.compute.internal    Ready    control-plane,master,worker   43h   v1.33.5
    ip-10-0-30-218.us-east-2.compute.internal   Ready    worker                        21h   v1.33.5
    ```
1. Make sure to label them.
    ```
    oc label node ip-10-0-13-14.us-east-2.compute.internal run.ai/simulated-gpu-node-pool=default
    oc label node ip-10-0-30-218.us-east-2.compute.internal run.ai/simulated-gpu-node-pool=default
    ```
1. Deploy the helm chart. You can get the particular version you want by looking at the fake-gpu-operator repository releases page. Make sure you drop the version prefix "v" when running the helm command. Overwrite the environment.openshift value from the values file so the approprate security context constraints can be configured.
    ```
    helm upgrade -i gpu-operator oci://ghcr.io/run-ai/fake-gpu-operator/fake-gpu-operator --namespace gpu-operator --create-namespace --version 0.0.64 --set environment.openshift=true
    ```
1. You should see the following helm output.
    ```
    Release "gpu-operator" does not exist. Installing it now.
    Pulled: ghcr.io/run-ai/fake-gpu-operator/fake-gpu-operator:0.0.64
    Digest: sha256:f3a96f26ebc3bd77a2c50c4f792c692064826b99906aead51720413e6936e08b
    NAME: gpu-operator
    LAST DEPLOYED: Wed Dec 10 13:30:57 2025
    NAMESPACE: gpu-operator
    STATUS: deployed
    REVISION: 1
    TEST SUITE: None
    ```
1. Validate the deployments and daemonsets are up.
    ```
    $ oc get deploy,ds -n gpu-operator
    NAME                                     READY   UP-TO-DATE   AVAILABLE   AGE
    deployment.apps/gpu-operator             1/1     1            1           14m
    deployment.apps/kwok-gpu-device-plugin   1/1     1            1           14m
    deployment.apps/nvidia-dcgm-exporter     1/1     1            1           14m
    deployment.apps/status-updater           1/1     1            1           14m
    deployment.apps/topology-server          1/1     1            1           14m

    NAME                                  DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR                                    AGE
    daemonset.apps/device-plugin          2         2         2       2            2           nvidia.com/gpu.deploy.device-plugin=true         14m
    daemonset.apps/mig-faker              0         0         0       0            0           node-role.kubernetes.io/runai-dynamic-mig=true   14m
    daemonset.apps/nvidia-dcgm-exporter   2         2         2       2            2           nvidia.com/gpu.deploy.dcgm-exporter=true         14m
    ```
1. Save the following content as a yaml file named `gpu-test-pod.yaml`.
    ```
    apiVersion: v1
    kind: Pod
    metadata:
      name: gpu-test-pod
      namespace: gpu-operator
    spec:
      containers:
      - name: gpu-container
        image: image-registry.openshift-image-registry.svc:5000/openshift/tools:latest
        resources:
          limits:
            nvidia.com/gpu: 1
          env:
          - name: NODE_NAME
            valueFrom:
              fieldRef:
                fieldPath: spec.nodeName
          command:
          - /bin/sh
          - -c
          - |
            while true; do
              echo "NODE_NAME=$NODE_NAME"
              sleep 10
            done
    ```
1. Create the pod that "needs" a gpu to simulate scheduling onto a "gpu" node.
    ```
    $ oc apply -f gpu-test-pod.yaml 
    pod/gpu-test-pod created
    ```
1. Confirm that the mock nvidia-smi command got injected into the pods runtime and that we get the default simulated `Tesla-K80` gpu info.
    ```
    $ oc exec pod/gpu-test-pod -n gpu-operator -- nvidia-smi
    Wed Dec 10 03:15:26 2025
    +------------------------------------------------------------------------------+
    | NVIDIA-SMI 470.129.06   Driver Version: 470.129.06   CUDA Version: 11.4      |
    +--------------------------------+----------------------+----------------------+
    | GPU  Name        Persistence-M | Bus-Id        Disp.A | Volatile Uncorr. ECC |
    | Fan  Temp  Perf  Pwr:Usage/Cap |         Memory-Usage | GPU-Util  Compute M. |
    |                                |                      |               MIG M. |
    +--------------------------------+----------------------+----------------------+
    |   0  Tesla-K80             Off | 00000001:00:00.0 Off |                  Off |
    | N/A   33C    P8    11W /  70W  |  11441MiB / 11441MiB |     100%     Default |
    |                                |                      |                  N/A |
    +--------------------------------+----------------------+----------------------+

    +------------------------------------------------------------------------------+
    | Processes:                                                                   |
    |  GPU   GI   CI        PID   Type   Process name                  GPU Memory  |
    |        ID   ID                                                   Usage       |
    +------------------------------------------------------------------------------+
    |    0   N/A  N/A       17       G   /bin/sh-cwhile true; do                   |
    |  ..    11441MiB                                                              |
    +------------------------------------------------------------------------------+
    ```


## Tips
- Have atleast 2 nodes labelled as for some reason the nvidia-dcgm-exporter pods complain with the following strange messages:
    ```
    2025/12/09 05:16:40 Topology update not received within interval, publishing...
    2025/12/09 05:16:40 Error getting configmap: topology-ip-10-0-13-14.us-east-2.compute.internal
    ```
- If for any reason you need to remove the helm release execute the following. Replace with your version.
    ```
    $ helm uninstall gpu-operator --namespace gpu-operator
    release "gpu-operator" uninstalled
    ```