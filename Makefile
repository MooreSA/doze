.PHONY: help build up down logs test clean dev dev-watch fmt lint tidy

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

build: ## Build Docker images
	docker-compose build

up: ## Start services (production mode)
	docker-compose up -d

up-dev: ## Start services (dev mode with hot reload)
	docker-compose -f docker-compose.yml -f docker-compose.dev.yml up

down: ## Stop services
	docker-compose down

logs: ## Show logs
	docker-compose logs -f

logs-api: ## Show API logs only
	docker-compose logs -f api

restart: ## Restart services
	docker-compose restart

rebuild: ## Rebuild and restart
	docker-compose down
	docker-compose build --no-cache
	docker-compose up -d

test: ## Run tests (when we have them)
	cd api && go test ./...

clean: ## Clean up containers and volumes
	docker-compose down -v
	rm -f api/doze

dev: ## Run API in dev mode (outside Docker)
	cd api && go run .

dev-watch: ## Run API with auto-reload (requires air)
	cd api && air

fmt: ## Format Go code
	cd api && go fmt ./...

lint: ## Run linter (requires golangci-lint)
	cd api && golangci-lint run

tidy: ## Tidy Go modules
	cd api && go mod tidy

# Fly.io deployment (coming later)
deploy: ## Deploy to Fly.io
	fly deploy

# Helper to check if .env exists
check-env:
	@test -f .env || (echo "⚠️  .env not found. Copy .env.example to .env and fill in values." && exit 1)

.DEFAULT_GOAL := help
