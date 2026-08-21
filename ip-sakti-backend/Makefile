.PHONY: run build test migrate tidy

run:
	go run ./cmd/server/...

build:
	go build -o bin/server ./cmd/server/...

test:
	go test ./...

migrate:
	psql $(DATABASE_URL) -f migrations/001_initial.sql

tidy:
	go mod tidy
