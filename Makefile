BUILD_DIR=$(shell pwd)/bin
COMPONENTS?=device-plugin dra-plugin-gpu status-updater kwok-gpu-device-plugin kwok-dra-plugin kwok-compute-domain-dra-plugin status-exporter status-exporter-kwok topology-server mig-faker compute-domain-controller compute-domain-dra-plugin

DOCKER_REPO_BASE=ghcr.io/run-ai/fake-gpu-operator
DOCKER_TAG?=0.0.0-dev
NAMESPACE=gpu-operator

SHOULD_PUSH?=false
DOCKER_BUILDX_PUSH_FLAG=$(if $(filter true,$(SHOULD_PUSH)),--push,)
DOCKER_BUILDX_PLATFORMS?=linux/amd64,linux/arm64
DOCKER_BUILDX_BUILDER?=fgo-multi-platform

OS?=linux
ARCH?=amd64

# Common helm --set flags for image tags (reused by setup and upgrade targets)
HELM_IMAGE_SETS=\
	--set draPlugin.image.tag="$(DOCKER_TAG)" \
	--set statusUpdater.image.tag="$(DOCKER_TAG)" \
	--set statusExporter.image.tag="$(DOCKER_TAG)" \
	--set topologyServer.image.tag="$(DOCKER_TAG)" \
	--set kwokDraPlugin.image.tag="$(DOCKER_TAG)" \
	--set computeDomainController.image.tag="$(DOCKER_TAG)" \
	--set computeDomainDraPlugin.image.tag="$(DOCKER_TAG)" \
	--set kwokComputeDomainDraPlugin.image.tag="$(DOCKER_TAG)"

build:
	for component in $(COMPONENTS); do \
		env GOOS=${OS} GOARCH=${ARCH} go build -o ${BUILD_DIR}/ ./cmd/$$component; \
	done
.PHONY: build

build-preloader:
	mkdir -p ${BUILD_DIR}
	gcc -fPIC -shared -o ${BUILD_DIR}/preloader ./cmd/preloader/main.c
.PHONY: build

lint: golangci-lint
	$(GOLANGCI_LINT) run -v --timeout 5m
.PHONY: lint

image:
	for component in $(COMPONENTS); do \
		docker buildx build -t ${DOCKER_REPO_BASE}/$$component:${DOCKER_TAG} --target $$component --platform ${DOCKER_BUILDX_PLATFORMS} ${DOCKER_BUILDX_PUSH_FLAG} .; \
	done
.PHONY: image

test: ginkgo
	$(GINKGO) ./internal/... ./cmd/... --procs=1 --output-dir=/tmp/artifacts/test-results/service-tests  --compilers=1 --randomize-all --randomize-suites --fail-on-pending  --keep-going --timeout=5m --race --trace  --json-report=report.json
.PHONY: test

# ─── Integration test targets ────────────────────────────────────────────────
# Full pipeline: setup → test old format → upgrade → test profiles → teardown
#   make integration
#
# Individual phases (cluster must already exist):
#   make test-integration-old-format
#   make upgrade-to-profiles
#   make test-integration-profiles

setup-integration:
	test/integration/setup.sh
.PHONY: setup-integration

teardown-integration:
	test/integration/teardown.sh
.PHONY: teardown-integration

# Phase 1: run tests against old-format values (deployed by setup.sh)
test-integration-old-format: ginkgo
	cd test/integration && \
		EXPECTED_GPU_PRODUCT="NVIDIA-A100-SXM4-40GB" \
		EXPECTED_GPU_COUNT="2" \
		EXPECTED_HIGHEND_GPU_PRODUCT="NVIDIA-H100-80GB-HBM3" \
		EXPECTED_HIGHEND_GPU_COUNT="4" \
		$(GINKGO) --procs=1 --timeout=30m --trace
.PHONY: test-integration-old-format

