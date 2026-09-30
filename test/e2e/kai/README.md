# KAI fractional GPU e2e on KIND

Run the complete test with Docker, KIND, Helm, kubectl, and Go installed:

```sh
make e2e-kai
```

The setup creates a KIND cluster, builds and installs the local fake-gpu-operator, and installs the latest published KAI Scheduler chart in `NonMemoryEnforced` GPU sharing mode. Set `KAI_VERSION` to pin a compatible chart version. For an iterative run, use `make setup-e2e-kai`, `make test-e2e-kai`, and `make teardown-e2e-kai` separately. The cluster name defaults to `fake-gpu-operator-kai` and can be set with `KIND_CLUSTER_NAME`.

The test confines workloads to the real KIND worker with two fake GPUs. It checks that four `gpu-fraction: "0.5"` pods run with two GPU reservation pods, that a fifth stays unscheduled, and that deleting one pod lets the fifth run. This tests KAI's scheduling and reservation accounting. Fake GPUs do not enforce CUDA memory limits.

CI runs this as the `e2e-kai` job in `.github/workflows/ci.yml`; it is required before release images are published.
