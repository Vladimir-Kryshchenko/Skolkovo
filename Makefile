# Задачи проекта «База Сколково».

BIN := bin/skolkovo

.PHONY: build test vet fmt up down scrape index sync mcp admin serve embed

build:
	go build -o $(BIN) ./cmd/skolkovo

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w ./src ./cmd

up:
	docker compose -f deploy/docker-compose.yml up -d

down:
	docker compose -f deploy/docker-compose.yml down

scrape: build
	$(BIN) scrape

index: build
	$(BIN) index

sync: build
	$(BIN) sync

mcp: build
	$(BIN) mcp

admin: build
	$(BIN) admin

serve: build
	$(BIN) serve

embed: build
	$(BIN) embed
