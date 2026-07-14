.PHONY: all proto backend frontend infra up down dev-db dev-full lint test clean docker-build deploy

# ─── Project Metadata ───────────────────────────────────────
APP_NAME        := observeid
BACKEND_DIR     := backend
FRONTEND_DIR    := frontend
INFRA_DIR       := infrastructure
DEPLOY_DIR      := deploy

# ─── Colors ─────────────────────────────────────────────────
RED     := \033[0;31m
GREEN   := \033[0;32m
YELLOW  := \033[1;33m
CYAN    := \033[0;36m
NC      := \033[0m

# ─── Targets ────────────────────────────────────────────────

all: proto backend frontend

# ─── Protobuf ──────────────────────────────────────────────
proto:
	@echo "$(CYAN)▶ Generating protobuf code...$(NC)"
	@mkdir -p $(BACKEND_DIR)/pkg/proto
	protoc \
		--proto_path=proto \
		--go_out=$(BACKEND_DIR)/pkg/proto \
		--go_opt=paths=source_relative \
		--go-grpc_out=$(BACKEND_DIR)/pkg/proto \
		--go-grpc_opt=paths=source_relative \
		proto/event/v1/*.proto \
		proto/model/v1/*.proto
	@echo "$(GREEN)✓ Protobuf generation complete$(NC)"

proto-clean:
	@rm -rf $(BACKEND_DIR)/pkg/proto

# ─── Backend ───────────────────────────────────────────────
backend: proto
	@echo "$(CYAN)▶ Building backend services...$(NC)"
	cd $(BACKEND_DIR) && CGO_ENABLED=0 go build -o ../build/ ./...
	@echo "$(GREEN)✓ Backend build complete$(NC)"

backend-test:
	@echo "$(CYAN)▶ Running backend tests...$(NC)"
	cd $(BACKEND_DIR) && go test -v -race -count=1 ./...
	@echo "$(GREEN)✓ Backend tests complete$(NC)"

backend-lint:
	@echo "$(CYAN)▶ Linting backend code...$(NC)"
	cd $(BACKEND_DIR) && \
		golangci-lint run ./... --timeout=5m && \
		go vet ./...
	@echo "$(GREEN)✓ Backend linting complete$(NC)"

# ─── Frontend ──────────────────────────────────────────────
frontend:
	@echo "$(CYAN)▶ Building frontend...$(NC)"
	cd $(FRONTEND_DIR) && npm install && npm run build
	@echo "$(GREEN)✓ Frontend build complete$(NC)"

frontend-dev:
	@echo "$(CYAN)▶ Starting frontend dev server...$(NC)"
	cd $(FRONTEND_DIR) && npm run dev

frontend-lint:
	@echo "$(CYAN)▶ Linting frontend...$(NC)"
	cd $(FRONTEND_DIR) && npm run lint

# ─── Infrastructure ────────────────────────────────────────
up:
	@echo "$(CYAN)▶ Starting infrastructure...$(NC)"
	docker compose -f $(INFRA_DIR)/docker-compose.yml up -d
	@echo "$(GREEN)✓ Infrastructure started$(NC)"

down:
	@echo "$(CYAN)▶ Stopping infrastructure...$(NC)"
	docker compose -f $(INFRA_DIR)/docker-compose.yml down
	@echo "$(GREEN)✓ Infrastructure stopped$(NC)"

logs:
	docker compose -f $(INFRA_DIR)/docker-compose.yml logs -f

# ─── Development ───────────────────────────────────────────
dev-db: up
	@echo "$(CYAN)▶ Waiting for databases to be ready...$(NC)"
	@sleep 10
	@echo "$(CYAN)▶ Running database migrations...$(NC)"
	@cat $(INFRA_DIR)/postgres/init.sql | docker exec -i observeid-postgres psql -U observeid -d observeid
	@docker exec -i observeid-neo4j cypher-shell -u neo4j -p $${NEO4J_PASSWORD:-observeid123} -f /init.cypher
	@echo "$(GREEN)✓ Database migrations complete$(NC)"

dev-full: dev-db
	@echo "$(CYAN)▶ Starting all services in dev mode...$(NC)"
	@echo "$(GREEN)✓ Dev environment ready!$(NC)"
	@echo ""
	@echo "  $(YELLOW)PostgreSQL:$(NC)  localhost:5432"
	@echo "  $(YELLOW)Neo4j:$(NC)       localhost:7687"
	@echo "  $(YELLOW)Kafka:$(NC)       localhost:9092"
	@echo "  $(YELLOW)Temporal:$(NC)    localhost:7233"
	@echo "  $(YELLOW)Redis:$(NC)       localhost:6379"
	@echo "  $(YELLOW)Qdrant:$(NC)      localhost:6333"
	@echo "  $(YELLOW)Grafana:$(NC)     http://localhost:3000"
	@echo "  $(YELLOW)Frontend:$(NC)    http://localhost:3001"
	@echo ""

# ─── Docker ────────────────────────────────────────────────
docker-build:
	@echo "$(CYAN)▶ Building Docker images...$(NC)"
	docker build -t $(APP_NAME)/identity-service -f docker/identity-service.Dockerfile .
	docker build -t $(APP_NAME)/frontend -f docker/frontend.Dockerfile .
	@echo "$(GREEN)✓ Docker images built$(NC)"

# ─── Kubernetes ─────────────────────────────────────────────
deploy:
	@echo "$(CYAN)▶ Deploying to Kubernetes...$(NC)"
	kubectl apply -f $(DEPLOY_DIR)/k8s/namespace.yaml
	kustomize build $(DEPLOY_DIR)/k8s/overlays/prod | kubectl apply -f -
	@echo "$(GREEN)✓ Deployment complete$(NC)"

deploy-dev:
	kubectl apply -f $(DEPLOY_DIR)/k8s/namespace.yaml
	kustomize build $(DEPLOY_DIR)/k8s/overlays/dev | kubectl apply -f -

# ─── Quality ────────────────────────────────────────────────
lint: backend-lint frontend-lint
	@echo "$(GREEN)✓ All linting complete$(NC)"

test: backend-test
	@echo "$(GREEN)✓ All tests complete$(NC)"

check: lint test
	@echo "$(GREEN)✓ All checks passed$(NC)"

# ─── Clean ──────────────────────────────────────────────────
clean: down
	@echo "$(CYAN)▶ Cleaning build artifacts...$(NC)"
	@rm -rf build/
	@rm -rf $(BACKEND_DIR)/pkg/proto
	@cd $(BACKEND_DIR) && go clean -cache
	@echo "$(GREEN)✓ Clean complete$(NC)"

# ─── Help ──────────────────────────────────────────────────
help:
	@echo "$(CYAN)ObserveID Reimagined Build System$(NC)"
	@echo ""
	@echo "  $(YELLOW)make all$(NC)        - Build everything (proto + backend + frontend)"
	@echo "  $(YELLOW)make proto$(NC)      - Generate protobuf code"
	@echo "  $(YELLOW)make backend$(NC)    - Build Go backend"
	@echo "  $(YELLOW)make frontend$(NC)   - Build Next.js frontend"
	@echo "  $(YELLOW)make up$(NC)         - Start infrastructure (Docker Compose)"
	@echo "  $(YELLOW)make down$(NC)       - Stop infrastructure"
	@echo "  $(YELLOW)make dev-full$(NC)   - Start everything + run migrations"
	@echo "  $(YELLOW)make test$(NC)       - Run tests"
	@echo "  $(YELLOW)make lint$(NC)       - Lint all code"
	@echo "  $(YELLOW)make deploy$(NC)     - Deploy to Kubernetes"
	@echo "  $(YELLOW)make clean$(NC)      - Clean build artifacts"

# ─── Default ────────────────────────────────────────────────
.DEFAULT_GOAL := help
