GOOS?=linux

.PHONY: build
build: build-client build-server

.PHONY: proto
proto:
	protoc --go_out="plugins=grpc:." proto/chat.proto

.PHONY: build-client
build-client:
	echo build-client

.PHONY: build-server
build-server:
	echo build-server