# Upgrade helm release to profile-based config, delete stale topology CMs,
# and wait for the system to reconverge.
upgrade-to-profiles:
	@echo "════ Upgrading Helm release to profile-based config ════"
	helm upgrade fake-gpu-operator deploy/fake-gpu-operator \
		--namespace $(NAMESPACE) \
		-f test/integration/values-profiles.yaml \
		$(HELM_IMAGE_SETS)
	@echo "Deleting per-node topology CMs (status-updater only creates, never updates)..."
	kubectl delete cm -n $(NAMESPACE) -l node-topology=true --ignore-not-found=true
	@echo "Deleting KWOK ResourceSlices..."
	for i in 1 2 3 4 5; do \
		kubectl delete resourceslice "kwok-kwok-gpu-node-$$i-gpu" --ignore-not-found=true; \
	done
	@echo "Waiting for rollouts..."
	kubectl rollout status deployment/status-updater -n $(NAMESPACE) --timeout=120s
	kubectl rollout status daemonset/nvidia-dcgm-exporter -n $(NAMESPACE) --timeout=120s
	kubectl rollout status deployment/kwok-dra-plugin -n $(NAMESPACE) --timeout=120s
	@echo "Waiting for topology CMs to be recreated..."
	@for node in kwok-gpu-node-1 kwok-gpu-node-2 kwok-gpu-node-3 kwok-gpu-node-4 kwok-gpu-node-5 \
		$$(kubectl get nodes -l "run.ai/simulated-gpu-node-pool" --no-headers -o custom-columns=NAME:.metadata.name 2>/dev/null | grep -v "^kwok-"); do \
		for i in $$(seq 1 30); do \
			if kubectl get cm -n $(NAMESPACE) -l "node-name=$$node" -o jsonpath='{.items[0].data.topology\.yml}' 2>/dev/null | grep -q "gpuProduct"; then \
				echo "  ✓ topology CM for $$node"; \
				break; \
			fi; \
			if [ "$$i" = "30" ]; then echo "ERROR: timed out waiting for $$node topology CM" && exit 1; fi; \
			sleep 2; \
		done; \
	done
	@echo "Waiting for KWOK ResourceSlices..."
	@for i in 1 2 3 4 5; do \
		for j in $$(seq 1 30); do \
			if kubectl get resourceslice "kwok-kwok-gpu-node-$$i-gpu" >/dev/null 2>&1; then \
				echo "  ✓ ResourceSlice for kwok-gpu-node-$$i"; \
				break; \
			fi; \
			if [ "$$j" = "30" ]; then echo "ERROR: timed out waiting for kwok-gpu-node-$$i ResourceSlice" && exit 1; fi; \
			sleep 2; \
		done; \
	done
	@echo "Restarting DRA plugin to pick up new topology..."
	kubectl delete pods -n $(NAMESPACE) -l app.kubernetes.io/component=kubeletplugin --ignore-not-found=true
	kubectl wait --for=condition=Ready pod -l app.kubernetes.io/component=kubeletplugin -n $(NAMESPACE) --timeout=120s
	@echo "Letting labels converge..."
	sleep 5
	@echo "════ Upgrade complete ════"
.PHONY: upgrade-to-profiles

# Phase 2: run tests against profile-based values
test-integration-profiles: ginkgo
	cd test/integration && \
		EXPECTED_GPU_PRODUCT="NVIDIA T4" \
		EXPECTED_GPU_COUNT="2" \
		EXPECTED_HIGHEND_GPU_PRODUCT="NVIDIA H100 80GB HBM3" \
		EXPECTED_HIGHEND_GPU_COUNT="4" \
		$(GINKGO) --procs=1 --timeout=30m --trace
.PHONY: test-integration-profiles

# Full integration pipeline: both formats in a single run
integration: setup-integration test-integration-old-format upgrade-to-profiles test-integration-profiles teardown-integration
.PHONY: integration

clean:
	rm -rf ${BUILD_DIR}
.PHONY: clean

# Tools
GINKGO=$(BUILD_DIR)/ginkgo
$(GINKGO):
	GOBIN=${BUILD_DIR} go install github.com/onsi/ginkgo/v2/ginkgo@v2.17.1

ginkgo: $(GINKGO)
.PHONY: ginkgo

GOLANGCI_LINT=$(BUILD_DIR)/golangci-lint
$(GOLANGCI_LINT):
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(BUILD_DIR) v2.1.2

golangci-lint: $(GOLANGCI_LINT)
.PHONY: golangci-lint
