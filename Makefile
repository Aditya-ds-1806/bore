.PHONY: protos

build-client:
	go build -o bore -ldflags "-X 'main.AppMode=client' -X 'bore/internal/client.BoreServerHost=trybore.com'" cmd/main.go

build-server:
	go build -o bore-server -ldflags "-X 'main.AppMode=server'" cmd/main.go

run-client:
	go run -race -ldflags "-X 'main.AppMode=client' -X 'bore/internal/client.BoreServerHost=127.0.0.1'" cmd/main.go

run-server:
	go run -race -ldflags "-X 'main.AppMode=server'" cmd/main.go

start-server:
	nginx -t -c $(PWD)/nginx.conf
	nginx -c $(PWD)/nginx.conf
	./bore-server

protos:
	rm -rf borepb
	mkdir -p borepb
	protoc --go_out=./borepb protos/*.proto
