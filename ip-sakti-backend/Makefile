.PHONY: run build test migrate tidy

run:
	go run ./cmd/server/...

build:
	go build -o bin/server ./cmd/server/...

test:
	go test ./...

migrate:
	go run ./scripts/migrate/main.go

tidy:
	go mod tidy