.PHONY: build clean test-unit test-integration lint install release image push

BINARY := fray
IMAGE_REPO ?= ghcr.io/hexfusion/fray
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +%Y%m%d)
GIT_TREE_STATE := $(shell if git diff --quiet 2>/dev/null; then echo "clean"; else echo "dirty"; fi)

VERSION_PKG := github.com/hexfusion/fray/pkg/version
LDFLAGS := -s -w \
	-X $(VERSION_PKG).version=$(VERSION) \
	-X $(VERSION_PKG).commit=$(COMMIT) \
	-X $(VERSION_PKG).buildDate=$(BUILD_DATE) \
	-X $(VERSION_PKG).gitTreeState=$(GIT_TREE_STATE)

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/fray

clean:
	rm -f $(BINARY)
	rm -rf dist/

test-unit:
	go test -race ./...

test-integration:
	go test -race -tags=integration ./test/...

lint:
	golangci-lint run

install: build
	install -m 755 $(BINARY) /usr/local/bin/

release: clean
	mkdir -p dist
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-linux-amd64 ./cmd/fray
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-linux-arm64 ./cmd/fray
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-darwin-amd64 ./cmd/fray
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-darwin-arm64 ./cmd/fray

image:
	podman build --platform linux/amd64,linux/arm64 \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		--build-arg GIT_TREE_STATE=$(GIT_TREE_STATE) \
		--manifest $(IMAGE_REPO):$(VERSION) .

push:
	podman manifest push $(IMAGE_REPO):$(VERSION) $(IMAGE_REPO):$(VERSION)
	podman manifest push $(IMAGE_REPO):$(VERSION) $(IMAGE_REPO):latest
