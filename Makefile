COMMIT = $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)

.PHONY: build test generate fmt run

build:
	docker build \
		--file ./cmd/plural-api/Dockerfile \
		--tag=plural-api:$(COMMIT) \
		--tag=plural-api:latest .

test:
	go test -cover -race ./...

generate:
	go generate ./...

fmt:
	go tool gofumpt -l -w .

run:
	docker compose up -d postgres seaweedfs
	go run ./cmd/plural-api
