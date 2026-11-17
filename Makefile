BINARY  := ola
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
GOBIN   ?= $(shell go env GOBIN)
ifeq ($(GOBIN),)
GOBIN   := $(shell go env GOPATH)/bin
endif

# Targets cross-compiled by `make dist` (used by the release workflow).
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

.PHONY: all build install uninstall vet dist clean

all: build

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) .

install:
	go build -ldflags "$(LDFLAGS)" -o $(GOBIN)/$(BINARY) .
	@echo "Installed $(GOBIN)/$(BINARY)"

uninstall:
	rm -f $(GOBIN)/$(BINARY)

vet:
	go vet ./...

dist:
	rm -rf dist && mkdir -p dist
	@set -e; for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; ext=; [ $$os = windows ] && ext=.exe; \
		name=$(BINARY)_$(VERSION)_$${os}_$${arch}; \
		echo "building $$name"; \
		mkdir -p dist/$$name; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$$name/$(BINARY)$$ext .; \
		cp README.md dist/$$name/; \
		if [ $$os = windows ]; then (cd dist && zip -qr $$name.zip $$name); \
		else tar -C dist -czf dist/$$name.tar.gz $$name; fi; \
		rm -rf dist/$$name; \
	done
	cd dist && sha256sum * > checksums.txt

clean:
	rm -rf $(BINARY) dist
