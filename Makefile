BINARY := wsproxy
CMD := ./cmd/server
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X 'main.Version=$(VERSION)' -X 'main.Commit=$(COMMIT)' -X 'main.BuildDate=$(BUILD_DATE)'

.PHONY: build install run tui test clean sync

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) $(CMD)

install:
	@GOBIN=$$(go env GOBIN); \
	if [ -z "$$GOBIN" ]; then GOBIN="$$(go env GOPATH)/bin"; fi; \
	mkdir -p "$$GOBIN"; \
	go build -ldflags "$(LDFLAGS)" -o "$$GOBIN/$(BINARY)" $(CMD) && \
	echo "Installed $$GOBIN/$(BINARY)"

run:
	go run $(CMD)

tui:
	go run $(CMD) --tui

test:
	go test ./...

clean:
	rm -f $(BINARY)

sync:
	@if [ "$$(git rev-parse --abbrev-ref HEAD)" != "main" ]; then \
		echo "Error: switch to main before syncing"; exit 1; \
	fi
	@if [ -n "$$(git status --porcelain)" ]; then \
		echo "Error: working tree is dirty; commit or stash changes first"; exit 1; \
	fi
	git fetch upstream --no-tags
	git merge upstream/main
	$(MAKE) build
	$(MAKE) test
