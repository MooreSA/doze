.PHONY: help build up down logs test clean

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

build: ## Build Docker images
	docker-compose build

up: ## Start services
	docker-compose up -d

down: ## Stop services
	docker-compose down

logs: ## Show logs
	docker-compose logs -f

logs-api: ## Show API logs only
	docker-compose logs -f api

restart: ## Restart services
	docker-compose restart

test: ## Run tests (when we have them)
	cd api && go test ./...

clean: ## Clean up containers and volumes
	docker-compose down -v
	rm -f api/doze

dev: ## Run API in dev mode (outside Docker)
	cd api && go run main.go

# Fly.io deployment (coming later)
deploy: ## Deploy to Fly.io
	fly deploy

# Helper to check if .env exists
check-env:
	@test -f .env || (echo "⚠️  .env not found. Copy .env.example to .env and fill in values." && exit 1)

.DEFAULT_GOAL := help
