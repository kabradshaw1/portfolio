.PHONY: preflight preflight-python preflight-frontend preflight-e2e preflight-java preflight-java-integration preflight-go preflight-security preflight-ai-service preflight-ai-service-evals

# Run all CI checks locally before pushing
preflight: preflight-python preflight-frontend preflight-security preflight-java preflight-go
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
	cd go/ecommerce-service && golangci-lint run ./...
	cd go/ai-service && golangci-lint run ./...
	@echo "\n=== Go: tests ==="
	cd go/auth-service && go test ./... -v -race
	cd go/ecommerce-service && go test ./... -v -race
	cd go/ai-service && go test ./... -v -race

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

# --- Security scans ---
preflight-security:
	@echo "\n=== Security: bandit ==="
	bandit -r services/ -ll
	@echo "\n=== Security: CORS guardrail ==="
	@if grep -r 'allow_origins=\["\*"\]' services/; then \
		echo "ERROR: Wildcard CORS found"; exit 1; \
	fi
	@echo "CORS check passed"
