# Fake GPU Operator

The purpose of the _fake GPU Operator_ or GPU Operator Simulator is to simulate the NVIDIA GPU Operator without a GPU. The software has been created by Run:ai in order to save money on actual machines in situations that do not require the GPU itself. This simulator:

* Allows you to take a CPU-only node and externalize it as if it has 1 or more GPUs. 
* Simulates all aspects of the NVIDIA GPU Operator including feature discovery, NVIDIA MIG and more. 
* Emits metrics to Prometheus simulating actual GPUs

You can configure the simulator to have any NVIDIA GPU type or amount of GPU memory. 

