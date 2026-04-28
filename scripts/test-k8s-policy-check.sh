#!/usr/bin/env bash
# Tests for k8s-policy-check.sh. Creates temp fixtures with known-bad and
# known-good manifests and asserts the script's exit code and output.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
POLICY="$SCRIPT_DIR/k8s-policy-check.sh"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

fail() { echo "FAIL: $*" >&2; exit 1; }
pass() { echo "PASS: $*"; }

# --- Fixture 1: postgres Deployment WITHOUT readinessProbe (should fail) ---
mkdir -p "$TMP/case1"
cat > "$TMP/case1/postgres.yml" <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: postgres
spec:
  template:
    spec:
      containers:
        - name: postgres
          image: postgres:17-alpine
EOF
if "$POLICY" "$TMP/case1" >/dev/null 2>&1; then
  fail "case1: postgres without readinessProbe should have failed"
fi
pass "case1: missing postgres readinessProbe is detected"

# --- Fixture 2: postgres Deployment WITH readinessProbe (should pass) ---
mkdir -p "$TMP/case2"
cat > "$TMP/case2/postgres.yml" <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: postgres
spec:
  template:
    spec:
      containers:
        - name: postgres
          image: postgres:17-alpine
          readinessProbe:
            exec:
              command: ["pg_isready"]
EOF
if ! "$POLICY" "$TMP/case2" >/dev/null 2>&1; then
  fail "case2: postgres with readinessProbe should have passed"
fi
pass "case2: postgres with readinessProbe passes"

# --- Fixture 3: ConfigMap postgres:// URL WITHOUT sslmode=disable (fail) ---
mkdir -p "$TMP/case3"
cat > "$TMP/case3/cm.yml" <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: bad
data:
  DATABASE_URL: postgres://host:5432/db
EOF
if "$POLICY" "$TMP/case3" >/dev/null 2>&1; then
  fail "case3: postgres URL without sslmode=disable should have failed"
fi
pass "case3: missing sslmode=disable is detected"

# --- Fixture 4: ConfigMap postgres:// URL WITH sslmode=disable (pass) ---
mkdir -p "$TMP/case4"
cat > "$TMP/case4/cm.yml" <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: good
data:
  DATABASE_URL: postgres://host:5432/db?sslmode=disable
EOF
if ! "$POLICY" "$TMP/case4" >/dev/null 2>&1; then
  fail "case4: postgres URL with sslmode=disable should have passed"
fi
pass "case4: sslmode=disable URL passes"

# --- Fixture 5: unrelated Deployment without probe (should pass) ---
mkdir -p "$TMP/case5"
cat > "$TMP/case5/svc.yml" <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: chat
spec:
  template:
    spec:
      containers:
        - name: chat
          image: ghcr.io/example/chat:latest
EOF
if ! "$POLICY" "$TMP/case5" >/dev/null 2>&1; then
  fail "case5: unrelated Deployment without probe should have passed"
fi
pass "case5: non-stateful Deployment without probe is allowed"

# --- Fixture 6: ConfigMap with creds in non-DATABASE_URL key (R3 fail) ---
mkdir -p "$TMP/case6"
cat > "$TMP/case6/cm.yml" <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: bad
data:
  RABBITMQ_URL: amqp://guest:guest@rabbitmq:5672
EOF
if "$POLICY" "$TMP/case6" >/dev/null 2>&1; then
  fail "case6: amqp:// with embedded credentials should have failed (R3)"
fi
pass "case6: credential URL in ConfigMap is detected (R3)"

# --- Fixture 7: ConfigMap with host:port only (no creds) — should pass ---
mkdir -p "$TMP/case7"
cat > "$TMP/case7/cm.yml" <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: good
data:
  REDIS_URL: redis://redis.java-tasks.svc.cluster.local:6379
  POSTGRES_HOST: postgres.java-tasks.svc.cluster.local
  POSTGRES_PORT: "5432"
EOF
if ! "$POLICY" "$TMP/case7" >/dev/null 2>&1; then
  fail "case7: credential-free URLs should have passed"
fi
pass "case7: host:port URLs without credentials pass (R3)"

# --- Fixture 8: ConfigMap with mongodb+srv credentials — should fail (R3) ---
mkdir -p "$TMP/case8"
cat > "$TMP/case8/cm.yml" <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: bad
data:
  MONGO_URL: mongodb+srv://user:secret@cluster.example.net/db
EOF
if "$POLICY" "$TMP/case8" >/dev/null 2>&1; then
  fail "case8: mongodb+srv credentials should have failed (R3)"
fi
pass "case8: mongodb+srv credentials detected (R3)"

echo
echo "All policy check tests passed."
