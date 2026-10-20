# Polaris Makefile

.PHONY: all build run dev test test-race fmt vet lint deps migrate-up migrate-down docker-up docker-down tidy

APP_NAME := polaris
BIN_DIR  := ./bin
MAIN_PKG := ./cmd/polaris

all: build

deps:
	go mod download
	go mod tidy

build:
	@mkdir -p $(BIN_DIR)
	go build -ldflags "-s -w" -o $(BIN_DIR)/$(APP_NAME) $(MAIN_PKG)

dev:
	go run $(MAIN_PKG)

run: build
	$(BIN_DIR)/$(APP_NAME)

test:
	go test ./...

test-race:
	go test -race ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

tidy:
	go mod tidy

# 数据库迁移（启动时按 database.auto_migrate 自动执行；以下为手动执行方式）
migrate-up: build
	$(BIN_DIR)/$(APP_NAME) -migrate

migrate-down: build
	$(BIN_DIR)/$(APP_NAME) -migrate-down=1

docker-up:
	docker compose up -d

docker-down:
	docker compose down
