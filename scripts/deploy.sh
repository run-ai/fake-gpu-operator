#!/bin/bash

# Usage:
# ./deploy.sh [component]
# component: [device-plugin|status-updater|status-exporter]

COMPONENT_NAME=$1
# You can also put these in the git ignored .env file
DOCKER_REPO_BASE=${DOCKER_REPO_BASE:-"localhost:5001/fake-gpu-operator"}
DOCKER_REPO_FULL=${DOCKER_REPO_FULL:-"$DOCKER_REPO_BASE/$1"}
DOCKER_TAG=${DOCKER_TAG:-"shaibi"}
DOCKER_IMAGE_NAME=${DOCKER_IMAGE_NAME:-"$DOCKER_REPO_FULL:$DOCKER_TAG"}
NAMESPACE=${NAMESPACE:-"gpu-operator"}

if [ -f .env ]; then
  export $(cat .env | xargs)
fi

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )" &&

make -C $DIR/.. ${COMPONENT_NAME}-image &&

docker tag ${COMPONENT_NAME} ${DOCKER_IMAGE_NAME} &&
docker push ${DOCKER_IMAGE_NAME}

kubectl delete pod -l component=${COMPONENT_NAME} --force -n ${NAMESPACE}