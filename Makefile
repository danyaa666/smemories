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

.PHONY: image image-smoke build test lint run web-install web-build web-test web-lint print-check up down migrate migrate-down test-integration reset-limits

build:
	go build -o bin/smemories-api ./cmd/smemories-api
	go build -o bin/smemories-migrate ./cmd/smemories-migrate
	go build -o bin/smemories-media-backfill ./cmd/smemories-media-backfill
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

# Headless print check of the HTML book renderer (T-058, ADR 0003): prints a fixture book to PDF with Chromium and inspects the PDF.
# Needs poppler (pdfinfo, pdffonts, pdfimages, pdftotext) on the PATH; not part of `make test`.
print-check: web/node_modules
	cd tools/print-check && npm ci && npx playwright install chromium && npm test

# End-to-end test of the M1 journey (T-020, docs/e2e.md): Playwright against a locally started API and the built web app, on the
# stack from `make up`. Needs poppler like print-check. The API runs with SMEM_ENV=test, so only the Redis keys smem:test:* are reset.
.PHONY: e2e
e2e: web/node_modules
	cd tools/e2e && npm ci && npx playwright install chromium
	@$(MAKE) --no-print-directory reset-limits SMEM_ENV=test
	scripts/e2e.sh

# Starts MySQL 8.4, Valkey and MinIO and returns once all are healthy and the bucket exists.
up:
	@test -f .env || { cp .env.example .env; echo "created .env from .env.example"; }
	docker compose up -d --wait mysql redis minio
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
# -timeout 30m (T-077): Go's default is 10m per package, and internal/media took 763-858 s on a loaded machine before its tests ran in parallel;
# a slow machine must fail on a real hang, not on the default.
test-integration:
	go test -race -count=1 -timeout 30m -tags integration ./...

# Marks the Newman test accounts A and B of postman/notes.postman_collection.json as email-verified (the dev mail only goes to the API's stdout).
.PHONY: verify-newman-users
verify-newman-users:
	docker compose exec -T mysql sh -c 'mysql -u"$$MYSQL_USER" -p"$$MYSQL_PASSWORD" "$$MYSQL_DATABASE" -e "UPDATE user_tab SET email_verified_at = UNIX_TIMESTAMP(NOW(3)) * 1000 WHERE email IN (\"newman-notes-a@example.com\",\"newman-notes-b@example.com\") AND email_verified_at IS NULL"'

# Dev only: forgets every rate-limiter window (keys smem:<SMEM_ENV>:rl:*) in the compose Redis, so Newman can be re-run from one address.
reset-limits:
	docker compose exec -T redis sh -c "valkey-cli --scan --pattern 'smem:$(or $(SMEM_ENV),dev):rl:*' | xargs -r -n 100 valkey-cli DEL"

# Production image (T-022, docs/container.md). Needs only Docker. IMAGE names the tag; VERSION and COMMIT end up in the OCI labels and in the log line "starting".
IMAGE ?= smemories:local
image:
	docker build --provenance=false --sbom=false --build-arg VERSION=$$(git describe --tags --always --dirty) --build-arg COMMIT=$$(git rev-parse HEAD) -t $(IMAGE) .

# Starts its own private MySQL, Valkey and MinIO (never the `make up` stack) and checks the image; see the header of the script.
image-smoke: image
	scripts/image-smoke.sh $(IMAGE)
