.PHONY: protos

VERSION := dev

print-version:
	@echo $(VERSION)

build-client:
	go build -o build/bore \
		-ldflags "\
			-X 'bore/internal/client.BoreServerHost=app.trybore.com' \
			-X 'bore/internal/client.WSScheme=wss' \
			-X 'main.AppVersion=$(VERSION)'" \
		cmd/bore/main.go

build-server:
	go build -o build/bore-server \
		-ldflags "\
			-X 'main.AppVersion=$(VERSION)'" \
		cmd/bore-server/main.go

protos:
	rm -rf borepb
	mkdir -p borepb
	protoc --go_out=./borepb protos/*.proto -I protos
