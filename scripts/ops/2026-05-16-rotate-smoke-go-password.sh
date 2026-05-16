#!/usr/bin/env bash
# Rotate the seeded Go smoke-test user's password hash in prod and QA auth DBs.
# The plaintext password is stored in GitHub Actions secret SMOKE_GO_PASSWORD.
# Idempotent: yes. Re-running sets the same committed bcrypt hash on the same user.

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
seed_file="${repo_root}/go/auth-service/seed.sql"
email="smoke@kylebradshaw.dev"

password_hash="$(
  sed -n "s/.*SELECT '${email}', '\\([^']*\\)'.*/\\1/p" "${seed_file}" | head -n 1
)"

if [[ -z "${password_hash}" ]]; then
  echo "Could not extract smoke password hash from ${seed_file}" >&2
  exit 1
fi

password_hash_b64="$(printf '%s' "${password_hash}" | base64)"

ssh debian bash -s -- "${email}" "${password_hash_b64}" <<'REMOTE'
set -euo pipefail

email="$1"
password_hash="$(printf '%s' "$2" | base64 -d)"
namespace="java-tasks"
postgres="deployment/postgres"
databases=(authdb authdb_qa)

for db in "${databases[@]}"; do
  if ! kubectl exec -n "${namespace}" "${postgres}" -c postgres -- \
    psql -U taskuser -d taskdb -Atq \
      -c "SELECT 1 FROM pg_database WHERE datname='${db}'" | grep -qx 1; then
    echo "Skipping missing database ${db}"
    continue
  fi

  echo "Updating smoke user password hash in ${db}..."
  kubectl exec -i -n "${namespace}" "${postgres}" -c postgres -- \
    psql -U taskuser -d "${db}" -v ON_ERROR_STOP=1 \
      -v email="${email}" \
      -v password_hash="${password_hash}" <<'SQL'
UPDATE users
SET password_hash = :'password_hash'
WHERE email = :'email';
SQL

  if ! kubectl exec -i -n "${namespace}" "${postgres}" -c postgres -- \
    psql -U taskuser -d "${db}" -v ON_ERROR_STOP=1 -Atq \
      -v email="${email}" \
      <<'SQL' | grep -qx 1; then
SELECT 1 FROM users WHERE email = :'email';
SQL
    echo "Smoke user ${email} not found in ${db}" >&2
    exit 1
  fi
done

echo "Smoke Go password hash rotation complete."
REMOTE
