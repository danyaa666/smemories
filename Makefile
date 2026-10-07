# Load .env (dev-only values; see .env.example) so migrate/run/test-integration see SMEM_*.
ifneq (,$(wildcard .env))
include .env
export
endif

# One compose project per checkout, so two checkouts never share containers or volumes. An already
# set COMPOSE_PROJECT_NAME (shell or .env) wins; the env var overrides `name:` in docker-compose.yml.
# A checkout whose stack already runs under the old fixed name `smemories` keeps that name.
ifeq (,$(COMPOSE_PROJECT_NAME))
COMPOSE_PROJECT_NAME := $(shell if docker ps -a --filter label=com.docker.compose.project=smemories --format '{{.Label "com.docker.compose.project.working_dir"}}' 2>/dev/null | grep -qxF '$(CURDIR)'; then echo smemories; else basename '$(CURDIR)' | tr 'A-Z' 'a-z' | sed 's/[^a-z0-9_-]/-/g; s/^[^a-z0-9]*//'; fi)
endif
export COMPOSE_PROJECT_NAME

.PHONY: build test lint run spike web-install web-build web-test web-lint up down migrate migrate-down test-integration

build:
	go build -o bin/smemories-api ./cmd/smemories-api
	go build -o bin/smemories-migrate ./cmd/smemories-migrate
	$(MAKE) web-build

test:
	go test -race ./...
	$(MAKE) web-test

# gofmt checks tracked and new (not ignored) Go files, so .team/worktrees/* is never scanned.
# golangci-lint v2 (see .golangci.yml; CI pins the version) and the web checks (eslint, tsc, i18n, API types).
lint:
	@out="$$(git ls-files -co --exclude-standard '*.go' | xargs gofmt -l)"; if [ -n "$$out" ]; then echo "gofmt needed on:"; echo "$$out"; exit 1; fi
	go vet ./...
	go vet -tags integration ./...
	golangci-lint run
	$(MAKE) web-lint

run:
	go run ./cmd/smemories-api

# Web (Vite + React, ./web). node_modules is installed on demand from package-lock.json.
web-install:
	cd web && npm ci

web/node_modules: web/package-lock.json
	cd web && npm ci
	@touch web/node_modules

web-build: web/node_modules
	cd web && npm run build

web-test: web/node_modules
	cd web && npm test

web-lint: web/node_modules
	cd web && npm run lint && npm run typecheck && npm run lint:i18n && npm run check:api

# Starts MySQL 8.4 and MinIO and returns once both are healthy and the bucket exists.
up:
	@test -f .env || { cp .env.example .env; echo "created .env from .env.example"; }
	docker compose up -d --wait mysql minio
	docker compose run --rm -T minio-init
	@# The init script only runs on a fresh volume; GRANT is idempotent, so re-run it for older volumes.
	docker compose exec -T mysql sh /docker-entrypoint-initdb.d/10-test-grants.sh

# Stops the stack; data volumes are kept (docker compose down -v wipes them).
down:
	docker compose down

migrate:
	go run ./cmd/smemories-migrate up

migrate-down:
	go run ./cmd/smemories-migrate down

# Needs `make up` first.
test-integration:
	go test -race -count=1 -tags integration ./...

# T-005 throwaway PDF-engine spike (see internal/pdf/spike/README.md). Writes PDFs to
# internal/pdf/spike/out/ and screenshots to docs/adr/0002-assets/.
SPIKE = go test -tags spike -count=1 -v ./internal/pdf/spike -run
PDFCPU = github.com/pdfcpu/pdfcpu/cmd/pdfcpu@v0.16.1

spike:
	$(SPIKE) 'TestFixtures|TestSplitRuns'
	$(SPIKE) 'TestFpdf_|TestGopdf_|TestQA_'
	@echo "--- C5 benchmark (one process per library, machine should be idle)"
	@$(SPIKE) TestBenchFpdf | grep -E 'BENCH|FAIL'
	@$(SPIKE) TestBenchGopdf | grep -E 'BENCH|FAIL'
	@echo "--- C7 pdfcpu strict validation"
	@go install $(PDFCPU)
	@for f in internal/pdf/spike/out/*.pdf; do "$$(go env GOPATH)/bin/pdfcpu" validate --mode strict "$$f" || exit 1; done
