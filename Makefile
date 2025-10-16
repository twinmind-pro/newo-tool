BIN_NAME := newo
BUILD_DIR := bin
GOBIN ?= $(abspath $(BUILD_DIR))
override LINT := golangci-lint
override VULNCHECK := govulncheck

.PHONY: build install clean lint test race vuln fmt

VERSION := $(shell git describe --tags --always)
COMMIT := $(shell git rev-parse HEAD)

build: fmt lint
	@mkdir -p $(BUILD_DIR)
	@echo "LDFLAGS: -X 'github.com/twinmind/newo-tool/internal/version.Version=$(VERSION)' -X 'github.com/twinmind/newo-tool/internal/version.Commit=$(COMMIT)'"
	go build -ldflags "-X 'github.com/twinmind/newo-tool/internal/version.Version=$(VERSION)' -X 'github.com/twinmind/newo-tool/internal/version.Commit=$(COMMIT)'" -o $(BUILD_DIR)/$(BIN_NAME) ./cmd/newo

install:
	@mkdir -p $(GOBIN)
	GOBIN=$(GOBIN) go install ./cmd/newo
	@echo "Installed to $(GOBIN)/$(BIN_NAME)"

clean:
	rm -rf $(BUILD_DIR)

lint:
	@mkdir -p $(LINT_CACHE)
	@GOLANGCI_LINT_CACHE=$(LINT_CACHE) GOCACHE=$(abspath .gocache) $(LINT) run ./... || { \
		echo "lint failed. Ensure $(LINT) is installed (e.g. brew install golangci-lint)"; \
		exit 1; \
	}

TEST_TARGET ?= ./...

test:
	go run ./cmd/tester $(TEST_TARGET)

race:
	go test -race $(TEST_TARGET)

vuln:
	@$(VULNCHECK) ./... || { \
		echo "vuln scan failed. Ensure $(VULNCHECK) is installed (go install golang.org/x/vuln/cmd/govulncheck@latest)"; \
		exit 1; \
	}

fmt:
	@gofmt -w $(shell go list -f '{{.Dir}}' ./...)
LINT_CACHE ?= $(abspath .golangci-cache)
