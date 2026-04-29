#!/usr/bin/env bash
# Remove stale QA backup PVCs that came from shared prod Postgres manifests.
# The QA overlay now deletes these PVCs at render time; this clears the already
# materialized Pending objects from the shared cluster.
# Idempotent: yes.

set -euo pipefail

ssh debian bash <<'REMOTE'
set -euo pipefail

kubectl delete pvc postgres-backup postgres-backup-readonly \
  -n java-tasks-qa \
  --ignore-not-found=true
REMOTE
