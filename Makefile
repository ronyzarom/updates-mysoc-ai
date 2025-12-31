.PHONY: all build build-server build-updater clean test run-server run-updater migrate dashboard

# Variables
BINARY_DIR=bin
SERVER_BINARY=$(BINARY_DIR)/update-server
UPDATER_BINARY=$(BINARY_DIR)/mysoc-updater
GO=go
GOFLAGS=-ldflags="-s -w"

# Version info
VERSION?=0.1.0
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS=-ldflags="-s -w -X main.Version=$(VERSION) -X main.GitCommit=$(GIT_COMMIT) -X main.BuildTime=$(BUILD_TIME)"

all: build

# Build both binaries
build: build-server build-updater

build-server:
	@echo "Building update-server..."
	@mkdir -p $(BINARY_DIR)
	$(GO) build $(LDFLAGS) -o $(SERVER_BINARY) ./cmd/update-server

build-updater:
	@echo "Building mysoc-updater..."
	@mkdir -p $(BINARY_DIR)
	$(GO) build $(LDFLAGS) -o $(UPDATER_BINARY) ./cmd/mysoc-updater

# Cross-compile updater for Linux
build-updater-linux:
	@echo "Building mysoc-updater for Linux..."
	@mkdir -p $(BINARY_DIR)
	GOOS=linux GOARCH=amd64 $(GO) build $(LDFLAGS) -o $(BINARY_DIR)/mysoc-updater-linux-amd64 ./cmd/mysoc-updater
	GOOS=linux GOARCH=arm64 $(GO) build $(LDFLAGS) -o $(BINARY_DIR)/mysoc-updater-linux-arm64 ./cmd/mysoc-updater

# Run the server locally
run-server:
	$(GO) run ./cmd/update-server

# Run the updater locally
run-updater:
	$(GO) run ./cmd/mysoc-updater

# Run tests
test:
	$(GO) test -v ./...

# Run tests with coverage
test-coverage:
	$(GO) test -v -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

# Clean build artifacts
clean:
	rm -rf $(BINARY_DIR)
	rm -f coverage.out coverage.html

# Database migrations
migrate-up:
	@echo "Running migrations..."
	psql -d mysoc_updates -f migrations/001_initial.up.sql

migrate-down:
	@echo "Rolling back migrations..."
	psql -d mysoc_updates -f migrations/001_initial.down.sql

# Dashboard commands
dashboard-install:
	cd dashboard && npm install

dashboard-dev:
	cd dashboard && npm run dev

dashboard-build:
	cd dashboard && npm run build

# Docker commands
docker-build:
	docker build -t updates-mysoc-ai:$(VERSION) .

docker-compose-up:
	docker-compose up -d

docker-compose-down:
	docker-compose down

# Development helpers
fmt:
	$(GO) fmt ./...

lint:
	golangci-lint run

tidy:
	$(GO) mod tidy

# Generate checksums for releases
checksums:
	@cd $(BINARY_DIR) && sha256sum * > checksums.txt

# Deployment commands
deploy:
	@echo "Deploying to updates.mysoc.ai..."
	./scripts/deploy.sh

deploy-build:
	./scripts/deploy.sh --build-only

deploy-only:
	./scripts/deploy.sh --deploy-only

deploy-restart:
	./scripts/deploy.sh --restart

deploy-status:
	./scripts/deploy.sh --status

deploy-logs:
	./scripts/deploy.sh --logs

deploy-ssh:
	./scripts/deploy.sh --ssh

# Help
help:
	@echo "Available targets:"
	@echo "  build          - Build both server and updater"
	@echo "  build-server   - Build update-server"
	@echo "  build-updater  - Build mysoc-updater"
	@echo "  run-server     - Run update-server locally"
	@echo "  run-updater    - Run mysoc-updater locally"
	@echo "  test           - Run tests"
	@echo "  clean          - Clean build artifacts"
	@echo "  migrate-up     - Run database migrations"
	@echo "  migrate-down   - Rollback database migrations"
	@echo "  dashboard-dev  - Run dashboard in development mode"
	@echo "  docker-build   - Build Docker image"
	@echo ""
	@echo "Deployment:"
	@echo "  deploy         - Full deployment to updates.mysoc.ai"
	@echo "  deploy-build   - Build binaries only"
	@echo "  deploy-restart - Restart services on server"
	@echo "  deploy-status  - Check service status"
	@echo "  deploy-logs    - Tail server logs"
	@echo "  deploy-ssh     - SSH into server"

