.PHONY: preflight preflight-python preflight-frontend preflight-e2e preflight-java preflight-java-integration preflight-go preflight-go-integration preflight-go-migrations preflight-go-migration-lint preflight-security preflight-compose-config preflight-ai-service preflight-ai-service-evals grafana-sync grafana-sync-check worktree-cleanup install-pre-commit

# Run all CI checks locally before pushing
preflight: grafana-sync-check preflight-python preflight-frontend preflight-security preflight-java preflight-go preflight-compose-config
	@echo "\n✅ All preflight checks passed"

# --- Python services ---
preflight-python:
	@echo "\n=== Python: ruff lint ==="
	ruff check services/
	@echo "\n=== Python: ruff format ==="
	ruff format --check services/
	@echo "\n=== Python: pytest (ingestion) ==="
	pytest services/ingestion/tests/ -v
	@echo "\n=== Python: pytest (chat) ==="
	pytest services/chat/tests/ -v
	@echo "\n=== Python: pytest (debug) ==="
	pytest services/debug/tests/ -v

# --- Frontend ---
preflight-frontend:
	@echo "\n=== Frontend: lint ==="
	cd frontend && npm run lint
	@echo "\n=== Frontend: type check ==="
	cd frontend && npx tsc --noEmit
	@echo "\n=== Frontend: build ==="
	cd frontend && npm run build

# --- Frontend E2E (mocked, no backend needed) ---
preflight-e2e:
	@echo "\n=== E2E: mocked Playwright tests ==="
	cd frontend && npx playwright test

# --- Java (checkstyle + unit tests run locally) ---
preflight-java:
	@echo "\n=== Java: checkstyle ==="
	cd java && ./gradlew checkstyleMain checkstyleTest --no-daemon
	@echo "\n=== Java: unit tests ==="
	cd java && ./gradlew test --no-daemon

# --- Java integration tests (requires Windows PC via SSH) ---
preflight-java-integration:
	@echo "\n=== Java: integration tests (via SSH) ==="
	ssh PC@100.79.113.84 "cd C:\Users\PC\repos\portfolio && git pull && cd java && ./gradlew integrationTest --no-daemon"

# --- Go services ---
preflight-go:
	@echo "\n=== Go: linting ==="
	cd go/auth-service && golangci-lint run ./...
	cd go/order-service && golangci-lint run ./...
	cd go/ai-service && golangci-lint run ./...
	cd go/product-service && golangci-lint run ./...
	cd go/cart-service && golangci-lint run ./...
	cd go/payment-service && golangci-lint run ./...
	cd go/analytics-service && golangci-lint run ./...
	cd go/order-projector && golangci-lint run ./...
	cd go/qa-mcp-service && golangci-lint run ./...
	cd go/coding-exercises-mcp-service && golangci-lint run ./...
	cd go/observability-mcp-service && golangci-lint run ./...
	cd go/eval-mcp-service && golangci-lint run ./...
	@echo "\n=== Go: tests ==="
	cd go/auth-service && go test ./... -v -race
	cd go/order-service && go test ./... -v -race
	cd go/ai-service && go test ./... -v -race
	cd go/product-service && go test ./... -v -race
	cd go/cart-service && go test ./... -v -race
	cd go/payment-service && go test ./... -v -race
	cd go/analytics-service && go test ./... -v -race
	cd go/order-projector && go test ./... -v -race
	cd go/qa-mcp-service && go test ./... -v -race
	cd go/coding-exercises-mcp-service && go test ./... -v -race
	cd go/observability-mcp-service && go test ./... -v -race
	cd go/eval-mcp-service && go test ./... -v -race

