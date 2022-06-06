#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

readonly HELM_URL=https://get.helm.sh
readonly HELM_TARBALL=helm-v3.0.2-linux-amd64.tar.gz

main() {
    setup_helm_client
    authenticate
    upload
}

setup_helm_client() {
    echo "Setting up Helm client..."

    curl --user-agent curl-ci-sync -sSL -o "$HELM_TARBALL" "$HELM_URL/$HELM_TARBALL"
    tar xzfv "$HELM_TARBALL"

    PATH="$(pwd)/linux-amd64/:$PATH"
} 

authenticate() {
    echo "Authenticating with Google Cloud..."
    echo "$STAGING_GCLOUD_SERVICE_JSON_CONTENT" | base64 -d > ./my.json
    gcloud auth activate-service-account staging-ci@run-ai-staging.iam.gserviceaccount.com --key-file ./my.json
}

upload() {
    echo "Uploading the new Run:AI fake-gpu helm chart to $UPLOAD_TARGET..."
    cd deploy/fake-gpu-operator/
    CHART_VERSION=$(echo "$PIPELINE_NUMBER-$CIRCLE_SHA1" | awk -F '-' '{short_sha=substr($2,1,7); printf("%s-%s",$1, short_sha)}')
    local sync_dir="stable-sync"
    local index_dir="stable-index"

    if [[ "$UPLOAD_TARGET" == "prod" ]]; then
      CHART_VERSION=${CIRCLE_TAG/v/''}
      sed -i "s/tag:.*/tag: $CHART_VERSION/g" values.yaml
    else
      sed -i "s/tag:.*/tag: $CIRCLE_SHA1/g" values.yaml
    fi

    mkdir -p "$sync_dir"
    if ! gsutil cp "$BUCKET/index.yaml" "$index_dir/index.yaml"; then
        echo "[ERROR] Exiting because unable to copy index locally. Not safe to proceed."
        exit 1
    fi
    sed -i "s/"CHART_VERSION"/$CHART_VERSION/g" Chart.yaml
    helm repo add ingress-nginx "https://kubernetes.github.io/ingress-nginx"
    helm repo update
    helm dep update .
    helm package . -n runai --destination "$sync_dir"
    if helm repo index --url "$REPO_URL" --merge "$index_dir/index.yaml" "$sync_dir"; then
        # Move updated index.yaml to sync folder so we don't push the old one again
        mv -f "$sync_dir/index.yaml" "$index_dir/index.yaml"

        gsutil -h "Cache-Control:no-cache,max-age=0" -m rsync "$sync_dir" "$BUCKET"

        # Make sure index.yaml is synced last
        gsutil -h "Cache-Control:no-cache,max-age=0" cp "$index_dir/index.yaml" "$BUCKET"
    else
            echo "[ERROR] Exiting because unable to update index. Not safe to push update."
            exit 1
    fi

    return 0
}

log_error() {
    printf '\e[31mERROR: %s\n\e[39m' "$1" >&2
}

main
