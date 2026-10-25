COMMIT = $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)

.PHONY: build build-transcoder test generate fmt run run-transcoder seed

build:
	docker build \
		--file ./cmd/plural-api/Dockerfile \
		--tag=plural-api:$(COMMIT) \
		--tag=plural-api:latest .

build-transcoder:
	docker build \
		--file ./cmd/plural-api-transcoder/Dockerfile \
		--tag=plural-api-transcoder:$(COMMIT) \
		--tag=plural-api-transcoder:latest .

test:
	go test -cover -race ./...

generate:
	go generate ./...

fmt:
	go tool gofumpt -l -w .

run:
	docker compose up -d postgres seaweedfs
	go run ./cmd/plural-api

run-transcoder:
	go run ./cmd/plural-api-transcoder

seed:
	./scripts/seed.sh
