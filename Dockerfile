FROM --platform=$BUILDPLATFORM golang:1.24.0 AS common-builder
WORKDIR $GOPATH/src/github.com/run-ai/fake-gpu-operator
COPY go.mod .
COPY go.sum .
RUN go mod download
COPY Makefile .
COPY internal/common ./internal/common
ARG TARGETOS TARGETARCH

FROM common-builder AS device-plugin-builder
COPY ./cmd/device-plugin/ ./cmd/device-plugin/
COPY ./internal/deviceplugin/ ./internal/deviceplugin/
RUN --mount=type=cache,target=/root/.cache/go-build make build OS=$TARGETOS ARCH=$TARGETARCH COMPONENTS=device-plugin

FROM common-builder AS status-updater-builder
COPY ./cmd/status-updater/ ./cmd/status-updater/
COPY ./internal/status-updater/ ./internal/status-updater/
RUN --mount=type=cache,target=/root/.cache/go-build make build OS=$TARGETOS ARCH=$TARGETARCH COMPONENTS=status-updater

FROM common-builder AS kwok-gpu-device-plugin-builder
COPY ./cmd/kwok-gpu-device-plugin/ ./cmd/kwok-gpu-device-plugin/
COPY ./internal/status-updater/ ./internal/status-updater/
COPY ./internal/kwok-gpu-device-plugin/ ./internal/kwok-gpu-device-plugin/
RUN --mount=type=cache,target=/root/.cache/go-build make build OS=$TARGETOS ARCH=$TARGETARCH COMPONENTS=kwok-gpu-device-plugin

FROM common-builder AS status-exporter-builder
COPY ./cmd/status-exporter/ ./cmd/status-exporter/
COPY ./internal/ ./internal/
RUN --mount=type=cache,target=/root/.cache/go-build make build OS=$TARGETOS ARCH=$TARGETARCH COMPONENTS=status-exporter

FROM common-builder AS topology-server-builder
COPY ./cmd/topology-server/ ./cmd/topology-server/
RUN --mount=type=cache,target=/root/.cache/go-build make build OS=$TARGETOS ARCH=$TARGETARCH COMPONENTS=topology-server

FROM common-builder AS nvidia-smi-builder
COPY ./cmd/nvidia-smi/ ./cmd/nvidia-smi/
RUN --mount=type=cache,target=/root/.cache/go-build make build OS=$TARGETOS ARCH=$TARGETARCH COMPONENTS=nvidia-smi

FROM common-builder AS mig-faker-builder
COPY ./cmd/mig-faker/ ./cmd/mig-faker/
COPY ./internal/ ./internal/
RUN --mount=type=cache,target=/root/.cache/go-build make build OS=$TARGETOS ARCH=$TARGETARCH COMPONENTS=mig-faker

FROM common-builder AS dra-plugin-gpu-builder
COPY ./cmd/dra-plugin-gpu/ ./cmd/dra-plugin-gpu/
COPY ./internal/dra-plugin-gpu/ ./internal/dra-plugin-gpu/
RUN --mount=type=cache,target=/root/.cache/go-build make build OS=$TARGETOS ARCH=$TARGETARCH COMPONENTS=dra-plugin-gpu

FROM common-builder AS preloader-builder 
COPY ./cmd/preloader/ ./cmd/preloader/
RUN make build-preloader

FROM jupyter/minimal-notebook AS jupyter-notebook
COPY --from=nvidia-smi-builder /go/src/github.com/run-ai/fake-gpu-operator/bin/nvidia-smi /bin/

FROM ubuntu AS device-plugin
COPY --from=device-plugin-builder /go/src/github.com/run-ai/fake-gpu-operator/bin/device-plugin /bin/
COPY --from=nvidia-smi-builder /go/src/github.com/run-ai/fake-gpu-operator/bin/nvidia-smi /bin/
COPY --from=preloader-builder /go/src/github.com/run-ai/fake-gpu-operator/bin/preloader /shared/memory/preloader.so
COPY --from=preloader-builder /go/src/github.com/run-ai/fake-gpu-operator/bin/preloader /shared/pid/preloader.so
ENTRYPOINT ["/bin/device-plugin"]

FROM ubuntu AS status-updater
COPY --from=status-updater-builder /go/src/github.com/run-ai/fake-gpu-operator/bin/status-updater /bin/
ENTRYPOINT ["/bin/status-updater"]

FROM ubuntu AS status-exporter
COPY --from=status-exporter-builder /go/src/github.com/run-ai/fake-gpu-operator/bin/status-exporter /bin/
ENTRYPOINT ["/bin/status-exporter"]

FROM ubuntu AS topology-server
COPY --from=topology-server-builder /go/src/github.com/run-ai/fake-gpu-operator/bin/topology-server /bin/
ENTRYPOINT ["/bin/topology-server"]

FROM ubuntu AS mig-faker
COPY --from=mig-faker-builder /go/src/github.com/run-ai/fake-gpu-operator/bin/mig-faker /bin/
ENTRYPOINT ["/bin/mig-faker"]

FROM ubuntu AS kwok-gpu-device-plugin
COPY --from=kwok-gpu-device-plugin-builder /go/src/github.com/run-ai/fake-gpu-operator/bin/kwok-gpu-device-plugin /bin/
ENTRYPOINT ["/bin/kwok-gpu-device-plugin"]

FROM ubuntu AS dra-plugin-gpu
COPY --from=dra-plugin-gpu-builder /go/src/github.com/run-ai/fake-gpu-operator/bin/dra-plugin-gpu /bin/
COPY --from=nvidia-smi-builder /go/src/github.com/run-ai/fake-gpu-operator/bin/nvidia-smi /bin/
ENTRYPOINT ["/bin/dra-plugin-gpu"]
