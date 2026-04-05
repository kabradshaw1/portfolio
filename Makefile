.PHONY: preflight preflight-python preflight-frontend preflight-e2e preflight-java preflight-java-integration preflight-security

# Run all CI checks locally before pushing
preflight: preflight-python preflight-frontend preflight-security preflight-java
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
	ssh PC@100.79.113.84 "cd C:\Users\PC\repos\gen_ai_engineer && git pull && cd java && ./gradlew integrationTest --no-daemon"

# --- Security scans ---
preflight-security:
	@echo "\n=== Security: bandit ==="
	bandit -r services/ -ll
	@echo "\n=== Security: CORS guardrail ==="
	@if grep -r 'allow_origins=\["\*"\]' services/; then \
		echo "ERROR: Wildcard CORS found"; exit 1; \
	fi
	@echo "CORS check passed"
