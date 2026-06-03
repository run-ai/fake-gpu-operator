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

setup-e2e:
	test/e2e/scripts/setup.sh
.PHONY: setup-e2e

test-e2e: ginkgo
	cd test/e2e && $(GINKGO) --procs=1 --timeout=30m --trace
.PHONY: test-e2e

teardown-e2e:
	test/e2e/scripts/teardown.sh
.PHONY: teardown-e2e

e2e: setup-e2e test-e2e teardown-e2e
.PHONY: e2e

e2e-profiles:
	VALUES_FILE=$(shell pwd)/test/e2e/fixtures/values-profiles.yaml $(MAKE) e2e
.PHONY: e2e-profiles

setup-e2e-mock: ginkgo
	test/e2e/mock/scripts/setup.sh
.PHONY: setup-e2e-mock

test-e2e-mock: ginkgo
	cd test/e2e/mock && $(GINKGO) --procs=1 --timeout=30m --trace
.PHONY: test-e2e-mock

teardown-e2e-mock:
	test/e2e/mock/scripts/teardown.sh
.PHONY: teardown-e2e-mock

e2e-mock: setup-e2e-mock test-e2e-mock teardown-e2e-mock
.PHONY: e2e-mock

# The upgrade suite is split into three stages so the inner loop is fast:
#   make setup-e2e-upgrade    — kind cluster + baseline OCI install   (~3 min, once)
#   make upgrade-e2e-upgrade  — helm upgrade to local HEAD chart      (~30s,  per chart edit)
#   make test-e2e-upgrade     — Ginkgo assertions only                (~5s,   per check)
# Compose them via 'make e2e-upgrade' for end-to-end CI / one-shot local runs.

setup-e2e-upgrade:
	test/e2e/upgrade/scripts/setup.sh
.PHONY: setup-e2e-upgrade

upgrade-e2e-upgrade:
	test/e2e/upgrade/scripts/apply-upgrade.sh
.PHONY: upgrade-e2e-upgrade

test-e2e-upgrade: ginkgo
	cd test/e2e/upgrade && $(GINKGO) --procs=1 --timeout=15m --trace
.PHONY: test-e2e-upgrade

teardown-e2e-upgrade:
	test/e2e/upgrade/scripts/teardown.sh
.PHONY: teardown-e2e-upgrade

e2e-upgrade: setup-e2e-upgrade upgrade-e2e-upgrade test-e2e-upgrade teardown-e2e-upgrade
.PHONY: e2e-upgrade

# Convenience: run e2e-upgrade with the baseline pinned to the latest
# pullable chart from origin/main. Walks first-parent commits (only those
# get a published 0.0.0-<sha> chart) and falls back to the latest release
# tag if none is published yet. CI does the same via matrix.
e2e-upgrade-from-main:
	@CHART=oci://ghcr.io/run-ai/fake-gpu-operator/fake-gpu-operator; \
	git fetch origin main --depth=40 >/dev/null 2>&1 || true; \
	git fetch origin --tags --depth=1 >/dev/null 2>&1 || true; \
	for SHA in $$(git log origin/main --first-parent --format='%h' --max-count=15); do \
		VERSION="0.0.0-$$SHA"; \
		if helm pull $$CHART --version $$VERSION --destination /tmp >/dev/null 2>&1; then \
			echo "Using main baseline: $$VERSION"; \
			BASELINE_CHART_VERSION=$$VERSION $(MAKE) e2e-upgrade; \
			exit 0; \
		fi; \
		echo "  not pullable: $$VERSION"; \
	done; \
	LATEST=$$(git tag -l 'v*' --sort=-v:refname | sed -n '1s/^v//p'); \
	if [ -z "$$LATEST" ]; then echo "ERROR: no main chart and no release tag to fall back to"; exit 1; fi; \
	echo "No recent main chart; falling back to latest release: $$LATEST"; \
	BASELINE_CHART_VERSION=$$LATEST $(MAKE) e2e-upgrade
.PHONY: e2e-upgrade-from-main

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
