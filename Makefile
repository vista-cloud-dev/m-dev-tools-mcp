# m-dev-tools-mcp — a thin MCP server over the m toolchain (spec §5.5).
# Inherits the go-cli-template build conventions for the m-cli Go toolchain.
# Every toolchain Go repo inherits this: static (CGO_ENABLED=0), -trimpath,
# version stamped via -ldflags, cross-compile matrix, lint, test, schema.

BIN     ?= m-dev-tools-mcp
PKG     := github.com/vista-cloud-dev/m-dev-tools-mcp
LDPKG   := github.com/vista-cloud-dev/clikit
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%d)
LDFLAGS := -s -w -X $(LDPKG).Version=$(VERSION) -X $(LDPKG).Commit=$(COMMIT) -X $(LDPKG).Date=$(DATE)

# Static, no-libc, reproducible (spec §10).
GOFLAGS := -trimpath
export CGO_ENABLED := 0

PLATFORMS := linux/amd64 linux/arm64 darwin/arm64 windows/amd64

.PHONY: all build run lint test tidy schema dist clean

all: lint test build

build:
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o dist/$(BIN) .

run: build
	./dist/$(BIN) $(ARGS)

lint:
	golangci-lint run ./...

test:
	CGO_ENABLED=1 go test $(GOFLAGS) -race -cover ./...

tidy:
	go mod tidy

# Emit the machine schema (the §5.5 contract) — also a CI conformance artifact.
schema: build
	./dist/$(BIN) schema

# Cross-compile the pinned matrix into dist/.
dist:
	@mkdir -p dist
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; ext=""; [ "$$os" = "windows" ] && ext=".exe"; \
		echo "  $$os/$$arch"; \
		GOOS=$$os GOARCH=$$arch go build $(GOFLAGS) -ldflags "$(LDFLAGS)" \
			-o dist/$(BIN)-$$os-$$arch$$ext . ; \
	done

clean:
	rm -rf dist
