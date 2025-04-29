BUILD_DIR=$(shell pwd)/bin
COMPONENTS?=device-plugin status-updater kwok-gpu-device-plugin status-exporter topology-server mig-faker jupyter-notebook

DOCKER_REPO_BASE=gcr.io/run-ai-lab/fake-gpu-operator
DOCKER_TAG?=0.0.0-dev
NAMESPACE=gpu-operator

SHOULD_PUSH?=false
DOCKER_BUILDX_PUSH_FLAG=$(if $(filter true,$(SHOULD_PUSH)),--push,)
DOCKER_BUILDX_PLATFORMS?=linux/amd64,linux/arm64
DOCKER_BUILDX_BUILDER?=fgo-multi-platform

OS?=linux
ARCH?=amd64

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
	$(GINKGO) -r --procs=1 --output-dir=/tmp/artifacts/test-results/service-tests  --compilers=1 --randomize-all --randomize-suites --fail-on-pending  --keep-going --timeout=5m --race --trace  --json-report=report.json
.PHONY: test

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
