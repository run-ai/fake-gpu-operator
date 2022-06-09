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
RUN make build COMPONENT=status-updater

FROM common-builder as status-exporter-builder
COPY ./cmd/status-exporter/ ./cmd/status-exporter/
COPY ./internal/status-exporter/ ./internal/status-exporter/
RUN make build COMPONENT=status-exporter

FROM --platform=linux/amd64 alpine:3.16.0 as minimal-builder

FROM minimal-builder as device-plugin
COPY --from=device-plugin-builder /go/src/github.com/run-ai/fake-gpu-operator/bin/device-plugin /bin/
ENTRYPOINT ["/bin/device-plugin"]

FROM minimal-builder as status-updater
COPY --from=status-updater-builder /go/src/github.com/run-ai/fake-gpu-operator/bin/status-updater /bin/
ENTRYPOINT ["/bin/status-updater"]

FROM minimal-builder as status-exporter
COPY --from=status-exporter-builder /go/src/github.com/run-ai/fake-gpu-operator/bin/status-exporter /bin/
ENTRYPOINT ["/bin/status-exporter"]