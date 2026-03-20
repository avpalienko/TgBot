BINARY       := tgbot
CMD          := ./cmd/bot
PKG          := github.com/user/tgbot/internal/version
DOCKER_USER  ?= $(error set DOCKER_USER, e.g. make docker-build DOCKER_USER=myuser)
DOCKER_TAG   ?= latest
DOCKER_IMAGE  = $(DOCKER_USER)/tgbot:$(DOCKER_TAG)

GIT_COMMIT  := $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
GIT_DATE    := $(shell git log -1 --format=%ci 2>/dev/null || echo unknown)
GIT_BRANCH  := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo unknown)
BUILD_DATE  := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || echo unknown)

LDFLAGS := -s -w \
    -X '$(PKG).GitCommit=$(GIT_COMMIT)' \
    -X '$(PKG).GitDate=$(GIT_DATE)' \
    -X '$(PKG).GitBranch=$(GIT_BRANCH)' \
    -X '$(PKG).BuildDate=$(BUILD_DATE)'

COVERAGE_OUT  := coverage.out
COVERAGE_HTML := coverage.html
COVERAGE_THRESHOLD := 60

.DEFAULT_GOAL := help

## Build & Run ----------------------------------------------------------------

.PHONY: build
build: ## Build binary for current platform
	go build -ldflags="$(LDFLAGS)" -o $(BINARY) $(CMD)
	@echo "Built $(BINARY)"

.PHONY: build-linux
build-linux: ## Cross-compile for linux/amd64
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BINARY) $(CMD)
	@echo "Built $(BINARY) (linux/amd64)"

.PHONY: run
run: build ## Build and run
	./$(BINARY)

## Quality ---------------------------------------------------------------------

.PHONY: test
test: ## Run tests with race detector
	go test -race -count=1 ./...

.PHONY: cover
cover: ## Run tests with coverage report
	go test -race -coverprofile=$(COVERAGE_OUT) -covermode=atomic ./...
	go tool cover -func=$(COVERAGE_OUT)
	go tool cover -html=$(COVERAGE_OUT) -o $(COVERAGE_HTML)
	@echo "HTML report: $(COVERAGE_HTML)"
	@total=$$(go tool cover -func=$(COVERAGE_OUT) | grep '^total:' | awk '{print $$NF}' | tr -d '%'); \
	if [ "$$(echo "$$total < $(COVERAGE_THRESHOLD)" | bc -l)" -eq 1 ]; then \
	    echo "FAIL: coverage $${total}% < $(COVERAGE_THRESHOLD)% threshold"; exit 1; \
	else \
	    echo "OK: coverage $${total}% >= $(COVERAGE_THRESHOLD)% threshold"; \
	fi

.PHONY: lint
lint: ## Run go vet and golangci-lint
	go vet ./...
	@if command -v golangci-lint >/dev/null 2>&1; then \
	    golangci-lint run ./...; \
	else \
	    echo "golangci-lint not installed, skipping (install: https://golangci-lint.run/welcome/install/)"; \
	fi

.PHONY: fmt
fmt: ## Format code and report diffs
	gofmt -s -w .
	@if [ -n "$$(gofmt -l .)" ]; then \
	    echo "Files were reformatted:"; gofmt -l .; \
	else \
	    echo "All files formatted"; \
	fi

.PHONY: fmt-check
fmt-check: ## Check formatting without modifying files
	@if [ -n "$$(gofmt -l .)" ]; then \
	    echo "Unformatted files:"; gofmt -l .; exit 1; \
	else \
	    echo "All files formatted"; \
	fi

## Docker ----------------------------------------------------------------------

.PHONY: docker-build
docker-build: ## Build Docker image (requires DOCKER_USER)
	docker build -t $(DOCKER_IMAGE) .
	@echo "Built $(DOCKER_IMAGE)"

.PHONY: docker-push
docker-push: docker-build ## Build and push Docker image (requires DOCKER_USER)
	docker push $(DOCKER_IMAGE)
	@echo "Pushed $(DOCKER_IMAGE)"

## Misc ------------------------------------------------------------------------

.PHONY: clean
clean: ## Remove build artifacts
	rm -f $(BINARY) $(BINARY).exe $(COVERAGE_OUT) $(COVERAGE_HTML)

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*##' $(MAKEFILE_LIST) | \
	    awk 'BEGIN {FS = ":.*## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'
