FROM golang:1.18.2 as common-builder

WORKDIR $GOPATH/src/github.com/run-ai/fake-gpu-operator
COPY go.mod .
COPY go.sum .
RUN go mod download

COPY Makefile .
COPY internal/common ./internal/common


FROM common-builder as device-plugin-builder

COPY cmd/device-plugin ./cmd/device-plugin
COPY internal/deviceplugin ./internal/deviceplugin
RUN pwd
RUN make build


FROM common-builder as status-updater-builder

COPY cmd/status-updater ./cmd/status-updater
COPY internal/status-updater ./internal/status-updater
RUN make build


FROM common-builder as status-exporter-builder

COPY cmd/status-exporter ./cmd/status-exporter
COPY internal/status-exporter ./internal/status-exporter
RUN make build


FROM common-builder as test
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN mkdir -p /tmp/artifacts
RUN make test-all

FROM alpine:3.16.0 as device-plugin
COPY --from=device-plugin-builder /go/src/github.com/run-ai/fake-gpu-operator/bin/device-plugin /bin/

ENTRYPOINT ["/bin/device-plugin"]

FROM alpine:3.16.0 as status-updater

COPY --from=status-updater-builder /go/src/github.com/run-ai/fake-gpu-operator/bin/status-updater /bin/
ENTRYPOINT ["/bin/status-updater"]

FROM alpine:3.16.0 as status-exporter

COPY --from=status-exporter-builder /go/src/github.com/run-ai/fake-gpu-operator/bin/status-exporter /bin/
ENTRYPOINT ["/bin/status-exporter"]


###### Debug images
# FROM golang:1.18.2-alpine as debugger
# RUN go install github.com/go-delve/delve/cmd/dlv@latest

# FROM debugger as device-plugin-debug

# RUN pwd
# RUN ls -la
# COPY --from=device-plugin-builder $GOPATH/src/github.com/run-ai/fake-gpu-operator/bin/device-plugin /bin/
# ENTRYPOINT ["/go/bin/dlv", "exec", "--headless", "-l", ":10000", "--api-version=2", "/bin/device-plugin", "--"]

# FROM debugger as status-updater-debug

# COPY --from=status-updater-builder $GOPATH/src/github.com/run-ai/fake-gpu-operator/bin/status-updater /bin/
# ENTRYPOINT ["/go/bin/dlv", "exec", "--headless", "-l", ":10000", "--api-version=2", "/bin/status-updater", "--"]
