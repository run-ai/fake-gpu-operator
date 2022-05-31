BUILD_DIR=./bin
COMPONENT="$1"

DOCKER_REPO_BASE=gcr.io/run-ai-lab/fake-gpu-operator
DOCKER_REPO_FULL=${DOCKER_REPO_BASE}/${COMPONENT}
DOCKER_TAG=0.0.2
DOCKER_IMAGE_NAME=${DOCKER_REPO_FULL}:${DOCKER_TAG}
NAMESPACE=gpu-operator

build:
	go build -a -o ${BUILD_DIR}/ ./cmd/...

clean:
	rm -rf ${BUILD_DIR}

image:
	docker build -t ${DOCKER_IMAGE_NAME} --target ${COMPONENT} .
.PHONY: image

images:
	make image COMPONENT=device-plugin
	make image COMPONENT=status-updater
	make image COMPONENT=status-exporter
.PHONY: images

push:
	docker push ${DOCKER_IMAGE_NAME}
.PHONY: push

restart: 
	kubectl delete pod -l component=${COMPONENT} --force -n ${NAMESPACE}
.PHONY: restart

deploy: image push
.PHONY: deploy

deploy-all:
	make image push COMPONENT=device-plugin
	make image push COMPONENT=status-updater
	make image push COMPONENT=status-exporter
.PHONY: deploy-all

test-all:
	ginkgo run ./...
.PHONY: test-all