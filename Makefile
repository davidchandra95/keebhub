GO ?= go
PNPM ?= pnpm
SQLC ?= sqlc
GOLANGCI_LINT ?= golangci-lint
SQLC_VERSION := 1.31.1
GOLANGCI_LINT_VERSION := 2.12.2

.PHONY: help tools-install fmt vet lint test test-race test-integration mod-verify sqlc \
	docs-check web-install web-dev web-check web-build server migrate-up migrate-status \
	build docker-build docker-up docker-down docker-logs ci

help:
	@echo "KeebHub development targets"
	@echo "  make web-dev       Start the Vite development server"
	@echo "  make server        Start the Go server"
	@echo "  make docker-up     Build and start the full local stack"
	@echo "  make ci            Run local documentation, backend, and web gates"

tools-install:
	$(GO) install github.com/sqlc-dev/sqlc/cmd/sqlc@v$(SQLC_VERSION)
	$(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v$(GOLANGCI_LINT_VERSION)

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

lint:
	@$(GOLANGCI_LINT) version | grep -F "version $(GOLANGCI_LINT_VERSION) " >/dev/null || (echo "golangci-lint $(GOLANGCI_LINT_VERSION) is required; run make tools-install"; exit 1)
	$(GOLANGCI_LINT) run

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

test-integration:
	@if [ -z "$$TEST_DATABASE_URL" ]; then echo "TEST_DATABASE_URL is required"; exit 1; fi
	$(GO) test -race ./...

mod-verify:
	$(GO) mod verify

sqlc:
	@test "$$($(SQLC) version)" = "v$(SQLC_VERSION)" || (echo "sqlc $(SQLC_VERSION) is required; run make tools-install"; exit 1)
	$(SQLC) generate

docs-check:
	cd web && $(PNPM) docs:check

web-install:
	cd web && $(PNPM) install --frozen-lockfile

web-dev:
	cd web && $(PNPM) dev

web-check:
	cd web && $(PNPM) typecheck
	cd web && $(PNPM) lint
	cd web && $(PNPM) test
	cd web && $(PNPM) build

web-build:
	cd web && $(PNPM) build

server:
	$(GO) run ./cmd/server

migrate-up:
	$(GO) run ./cmd/migrate up

migrate-status:
	$(GO) run ./cmd/migrate status

build: web-build
	mkdir -p bin
	$(GO) build -trimpath -o bin/server ./cmd/server
	$(GO) build -trimpath -o bin/migrate ./cmd/migrate

docker-build:
	docker build -t keebhub:local .

docker-up:
	docker compose up --build -d --wait

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f app

ci: docs-check fmt vet lint test-race mod-verify web-check
