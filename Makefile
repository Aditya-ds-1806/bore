.PHONY: protos build

build:
	go build -o bore cmd/main.go

protos:
	rm -rf borepb
	mkdir -p borepb
	protoc --go_out=./borepb protos/*.proto
