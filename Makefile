BUILD_DIR=$(shell pwd)/bin
COMPONENT="$1"

DOCKER_REPO_BASE=gcr.io/run-ai-lab/fake-gpu-operator
DOCKER_REPO_FULL=${DOCKER_REPO_BASE}/${COMPONENT}
DOCKER_TAG=0.0.0-dev
DOCKER_IMAGE_NAME=${DOCKER_REPO_FULL}:${DOCKER_TAG}
NAMESPACE=gpu-operator

SHOULD_PUSH?=false
DOCKER_BUILDX_PUSH_FLAG=$(if $(filter true,$(SHOULD_PUSH)),--push,)
DOCKER_BUILDX_PLATFORMS?=linux/amd64,linux/arm64
DOCKER_BUILDX_BUILDER?=fgo-multi-platform

OS?=linux
ARCH?=amd64

build:
	env GOOS=${OS} GOARCH=${ARCH} go build -o ${BUILD_DIR}/ ./cmd/${COMPONENT}
.PHONY: build

build-preloader:
	mkdir -p ${BUILD_DIR}
	gcc -fPIC -shared -o ${BUILD_DIR}/preloader ./cmd/preloader/main.c
.PHONY: build

clean:
	rm -rf ${BUILD_DIR}
.PHONY: clean

init-buildx:
	docker buildx inspect fgo-multi-platform > /dev/null || docker buildx create --name=fgo-multi-platform
.PHONY: init-buildx

image: init-buildx
	docker buildx --builder=fgo-multi-platform build -t ${DOCKER_IMAGE_NAME} --target ${COMPONENT} --platform ${DOCKER_BUILDX_PLATFORMS} ${DOCKER_BUILDX_PUSH_FLAG} .
.PHONY: image

images:
	make image COMPONENT=device-plugin
	make image COMPONENT=status-updater
	make image COMPONENT=status-exporter
	make image COMPONENT=topology-server
	make image COMPONENT=mig-faker
	make image COMPONENT=jupyter-notebook
.PHONY: images

restart: 
	kubectl delete pod -l component=${COMPONENT} --force -n ${NAMESPACE}
.PHONY: restart

image-test:
	mkdir -p /tmp/artifacts/test-results
	mkdir -p /tmp/artifacts/test-results/unit-tests
	mkdir -p /tmp/artifacts/test-results/service-tests
	docker build -t test-image --target test .
.PHONY: image-test

GINKGO=$(BUILD_DIR)/ginkgo
$(GINKGO):
	GOBIN=${BUILD_DIR} go install github.com/onsi/ginkgo/v2/ginkgo@v2.6.0

test-all: $(GINKGO)
	$(GINKGO) -r --procs=1 --output-dir=/tmp/artifacts/test-results/service-tests  --compilers=1 --randomize-all --randomize-suites --fail-on-pending  --keep-going --timeout=5m --race --trace  --json-report=report.json
.PHONY: test-all
