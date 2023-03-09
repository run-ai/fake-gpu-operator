FROM golang:1.18.2 as common-builder
WORKDIR $GOPATH/src/github.com/run-ai/fake-gpu-operator
COPY go.mod .
COPY go.sum .
RUN go mod download
COPY Makefile .
COPY internal/common ./internal/common

FROM common-builder as device-plugin-builder
COPY ./cmd/device-plugin/ ./cmd/device-plugin/
COPY ./internal/deviceplugin/ ./internal/deviceplugin/
RUN make build COMPONENT=device-plugin

FROM common-builder as status-updater-builder
COPY ./cmd/status-updater/ ./cmd/status-updater/
COPY ./internal/status-updater/ ./internal/status-updater/
RUN --mount=type=cache,target=/root/.cache/go-build make build COMPONENT=status-updater

FROM common-builder as status-exporter-builder
COPY ./cmd/status-exporter/ ./cmd/status-exporter/
COPY ./internal/ ./internal/
RUN --mount=type=cache,target=/root/.cache/go-build make build COMPONENT=status-exporter

FROM common-builder as topology-server-builder
COPY ./cmd/topology-server/ ./cmd/topology-server/
RUN --mount=type=cache,target=/root/.cache/go-build make build COMPONENT=topology-server

FROM common-builder as nvidia-smi-builder
COPY ./cmd/nvidia-smi/ ./cmd/nvidia-smi/
RUN make build COMPONENT=nvidia-smi

FROM common-builder as mig-faker-builder
COPY ./cmd/mig-faker/ ./cmd/mig-faker/
COPY ./internal/ ./internal/
RUN --mount=type=cache,target=/root/.cache/go-build make build COMPONENT=mig-faker

FROM ubuntu as device-plugin
COPY --from=device-plugin-builder /go/src/github.com/run-ai/fake-gpu-operator/bin/device-plugin /bin/
COPY --from=nvidia-smi-builder /go/src/github.com/run-ai/fake-gpu-operator/bin/nvidia-smi /bin/
ENTRYPOINT ["/bin/device-plugin"]

FROM ubuntu as status-updater
COPY --from=status-updater-builder /go/src/github.com/run-ai/fake-gpu-operator/bin/status-updater /bin/
ENTRYPOINT ["/bin/status-updater"]

FROM ubuntu as status-exporter
COPY --from=status-exporter-builder /go/src/github.com/run-ai/fake-gpu-operator/bin/status-exporter /bin/
ENTRYPOINT ["/bin/status-exporter"]

FROM ubuntu as topology-server
COPY --from=topology-server-builder /go/src/github.com/run-ai/fake-gpu-operator/bin/topology-server /bin/
ENTRYPOINT ["/bin/topology-server"]

FROM ubuntu as mig-faker
COPY --from=mig-faker-builder /go/src/github.com/run-ai/fake-gpu-operator/bin/mig-faker /bin/
ENTRYPOINT ["/bin/mig-faker"]