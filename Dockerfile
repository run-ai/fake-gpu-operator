FROM golang:1.18 as common-builder

WORKDIR /go/src/github.com/run-ai/fake-gpu-operator
COPY go.mod .
COPY go.sum .
RUN go mod download

COPY Makefile .
COPY internal/common ./internal/common

FROM common-builder as device-plugin-builder

COPY cmd/device-plugin /go/src/github.com/run-ai/fake-gpu-operator/bin/device-plugin/cmd/device-plugin
COPY internal/deviceplugin /go/src/github.com/run-ai/fake-gpu-operator/bin/device-plugin/internal/deviceplugin
RUN make build

FROM common-builder as status-updater-builder

COPY cmd/status-updater /go/src/github.com/run-ai/fake-gpu-operator/bin/device-plugin/cmd/status-updater
COPY internal/status-updater /go/src/github.com/run-ai/fake-gpu-operator/bin/device-plugin/internal/status-updater
RUN make build

FROM common-builder as status-exporter-builder

COPY cmd/status-exporter /go/src/github.com/run-ai/fake-gpu-operator/bin/device-plugin/cmd/status-exporter
COPY internal/status-exporter /go/src/github.com/run-ai/fake-gpu-operator/bin/device-plugin/internal/status-exporter
RUN make build

FROM golang:1.18 as device-plugin

COPY --from=device-plugin-builder /go/src/github.com/run-ai/fake-gpu-operator/bin/device-plugin /bin/
ENTRYPOINT ["/bin/device-plugin"]

FROM golang:1.18 as status-updater

COPY --from=status-updater-builder /go/src/github.com/run-ai/fake-gpu-operator/bin/status-updater /bin/
ENTRYPOINT ["/bin/status-updater"]

FROM golang:1.18 as status-exporter

COPY --from=status-exporter-builder /go/src/github.com/run-ai/fake-gpu-operator/bin/status-exporter /bin/
ENTRYPOINT ["/bin/status-exporter"]


###### Debug images
FROM golang:1.18 as debugger
RUN go install github.com/go-delve/delve/cmd/dlv@latest

FROM debugger as device-plugin-debug

COPY --from=device-plugin-builder /build/bin/device-plugin /bin/
ENTRYPOINT ["/go/bin/dlv", "exec", "--headless", "-l", ":10000", "--api-version=2", "/bin/device-plugin", "--"]

FROM debugger as status-updater-debug

COPY --from=status-updater-builder /build/bin/status-updater /bin/
ENTRYPOINT ["/go/bin/dlv", "exec", "--headless", "-l", ":10000", "--api-version=2", "/bin/status-updater", "--"]
