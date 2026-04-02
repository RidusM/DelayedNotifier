PROJECT_NAME := delayed-notifier
MAIN_PACKAGE := ./cmd/delayed-notifier
BINARY_NAME := $(PROJECT_NAME)
BINARY_PATH := ./bin/$(BINARY_NAME)

BASE_STACK := docker compose -f docker-compose.yml
INTEGRATION_STACK := docker compose --env-file "$(CURDIR)/.env" -f tests/integration/docker-compose-integration-test.yml

INTEGRATION_TEST_DIR := .tests\integration

GOBUILD := CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build
GOTEST := go test -v -race
GOCOVER := -covermode=atomic -coverprofile=coverage.txt

.DEFAULT_GOAL := help

.PHONY: help
help: ## Display this help screen
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-25s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: deps
deps: ## Tidy and verify Go modules
	go mod tidy && go mod verify

.PHONY: deps-audit
deps-audit: ## Check dependencies for vulnerabilities using govulncheck (govulncheck is must be required)
	govulncheck ./...

.PHONY: run
run: deps swag-v1 ## Run the application locally (requires dependencies like DB/Rabbit to be running)
	@echo "Running application..."
	go run -tags migrate ./cmd/delayed-notifier -config=./configs/dev.env

.PHONY: infra-up
infra-up: ## Start infrastructure only (db, redis, rabbitmq) for local development
	$(BASE_STACK) up -d db redis rabbitmq
	@echo "Infrastructure started. Wait for healthchecks:"
	$(BASE_STACK) logs -f db redis rabbitmq

.PHONY: infra-down
infra-down: ## Down infrastructure only (db, redis, rabbitmq) for local development
	@echo "Stopping infrastructure..."
	$(BASE_STACK) down db redis rabbitmq
	@echo "Infrastructure stopped"

.PHONY: infra-logs
infra-logs: ## Show logs infrastructure only (db, redis, rabbitmq) for local cevelopment
	@$(BASE_STACK) logs -f db redis rabbitmq

.PHONY: compose-up
compose-up: ## Run all services (infrastructure + app)
	@echo "Starting all services..."
	$(BASE_STACK) up --build -d
	@echo "All services started"
	$(BASE_STACK) logs -f app

.PHONY: compose-down
compose-down: ## Stop and remove all containers, networks, and volumes (from all stacks)
	@echo "Stopping and cleaning"
	$(BASE_STACK) down --remove-orphans --volumes
	@echo "Cleanup completed"

.PHONY: migrate-up
migrate-up: ## Applied migrates to database
	@echo "Running migrations..."
	$(BASE_STACK) run --rm db-migrator
	@echo "Migrations applied"

.PHONY: migrate-down
migrate-down: ## Rolling back migrations
	@echo "Rolling back last migration..."
	@$(BASE_STACK) run --rm db-migrator -path /migrations -database "$${DB_DSN}" down 1
	@echo "Migration rolled back"

.PHONY: test
test: ## Run unit tests with race detector and coverage
	@echo "Running unit tests..."
	$(GOTEST) $(GOCOVER) ./internal/...
	@echo "Unit tests completed"
	go tool cover -func=coverage.txt | tail -1

.PHONY: test-verbose
test-verbose: ## Run verbose tests
	@echo "Running verbose tests..."
	@$(GOTEST) -v -race -cover ./internal/...

.PHONY: integration-test
integration-test: ## Run integration tests (requires Docker + Git Bash)
	@echo "Running integration tests..."
	@$(INTEGRATION_STACK) --env-file "$(CURDIR)/.env" up -d db redis rabbitmq
	@echo "Waiting for database..."
	@for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do \
		$(INTEGRATION_STACK) exec db pg_isready -U postgres -q 2>/dev/null && break || (echo "  ⏳ Waiting..." && sleep 2); \
	done
	@$(INTEGRATION_STACK) exec db pg_isready -U postgres -q 2>/dev/null || (echo "✗ Database not ready" && exit 1)
	@echo "Database ready"
	@echo "Applying migrations..."
	@$(INTEGRATION_STACK) run --rm db-migrator
	@echo "Running tests..."
	@$(INTEGRATION_STACK) up --abort-on-container-exit --exit-code-from integration-test integration-test; \
	TEST_EXIT_CODE=$$?; \
	$(INTEGRATION_STACK) down --remove-orphans --volumes; \
	exit $$TEST_EXIT_CODE
	@echo "Integration tests completed"

.PHONY: test-all
test-all: test integration-test ## Running all tests (unit + integration)
	@echo "All tests completed"

.PHONY: format
format: ## Code fromatting (gofumpt, gci, golines, goimports)
	@echo "Formatting code..."
	gofumpt -l -w .
	gci write . --skip-generated -s standard -s default
	goimports -w .
	golines -w --max-len=120 .
	@echo "Code formatted"

