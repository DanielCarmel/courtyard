BINARY    := courtyard
IMAGE     := courtyard
GOFLAGS   := -trimpath -ldflags="-s -w"
GOPATH    := $(shell go env GOPATH)

.PHONY: all build run test vet lint clean docker-build docker-run docker-up docker-down help

all: build ## Build the binary (default)

## ── Local development ─────────────────────────────────────────────────────────

build: ## Build the binary
	go build $(GOFLAGS) -o $(BINARY) ./cmd/courtyard

run: build ## Build and run locally (requires .env to be sourced)
	@if [ -f .env ]; then \
		set -a && . ./.env && set +a && ./$(BINARY); \
	else \
		echo "No .env file found — copy .env.example to .env and fill in credentials"; \
		exit 1; \
	fi

test: ## Run all tests
	go test ./...

test-v: ## Run all tests with verbose output
	go test -v ./...

vet: ## Run go vet
	go vet ./...

lint: vet ## Run vet + staticcheck (install: go install honnef.co/go/tools/cmd/staticcheck@latest)
	@if command -v staticcheck >/dev/null 2>&1; then \
		staticcheck ./...; \
	else \
		echo "staticcheck not installed — run: go install honnef.co/go/tools/cmd/staticcheck@latest"; \
	fi

tidy: ## Tidy go.mod and go.sum
	go mod tidy

clean: ## Remove built binary
	rm -f $(BINARY)

## ── Docker ────────────────────────────────────────────────────────────────────

docker-build: ## Build the Docker image
	docker build -t $(IMAGE) .

docker-run: docker-build ## Build and run the container (reads .env)
	@if [ ! -f .env ]; then \
		echo "No .env file found — copy .env.example to .env and fill in credentials"; \
		exit 1; \
	fi
	docker run --rm -p 8080:8080 --env-file .env $(IMAGE)

docker-up: ## Start with docker compose (reads .env)
	docker compose up --build

docker-down: ## Stop docker compose services
	docker compose down

docker-logs: ## Tail logs from running compose service
	docker compose logs -f

## ── Helpers ───────────────────────────────────────────────────────────────────

healthz: ## Check /healthz on localhost:8080
	curl -sf http://localhost:8080/healthz | jq .

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*##' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'