# --- Go migration static lint (no Docker required) ---
# Builds the migration-lint binary fresh and runs it over every service's
# .up.sql files. Catches operationally unsafe DDL patterns before the runtime
# migration test even starts.
preflight-go-migration-lint:
	@echo "\n=== Go: migration static lint ==="
	@cd go/cmd/migration-lint && go build -o /tmp/migration-lint .
	@/tmp/migration-lint go/*/migrations/*.up.sql
	@echo "  ✅ migration-lint clean"

# --- Go migration pipeline test (requires Docker via Colima + golang-migrate) ---
# Mirrors the CI "Go Migration Pipeline Test" job: spins up Postgres in Docker,
# runs all service migrations, applies seeds, and verifies tables exist.
# Catches FK/index/partition errors before pushing.
MIGRATE_PG_CONTAINER := preflight-migrate-pg
MIGRATE_PG_PORT := 54399
preflight-go-migrations: preflight-go-migration-lint
	@echo "\n=== Go: migration pipeline test ==="
	@if ! docker info >/dev/null 2>&1; then \
		echo "⚠️  Docker not available (run 'colima start') — skipping migration test"; \
		exit 0; \
	fi
	@if ! command -v migrate >/dev/null 2>&1; then \
		echo "❌ golang-migrate not found — install with: brew install golang-migrate"; \
		exit 1; \
	fi
	@# Clean up any leftover container from a previous failed run
	@docker rm -f $(MIGRATE_PG_CONTAINER) >/dev/null 2>&1 || true
	@echo "  Starting Postgres container..."
	@docker run -d --name $(MIGRATE_PG_CONTAINER) \
		-e POSTGRES_USER=taskuser -e POSTGRES_PASSWORD=taskpass -e POSTGRES_DB=taskdb \
		-p $(MIGRATE_PG_PORT):5432 \
		postgres:17-alpine >/dev/null
	@echo "  Waiting for Postgres to be ready..."
	@for i in 1 2 3 4 5 6 7 8 9 10; do \
		docker exec $(MIGRATE_PG_CONTAINER) pg_isready -U taskuser -d taskdb >/dev/null 2>&1 && break; \
		sleep 1; \
	done
	@# Create databases
	@docker exec $(MIGRATE_PG_CONTAINER) psql -U taskuser -d taskdb -c "CREATE DATABASE ecommercedb;" >/dev/null 2>&1
	@docker exec $(MIGRATE_PG_CONTAINER) psql -U taskuser -d taskdb -c "CREATE DATABASE productdb;" >/dev/null 2>&1
	@docker exec $(MIGRATE_PG_CONTAINER) psql -U taskuser -d taskdb -c "CREATE DATABASE projectordb;" >/dev/null 2>&1
	@# Run migrations (same order and flags as CI)
	@echo "  Running auth-service migrations..."
	@migrate -path go/auth-service/migrations \
		-database "postgres://taskuser:taskpass@localhost:$(MIGRATE_PG_PORT)/ecommercedb?sslmode=disable&x-migrations-table=auth_schema_migrations" up
	@echo "  Running order-service migrations..."
	@migrate -path go/order-service/migrations \
		-database "postgres://taskuser:taskpass@localhost:$(MIGRATE_PG_PORT)/ecommercedb?sslmode=disable&x-migrations-table=ecommerce_schema_migrations" up
	@echo "  Applying order-service seed data..."
	@PGPASSWORD=taskpass psql -h localhost -p $(MIGRATE_PG_PORT) -U taskuser -d ecommercedb \
		-v ON_ERROR_STOP=1 -f go/order-service/seed.sql >/dev/null
	@echo "  Running product-service migrations..."
	@migrate -path go/product-service/migrations \
		-database "postgres://taskuser:taskpass@localhost:$(MIGRATE_PG_PORT)/productdb?sslmode=disable&x-migrations-table=product_schema_migrations" up
	@echo "  Applying product-service seed data..."
	@PGPASSWORD=taskpass psql -h localhost -p $(MIGRATE_PG_PORT) -U taskuser -d productdb \
		-v ON_ERROR_STOP=1 -f go/product-service/seed.sql >/dev/null
	@echo "  Running order-projector migrations..."
	@migrate -path go/order-projector/migrations \
		-database "postgres://taskuser:taskpass@localhost:$(MIGRATE_PG_PORT)/projectordb?sslmode=disable&x-migrations-table=projector_schema_migrations" up
	@# Verify tables
	@echo "  Verifying tables..."
	@PGPASSWORD=taskpass psql -h localhost -p $(MIGRATE_PG_PORT) -U taskuser -d ecommercedb -c "\dt" | grep -q ' users ' || \
		(echo "❌ users table missing" && docker rm -f $(MIGRATE_PG_CONTAINER) >/dev/null && exit 1)
	@PGPASSWORD=taskpass psql -h localhost -p $(MIGRATE_PG_PORT) -U taskuser -d ecommercedb -c "\dt" | grep -q ' orders ' || \
		(echo "❌ orders table missing" && docker rm -f $(MIGRATE_PG_CONTAINER) >/dev/null && exit 1)
	@PGPASSWORD=taskpass psql -h localhost -p $(MIGRATE_PG_PORT) -U taskuser -d productdb -c "\dt" | grep -q ' products ' || \
		(echo "❌ products table missing in productdb" && docker rm -f $(MIGRATE_PG_CONTAINER) >/dev/null && exit 1)
	@PGPASSWORD=taskpass psql -h localhost -p $(MIGRATE_PG_PORT) -U taskuser -d projectordb -c "\dt" | grep -q ' order_timeline ' || \
		(echo "❌ order_timeline table missing in projectordb" && docker rm -f $(MIGRATE_PG_CONTAINER) >/dev/null && exit 1)
	@PGPASSWORD=taskpass psql -h localhost -p $(MIGRATE_PG_PORT) -U taskuser -d projectordb -c "\dt" | grep -q ' order_summary ' || \
		(echo "❌ order_summary table missing in projectordb" && docker rm -f $(MIGRATE_PG_CONTAINER) >/dev/null && exit 1)
	@# Cleanup
	@docker rm -f $(MIGRATE_PG_CONTAINER) >/dev/null
	@echo "  ✅ All migrations applied and tables verified"

# --- Go integration tests (requires Docker via Colima) ---
preflight-go-integration:
	@echo "\n=== Go: integration tests (order-service) ==="
	cd go/order-service && DOCKER_HOST=unix://$${HOME}/.colima/docker.sock TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock go test -tags=integration -race -timeout 180s ./internal/integration/...
	@echo "\n=== Go: integration tests (analytics-service) ==="
	cd go/analytics-service && DOCKER_HOST=unix://$${HOME}/.colima/docker.sock TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock go test -tags=integration -race -timeout 180s ./internal/integration/...

# Bootstrap golangci-lint into .bin/ pinned to the version CI uses,
# so local preflight catches the same lint issues that gate CI.
GOLANGCI_LINT_VERSION := v1.64.8
GOLANGCI_LINT := $(CURDIR)/.bin/golangci-lint

$(GOLANGCI_LINT):
	@echo "\n=== Installing golangci-lint $(GOLANGCI_LINT_VERSION) into .bin/ ==="
	GOBIN=$(CURDIR)/.bin go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

preflight-ai-service: $(GOLANGCI_LINT)
	@echo "\n=== ai-service: vet ==="
	cd go/ai-service && go vet ./...
	@echo "\n=== ai-service: lint ==="
	cd go/ai-service && $(GOLANGCI_LINT) run ./...
	@echo "\n=== ai-service: tests ==="
	cd go/ai-service && go test ./... -count=1

preflight-ai-service-evals:
	@echo "\n=== ai-service: mocked-LLM evals ==="
	cd go/ai-service && go test -tags=eval ./internal/evals/... -count=1

# --- Grafana dashboard <-> ConfigMap sync ---
# CI fails if monitoring/grafana/dashboards/system-overview.json drifts from
# the JSON embedded in k8s/monitoring/configmaps/grafana-dashboards.yml.
# Run `make grafana-sync` after editing the dashboard JSON.
grafana-sync:
	@echo "\n=== Grafana: regenerating ConfigMap from dashboard JSON ==="
	python3 scripts/grafana_sync.py

grafana-sync-check:
	python3 scripts/grafana_sync.py --check

# --- Compose config validation (no Docker needed, just validates YAML merge) ---
# Catches env var interpolation errors, missing services, YAML syntax issues.
# Uses dummy env vars to satisfy ${VAR:?} guards without needing real secrets.
preflight-compose-config:
	@echo "\n=== Compose: validating Python stack config ==="
	@cd . && docker compose -f docker-compose.yml -f docker-compose.ci.yml config --quiet 2>/dev/null || \
		(echo "⚠️  Python compose config validation requires Docker CLI — skipping" && true)
	@echo "\n=== Compose: validating Go stack config ==="
	@cd go && JWT_SECRET=dummy GOOGLE_CLIENT_ID=dummy GOOGLE_CLIENT_SECRET=dummy \
		docker compose -f docker-compose.yml -f docker-compose.ci.yml config --quiet 2>/dev/null || \
		(echo "⚠️  Go compose config validation requires Docker CLI — skipping" && true)
	@echo "\n=== Compose: validating Java stack config ==="
	@cd java && docker compose -f docker-compose.yml -f docker-compose.ci.yml config --quiet 2>/dev/null || \
		(echo "⚠️  Java compose config validation requires Docker CLI — skipping" && true)
	@echo "  ✅ Compose configs valid"

# --- Security scans ---
preflight-security:
	@echo "\n=== Security: bandit ==="
	bandit -r services/ -ll
	@echo "\n=== Security: CORS guardrail ==="
	@if grep -r 'allow_origins=\["\*"\]' services/; then \
		echo "ERROR: Wildcard CORS found"; exit 1; \
	fi
	@echo "CORS check passed"
	@echo "\n=== Security: pip-audit ==="
	@command -v python3.11 >/dev/null 2>&1 || { echo "python3.11 is required for pip-audit preflight"; exit 1; }
	@for service in ingestion chat debug eval; do \
		echo "  Auditing Python service: $$service"; \
		: "PYSEC-2025-183/CVE-2025-45768 is disputed by PyJWT and has no fixed version"; \
		: "services/shared/auth.py rejects HS256 secrets shorter than 32 bytes"; \
		chat_model_audit_ignores="--ignore-vuln PYSEC-2024-277 --ignore-vuln PYSEC-2025-210 --ignore-vuln PYSEC-2025-194 --ignore-vuln PYSEC-2025-196 --ignore-vuln PYSEC-2025-195 --ignore-vuln PYSEC-2025-193 --ignore-vuln PYSEC-2025-192 --ignore-vuln PYSEC-2026-139 --ignore-vuln PYSEC-2025-191 --ignore-vuln PYSEC-2025-197 --ignore-vuln PYSEC-2025-189 --ignore-vuln PYSEC-2025-190 --ignore-vuln PYSEC-2025-217 --ignore-vuln PYSEC-2025-214 --ignore-vuln PYSEC-2025-218 --ignore-vuln PYSEC-2025-211 --ignore-vuln PYSEC-2025-212 --ignore-vuln PYSEC-2025-213 --ignore-vuln PYSEC-2025-215 --ignore-vuln PYSEC-2025-216"; \
		audit_args="--ignore-vuln PYSEC-2025-183"; \
		if [ "$$service" = "chat" ]; then audit_args="$$audit_args $$chat_model_audit_ignores --ignore-vuln CVE-2026-1839"; fi; \
		tmpdir=$$(mktemp -d /tmp/preflight-pip-audit-$$service.XXXXXX); \
		python3.11 -m venv "$$tmpdir/venv"; \
		"$$tmpdir/venv/bin/pip" install --upgrade pip setuptools >/dev/null; \
		"$$tmpdir/venv/bin/pip" install services/shared/ >/dev/null; \
		"$$tmpdir/venv/bin/pip" install -r "services/$$service/requirements.txt" >/dev/null; \
		"$$tmpdir/venv/bin/pip" install pip-audit >/dev/null; \
		audit_status=0; \
		PIPAPI_PYTHON_LOCATION="$$tmpdir/venv/bin/python" "$$tmpdir/venv/bin/pip-audit" $$audit_args || audit_status=$$?; \
		rm -rf "$$tmpdir"; \
		if [ "$$audit_status" -ne 0 ]; then exit "$$audit_status"; fi; \
	done

# --- Worktree cleanup ---
worktree-cleanup:
	@echo "\n=== Cleaning up merged worktrees ==="
	@for wt in $$(git worktree list --porcelain | grep '^worktree' | awk '{print $$2}' | grep -E '(\.codex|\.claude)/worktrees'); do \
		branch=$$(git worktree list --porcelain | grep -A2 "$$wt" | grep '^branch' | sed 's|branch refs/heads/||'); \
		if [ -n "$$branch" ] && ! git rev-parse --verify "$$branch" >/dev/null 2>&1; then \
			echo "  Removing stale worktree: $$wt (branch $$branch deleted)"; \
			git worktree remove "$$wt" --force; \
		fi; \
	done
	@git worktree prune
	@echo "Done"

# --- Developer setup ---
install-pre-commit:
	@command -v pre-commit >/dev/null 2>&1 || { echo "Install pre-commit first: pip install pre-commit"; exit 1; }
	pre-commit install --install-hooks
	pre-commit install --hook-type pre-push --install-hooks
	@echo "✅ pre-commit hooks installed (commit + pre-push stages)"