.PHONY: lint
lint: ## Running golagci_lint
	@echo "Running linter..."
	golangci-lint run
	@echo "Lint passed"

.PHONY: lint-fix
lint-fix: ## Running golangci-lint with fixes
	@echo "Running linter with fixes..."
	golangci-lint run
	@echo "Lint completed"

.PHONY: lint-hadolint
lint-hadolint: ## Run hadolint on Dockerfiles (requires hadolint installed)
	hadolint Dockerfile

.PHONY: lint-dotenv
lint-dotenv: ## Run dotenv-linter on .env files (requires dotenv-linter installed)
	dotenv-linter check -r .

.PHONY: lint-dotenv-fix
lint-dotenv-fix: ## Fix .env files (requires dotenv-linter installed)
	dotenv-linter fix --no-backup -r .

.PHONY: swagger
swagger: ## Generate Swagger docs
	@echo "Generating Swagger docs..."
	@swag init -g internal/transport/http/routes.go --output docs
	@echo "Swagger docs generated in docs/"

.PHONY: mocks
mocks: ## Generate mocks
	@echo "Generating mocks..."
	@mockgen -package=mock_repository -destination=internal/repository/mock/telegram_repository_mock.go \
		delayednotifier/internal/service TelegramRepository
	@mockgen -package=mock_repository -destination=internal/repository/mock/notify_repository_mock.go \
		delayednotifier/internal/service NotifyRepository
	@mockgen -package=mock_repository -destination=internal/repository/mock/cache_repository_mock.go \
		delayednotifier/internal/service CacheRepository
	@mockgen -package=mock_sender -destination=internal/transport/sender/mock/sender_mock.go \
		delayednotifier/internal/service NotificationSender
	@echo "Mocks generated"

.PHONY: pre-commit
pre-commit: format lint lint-hadolint lint-dotenv swagger mocks test ## Run all checks before commit
	@echo "Pre-commit checks passed!"

.PHONY: build
build: deps ## Build bin for linux/amd64
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BINARY_PATH)
	@$(GOBUILD) -o $(BINARY_PATH) $(MAIN_PACKAGE)
	@echo "Binary built: $(BINARY_PATH)"

.PHONY: build-local
build-local: deps ## build for local os
	@echo "Building for local OS..."
	go build -o ./bin/$(BINARY_NAME) $(MAIN_PACKAGE)
	@echo "Binary built: ./bin/$(BINARY_NAME)"

.PHONY: build-docker
build-docker: ## Build docker image
	@echo "Building Docker image..."
	@docker build -t $(PROJECT_NAME):latest .
	@echo "Image built: $(PROJECT_NAME):latest"

.PHONY: build-docker-multiarch
build-docker-multiarch: ## Build multi-arch image for linux/amd64 | linux/arm64
	@echo "Building multi-arch image..."
	@docker buildx build --platform linux/amd64,linux/arm64 \
		-t $(PROJECT_NAME):latest --push .
	@echo "Multi-arch image built and pushed"

.PHONY: clean
clean: ## clean mock files and artifacts
	@echo "Cleaning up..."
	@rm -rf ./bin/ ./docs/ coverage*.txt
	@find . -type f -name '*_mock.go' -path '*/mock/*' -delete 2>/dev/null || true
	@echo "Cleanup completed"

.PHONY: clean-cache
clean-cache: ## Clean test and linter cache
	@echo "Cleaning caches..."
	go clean -testcache
	@$(GOLANGCI_LINT) cache clean 2>/dev/null || true
	@echo "Caches cleaned"

.PHONY: docker-prune
docker-prune: ## Pruning docker
	@echo "Pruning Docker..."
	@docker system prune -af
	@echo "Docker pruned"

.PHONY: docker-logs
docker-logs: ## Show logs all docker containers
	@$(BASE_STACK) logs -f

.PHONY: install-tools
install-tools: ## Download develop tools
	@echo "Installing development tools..."
	go install golang.org/x/vuln/cmd/govulncheck@latest
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	go install github.com/daixiang0/gci@latest
	go install mvdan.cc/gofumpt@latest
	go install github.com/segmentio/golines@latest
	go install golang.org/x/tools/cmd/goimports@latest
	go install github.com/swaggo/swag/cmd/swag@latest
	go install github.com/golang-migrate/migrate/v4/cmd/migrate@latest
	go install go.uber.org/mock/mockgen@latest
	@echo "Tools installed"

.PHONY: check-requirements
check-requirements: ## Check all requirments
	@echo "Checking requirements..."
	@command -v go >/dev/null 2>&1 || { echo "Go not found"; exit 1; }
	@command -v docker >/dev/null 2>&1 || { echo "Docker not found"; exit 1; }
	@command -v docker compose >/dev/null 2>&1 || { echo "Docker Compose not found"; exit 1; }
	@echo "All requirements met"