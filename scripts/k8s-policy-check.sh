#!/usr/bin/env bash
# k8s-policy-check.sh — enforce portfolio-specific k8s manifest rules.
#
# Rules:
#   R1. Any Deployment whose container image references postgres, mongo, or
#       redis MUST define a readinessProbe on that container. Rationale:
#       without a probe, kubectl rollout status returns before the database
#       is actually accepting connections, causing startup races.
#   R2. Any ConfigMap data key ending in DATABASE_URL whose value starts with
#       postgres:// MUST include sslmode=disable. Rationale: the Go pq driver
#       defaults to sslmode=require against a non-SSL postgres.
#
# Usage: scripts/k8s-policy-check.sh [dir ...]
# Exits 0 on success, 1 on any violation. Prints each violation to stderr.
set -euo pipefail

if ! command -v yq >/dev/null 2>&1; then
  echo "k8s-policy-check.sh: yq is required (v4, Go rewrite)" >&2
  exit 2
fi

DIRS=("$@")
if [ "${#DIRS[@]}" -eq 0 ]; then
  DIRS=(k8s java/k8s go/k8s)
fi

violations=0
report() {
  echo "VIOLATION: $*" >&2
  violations=$((violations + 1))
}

check_file() {
  local file="$1"
  local doc_count
  doc_count=$(yq 'di' "$file" 2>/dev/null | tail -1)
  if [ -z "$doc_count" ]; then
    return 0
  fi

  local i
  for i in $(seq 0 "$doc_count"); do
    local kind
    kind=$(yq "select(di == $i) | .kind // \"\"" "$file")

    if [ "$kind" = "Deployment" ]; then
      local n_containers
      n_containers=$(yq "select(di == $i) | .spec.template.spec.containers | length" "$file")
      local c
      for c in $(seq 0 $((n_containers - 1))); do
        local image probe
        image=$(yq "select(di == $i) | .spec.template.spec.containers[$c].image // \"\"" "$file")
        if echo "$image" | grep -Eq '(^|/)(postgres|mongo|redis)(:|$)'; then
          probe=$(yq "select(di == $i) | .spec.template.spec.containers[$c].readinessProbe // \"null\"" "$file")
          if [ "$probe" = "null" ]; then
            local name
            name=$(yq "select(di == $i) | .metadata.name" "$file")
            report "$file: Deployment/$name container '$image' is missing readinessProbe (R1)"
          fi
        fi
      done
    fi

    if [ "$kind" = "ConfigMap" ]; then
      local keys
      keys=$(yq "select(di == $i) | .data // {} | keys | .[]" "$file" 2>/dev/null || true)
      local key
      while IFS= read -r key; do
        [ -z "$key" ] && continue
        case "$key" in
          *DATABASE_URL)
            local value
            value=$(yq "select(di == $i) | .data[\"$key\"]" "$file")
            if echo "$value" | grep -q '^postgres://'; then
              if ! echo "$value" | grep -q 'sslmode=disable'; then
                local name
                name=$(yq "select(di == $i) | .metadata.name" "$file")
                report "$file: ConfigMap/$name key '$key' missing sslmode=disable (R2)"
              fi
            fi
            ;;
        esac
      done <<< "$keys"
    fi
  done
}

for dir in "${DIRS[@]}"; do
  if [ ! -d "$dir" ]; then
    continue
  fi
  while IFS= read -r -d '' file; do
    check_file "$file"
  done < <(find "$dir" -type f \( -name '*.yml' -o -name '*.yaml' \) -print0)
done

if [ "$violations" -gt 0 ]; then
  echo "" >&2
  echo "k8s-policy-check: $violations violation(s) found" >&2
  exit 1
fi

echo "k8s-policy-check: all rules passed"
