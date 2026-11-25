.PHONY: dev build run test test-e2e test-cov fmt fmt-check lint migrate migrate-baseline seed docker-up docker-down

BINARY := bin/dashboard-server

dev:
	go run ./cmd/server

build:
	@mkdir -p bin
	go build -o $(BINARY) ./cmd/server

run: build
	./$(BINARY)

test:
	go test -race -v ./internal/...

test-e2e:
	go test -race -v -tags=e2e ./internal/server/...

test-cov:
	go test -race -coverprofile=coverage.out ./internal/...
	go tool cover -func=coverage.out

fmt:
	gofmt -w .

fmt-check:
	@test -z "$$(gofmt -l .)" || (echo "Files need gofmt:"; gofmt -l .; exit 1)

lint:
	go vet ./...

migrate:
	go run ./cmd/migrate

migrate-baseline:
	go run ./cmd/migrate -baseline

seed:
	go run ./cmd/seed

docker-up:
	docker compose up -d

docker-down:
	docker compose down
