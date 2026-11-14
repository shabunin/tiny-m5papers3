# Usage: make launcher | paint | epdtest | touchtest | bench | wifiscan | psramtest | hello
#        make time | shot | monitor     (talk to the running launcher)
#        make test | sim | fonts        (on the PC)
#
# The launcher's WiFi app sets the clock from the internet with:
#        make launcher WIFI_SSID='My Network' WIFI_PASSWORD='secret'
TARGET ?= esp32s3-generic
PORT   ?= /dev/ttyACM0
FLASH   = tinygo flash -target $(TARGET) -port $(PORT)

WIFI_SSID     ?=
WIFI_PASSWORD ?=

.PHONY: launcher paint epdtest touchtest bench wifiscan psramtest hello monitor time shot test sim fonts

launcher:
	$(FLASH) -ldflags "-X 'main.wifiSSID=$(WIFI_SSID)' -X 'main.wifiPassword=$(WIFI_PASSWORD)'" ./examples/launcher
paint epdtest touchtest bench wifiscan psramtest:
	$(FLASH) ./examples/$@
hello:
	$(FLASH) -monitor ./hello
monitor:
	tinygo monitor -port $(PORT)

time:
	PAPERS3_PORT=$(PORT) tools/papers3.py time
shot:
	PAPERS3_PORT=$(PORT) tools/papers3.py shot papers3.png

test:
	go test ./...
sim:
	go run ./cmd/sim -out sim-out
fonts:
	go run ./cmd/fontgen
