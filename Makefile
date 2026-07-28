.PHONY: help install setup dev build docker-up docker-down clean

help:
	@echo "CDK Recharge System - Available Commands"
	@echo "========================================"
	@echo "  make install      - Install dependencies"
	@echo "  make setup        - Setup database and initial data"
	@echo "  make dev          - Start development servers (Docker Compose)"
	@echo "  make build        - Build frontend and backend"
	@echo "  make docker-up    - Start Docker containers"
	@echo "  make docker-down  - Stop Docker containers"
	@echo "  make clean        - Clean build artifacts"

install:
	@echo "Installing frontend dependencies..."
	cd frontend && npm install
	@echo "Backend dependencies are managed by Go modules"

setup:
	@echo "Setting up database..."
	cd backend && go run entgo.io/ent/cmd/ent generate ./ent/schema

dev:
	@echo "Starting development environment with Docker Compose..."
	docker-compose up -d
	@echo "Services started!"
	@echo "  Frontend: http://localhost:5173"
	@echo "  Backend:  http://localhost:8080"
	@echo "  PostgreSQL: localhost:5432"
	@echo "  Redis: localhost:6379"

build:
	@echo "Building frontend..."
	cd frontend && npm run build
	@echo "Frontend build complete!"
	@echo "Backend is built automatically by Go"

docker-up:
	docker-compose up -d

docker-down:
	docker-compose down

logs:
	docker-compose logs -f

clean:
	@echo "Cleaning build artifacts..."
	rm -rf frontend/dist
	cd backend && go clean
	@echo "Clean complete!"
