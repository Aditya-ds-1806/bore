.PHONY: protos

build-client:
	go build -o build/bore \
		-ldflags "\
			-X 'bore/internal/client.BoreServerHost=app.trybore.com' \
			-X 'bore/internal/client.WSScheme=wss' \
			-X 'main.AppVersion=0.1.0'" \
		cmd/bore/main.go

build-server:
	go build -o build/bore-server \
		-ldflags "\
			-X 'main.AppVersion=0.1.0'" \
		cmd/bore-server/main.go

protos:
	rm -rf borepb
	mkdir -p borepb
	protoc --go_out=./borepb protos/*.proto
