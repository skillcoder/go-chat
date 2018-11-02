APP?=go-chat-server
GOOS?=linux

.PHONY: build
build: clean
	CGO_ENABLED=0 GOOS=${GOOS} go build \
        -o bin/${GOOS}/${APP}

.PHONY: check
check:
	gometalinter --skip vendor --config=../.gometalinter.json ./...

.PHONY: clean
clean:
	@rm -f bin/${GOOS}/${APP}

.PHONY: vendor
vendor:
	glide update

.PHONY: fmt
fmt:
	go fmt

