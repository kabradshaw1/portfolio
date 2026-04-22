.PHONY: preflight preflight-python preflight-frontend preflight-e2e preflight-java preflight-java-integration preflight-go preflight-go-integration preflight-security preflight-ai-service preflight-ai-service-evals grafana-sync grafana-sync-check worktree-cleanup

# Run all CI checks locally before pushing
preflight: grafana-sync-check preflight-python preflight-frontend preflight-security preflight-java preflight-go
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
	@echo "\n=== Go: tests ==="
	cd go/auth-service && go test ./... -v -race
	cd go/order-service && go test ./... -v -race
	cd go/ai-service && go test ./... -v -race
	cd go/product-service && go test ./... -v -race
	cd go/cart-service && go test ./... -v -race
	cd go/payment-service && go test ./... -v -race

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

# --- Security scans ---
preflight-security:
	@echo "\n=== Security: bandit ==="
	bandit -r services/ -ll
	@echo "\n=== Security: CORS guardrail ==="
	@if grep -r 'allow_origins=\["\*"\]' services/; then \
		echo "ERROR: Wildcard CORS found"; exit 1; \
	fi
	@echo "CORS check passed"

# --- Worktree cleanup ---
worktree-cleanup:
	@echo "\n=== Cleaning up merged worktrees ==="
	@for wt in $$(git worktree list --porcelain | grep '^worktree' | awk '{print $$2}' | grep '.claude/worktrees'); do \
		branch=$$(git worktree list --porcelain | grep -A2 "$$wt" | grep '^branch' | sed 's|branch refs/heads/||'); \
		if [ -n "$$branch" ] && ! git rev-parse --verify "$$branch" >/dev/null 2>&1; then \
			echo "  Removing stale worktree: $$wt (branch $$branch deleted)"; \
			git worktree remove "$$wt" --force; \
		fi; \
	done
	@git worktree prune
	@echo "Done"
