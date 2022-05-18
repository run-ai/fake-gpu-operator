FROM golang:1.18 as common_builder

WORKDIR /build
COPY go.mod .
COPY go.sum .
RUN go mod download

COPY Makefile .
COPY internal/common ./internal/common

FROM common_builder as device_plugin_builder

COPY cmd/device-plugin ./cmd/device-plugin
COPY internal/deviceplugin ./internal/deviceplugin
RUN make compile

FROM common_builder as status_updater_builder

COPY cmd/status-updater ./cmd/status-updater
COPY internal/status-updater ./internal/status-updater
RUN make compile

FROM common_builder as status_exporter_builder

COPY cmd/status-exporter ./cmd/status-exporter
COPY internal/status-exporter ./internal/status-exporter
RUN make compile

FROM golang:1.18 as device_plugin

COPY --from=device_plugin_builder /build/bin/device-plugin /bin/
ENTRYPOINT ["/bin/device-plugin"]

FROM golang:1.18 as status_updater

COPY --from=status_updater_builder /build/bin/status-updater /bin/
ENTRYPOINT ["/bin/status-updater"]

FROM golang:1.18 as status_exporter

COPY --from=status_exporter_builder /build/bin/status-exporter /bin/
ENTRYPOINT ["/bin/status-exporter"]


###### Debug images
FROM golang:1.18 as debugger
RUN go install github.com/go-delve/delve/cmd/dlv@latest

FROM debugger as device_plugin_debug

COPY --from=device_plugin_builder /build/bin/device-plugin /bin/
ENTRYPOINT ["/go/bin/dlv", "exec", "--headless", "-l", ":10000", "--api-version=2", "/bin/device-plugin", "--"]

FROM debugger as status_updater_debug

COPY --from=status_updater_builder /build/bin/status-updater /bin/
ENTRYPOINT ["/go/bin/dlv", "exec", "--headless", "-l", ":10000", "--api-version=2", "/bin/status-updater", "--"]
