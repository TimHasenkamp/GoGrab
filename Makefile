# GoGrab dev/build/release tooling.
#
# Most targets run on Linux/macOS; on Windows use the powershell equivalents
# or run inside WSL.

GO          ?= go
NPM         ?= npm
SQLC        ?= sqlc
GOOSE       ?= goose
DB_URL      ?= $(GOGRAB_DATABASE_URL)
BINARY      ?= bin/gograb

.PHONY: help
help:
	@echo "Targets:"
	@echo "  dev            Run the SvelteKit dev server (proxies /api to :8080)"
	@echo "  run            Run the Go server (expects GOGRAB_DATABASE_URL set)"
	@echo "  build          Build frontend, then compile a single Go binary"
	@echo "  build-web      Build only the frontend"
	@echo "  build-go       Build only the Go binary (frontend must be built first)"
	@echo "  test           Run all Go tests"
	@echo "  sqlc           Regenerate sqlc code from queries.sql"
	@echo "  migrate-up     Apply all migrations (needs goose + DB_URL)"
	@echo "  migrate-down   Roll back last migration"
	@echo "  docker-build   Build the production Docker image (ARM64)"
	@echo "  clean          Remove built artifacts"

.PHONY: dev
dev:
	cd web && $(NPM) run dev

.PHONY: run
run:
	$(GO) run ./cmd/server

.PHONY: build
build: build-web build-go

.PHONY: build-web
build-web:
	cd web && $(NPM) ci --no-audit --no-fund && $(NPM) run build

.PHONY: build-go
build-go:
	$(GO) build -trimpath -ldflags="-s -w" -o $(BINARY) ./cmd/server

.PHONY: test
test:
	$(GO) test ./...

.PHONY: sqlc
sqlc:
	$(SQLC) generate

.PHONY: migrate-up
migrate-up:
	$(GOOSE) -dir migrations postgres "$(DB_URL)" up

.PHONY: migrate-down
migrate-down:
	$(GOOSE) -dir migrations postgres "$(DB_URL)" down

.PHONY: docker-build
docker-build:
	docker build --platform linux/arm64 -t gograb:latest .

.PHONY: clean
clean:
	rm -rf bin web/build web/.svelte-kit
