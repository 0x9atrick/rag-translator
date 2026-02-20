.PHONY: build run-ingest run-translate run-seed clean sqlc tidy help lint fmt migrate-up migrate-down migrate-create

# ────────────────────────────────────────────────────────
# Variables
# ────────────────────────────────────────────────────────
BINARY     := rag-translator
CMD_DIR    := ./cmd
BUILD_DIR  := ./bin
MIGRATIONS := db/migrations/

# ────────────────────────────────────────────────────────
# Build
# ────────────────────────────────────────────────────────
build: ## Build the binary
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY) $(CMD_DIR)/main.go

# ────────────────────────────────────────────────────────
# Run commands
# ────────────────────────────────────────────────────────
run-ingest: ## Run ingest command (usage: make run-ingest DIR=./game-files)
	go run $(CMD_DIR)/main.go ingest $(DIR)

run-translate: ## Run translate command (usage: make run-translate IN=./game-files OUT=./output)
	go run $(CMD_DIR)/main.go translate $(IN) $(OUT)

run-seed: ## Run seed ingestion (usage: make run-seed BASE=abc123 TARGET=def456 FOLDER=scripts/)
	go run $(CMD_DIR)/main.go ingest-seed-git $(BASE) $(TARGET) $(FOLDER)

# ────────────────────────────────────────────────────────
# Database migrations (golang-migrate)
# ────────────────────────────────────────────────────────
migrate-up: ## Apply all pending migrations (usage: make migrate-up DATABASE_URL=postgres://...)
	migrate -path $(MIGRATIONS) -database "$$DATABASE_URL" up

migrate-down: ## Roll back the last migration (usage: make migrate-down DATABASE_URL=postgres://...)
	migrate -path $(MIGRATIONS) -database "$$DATABASE_URL" down 1

migrate-create: ## Create a new migration (usage: make migrate-create NAME=add_users)
	migrate create -ext sql -dir $(MIGRATIONS) -seq $(NAME)

# ────────────────────────────────────────────────────────
# Code generation
# ────────────────────────────────────────────────────────
sqlc: ## Generate sqlc type-safe query code
	sqlc generate

# ────────────────────────────────────────────────────────
# Dependencies
# ────────────────────────────────────────────────────────
tidy: ## Tidy go modules
	go mod tidy

# ────────────────────────────────────────────────────────
# Utilities
# ────────────────────────────────────────────────────────
clean: ## Remove build artifacts
	rm -rf $(BUILD_DIR)

lint: ## Run go vet
	go vet ./...

fmt: ## Format code
	gofmt -w .

# ────────────────────────────────────────────────────────
# Help
# ────────────────────────────────────────────────────────
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
