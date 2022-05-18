BUILD_DIR="./bin"

compile:
	go build -o ${BUILD_DIR}/ ./cmd/...

device-plugin-image:
	docker build -t device-plugin --target device_plugin .

status-updater-image:
	docker build -t status-updater --target status_updater .

status-exporter-image:
	docker build -t status-exporter --target status_exporter .

images: device-plugin-image status-updater-image status-exporter-image

all: compile-all
