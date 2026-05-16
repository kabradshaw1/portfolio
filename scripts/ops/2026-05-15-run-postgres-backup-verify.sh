#!/usr/bin/env bash
# Run the fixed Postgres backup verification CronJob once to refresh Grafana metrics.
# This addresses stale backup-verification alerts after the curl-free verifier landed.
# Idempotent: yes. It creates a timestamped Job from the CronJob and only reads logs/status.

set -euo pipefail

ssh debian bash <<'REMOTE'
set -euo pipefail

namespace="java-tasks"
cronjob="postgres-backup-verify"
job="${cronjob}-manual-$(date -u +%Y%m%d%H%M%S)"

echo "Creating ${namespace}/${job} from cronjob/${cronjob}..."
kubectl create job -n "${namespace}" --from="cronjob/${cronjob}" "${job}"

echo "Waiting for ${job} to complete..."
if ! kubectl wait -n "${namespace}" --for=condition=complete --timeout=35m "job/${job}"; then
  echo "Job did not complete successfully. Recent pod logs:" >&2
  kubectl get pods -n "${namespace}" -l "job-name=${job}" -o name \
    | while read -r pod; do
        echo "== ${pod} ==" >&2
        kubectl logs -n "${namespace}" "${pod}" --all-containers --tail=160 >&2 || true
      done
  exit 1
fi

kubectl get pods -n "${namespace}" -l "job-name=${job}" -o name \
  | while read -r pod; do
      echo "== ${pod} =="
      kubectl logs -n "${namespace}" "${pod}" --all-containers --tail=160
    done

echo "Manual backup verification Job succeeded: ${namespace}/${job}"
REMOTE
