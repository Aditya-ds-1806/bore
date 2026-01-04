.PHONY: protos

build-client:
	go build -o bore -ldflags "-X 'main.AppMode=client' -X 'bore/internal/client.BoreServerHost=43.204.144.35'" cmd/main.go

build-server:
	go build -o bore-server -ldflags "-X 'main.AppMode=server'" cmd/main.go

run-client:
	go run -race -ldflags "-X 'main.AppMode=client' -X 'bore/internal/client.BoreServerHost=127.0.0.1'" cmd/main.go

run-server:
	go run -race -ldflags "-X 'main.AppMode=server'" cmd/main.go

protos:
	rm -rf borepb
	mkdir -p borepb
	protoc --go_out=./borepb protos/*.proto
