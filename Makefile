.PHONY: build test lint run

build:
	go build -o bin/smemories-api ./cmd/smemories-api

test:
	go test -race ./...

# golangci-lint, eslint and tsc join this target in later tasks (T-004, T-003).
lint:
	@out="$$(gofmt -l .)"; if [ -n "$$out" ]; then echo "gofmt needed on:"; echo "$$out"; exit 1; fi
	go vet ./...

run:
	go run ./cmd/smemories-api
