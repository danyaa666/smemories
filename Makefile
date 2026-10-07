# Load .env (dev-only values; see .env.example) so migrate/run/test-integration see SMEM_*.
ifneq (,$(wildcard .env))
include .env
export
endif

.PHONY: build test lint run up down migrate migrate-down test-integration

build:
	go build -o bin/smemories-api ./cmd/smemories-api
	go build -o bin/smemories-migrate ./cmd/smemories-migrate

test:
	go test -race ./...

# golangci-lint, eslint and tsc join this target in later tasks (T-004, T-003).
lint:
	@out="$$(gofmt -l .)"; if [ -n "$$out" ]; then echo "gofmt needed on:"; echo "$$out"; exit 1; fi
	go vet ./...
	go vet -tags integration ./...

run:
	go run ./cmd/smemories-api

# Starts MySQL 8.4 and MinIO and returns once both are healthy and the bucket exists.
up:
	@test -f .env || { cp .env.example .env; echo "created .env from .env.example"; }
	docker compose up -d --wait mysql minio
	docker compose run --rm -T minio-init

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
