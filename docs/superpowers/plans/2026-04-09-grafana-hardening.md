# Grafana Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the missing-Secret outage class from Grafana, land a first-time visitor directly on the dashboard, and add a "Scrape Targets" panel that shows the live health of every Prometheus scrape job.

**Architecture:** All changes are small YAML / JSON edits plus one git-rm of a template file. No new services, no new CI. The existing production smoke test at `frontend/e2e/smoke-prod/smoke.spec.ts` already covers Grafana health (verified — see Spec §4 note).

**Tech Stack:** Kubernetes manifests (YAML), Grafana dashboard JSON, Playwright smoke (existing, unchanged).

**Spec:** `docs/superpowers/specs/2026-04-09-grafana-hardening-design.md`

---

## File Structure

Files modified:
- `k8s/monitoring/deployments/grafana.yml` — drop Secret env, add default-home and login-form env vars.
- `k8s/monitoring/configmaps/grafana-dashboards.yml` — add Scrape Targets row + panel to embedded JSON.
- `monitoring/grafana/dashboards/system-overview.json` — same change as the ConfigMap (keep in sync for the Docker Compose path).

Files deleted:
- `k8s/monitoring/secrets/grafana-secrets.yml.template`

Files NOT modified (explicitly):
- `.gitignore` — the `**/k8s/secrets/*.yml` pattern covers other services and must stay.
- `frontend/e2e/smoke-prod/smoke.spec.ts` — already asserts HTTP 200 and `body.database === "ok"` on `https://grafana.kylebradshaw.dev/api/health`. No change needed.
- `k8s/monitoring/services/grafana.yml`, `k8s/monitoring/ingress.yml` — correct as-is.

**Branch:** `grafana-hardening` (already created and already contains the spec commit).

---

### Task 1: Update Grafana Deployment env vars

**Files:**
- Modify: `k8s/monitoring/deployments/grafana.yml:21-32`

- [ ] **Step 1: Edit the env block**

Replace lines 21–32 of `k8s/monitoring/deployments/grafana.yml`. The current block is:

```yaml
          env:
            - name: GF_SECURITY_ADMIN_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: grafana-secrets
                  key: admin-password
            - name: GF_AUTH_ANONYMOUS_ENABLED
              value: "true"
            - name: GF_AUTH_ANONYMOUS_ORG_ROLE
              value: Viewer
            - name: GF_SERVER_ROOT_URL
              value: https://grafana.kylebradshaw.dev
```

Replace it with:

```yaml
          env:
            - name: GF_AUTH_ANONYMOUS_ENABLED
              value: "true"
            - name: GF_AUTH_ANONYMOUS_ORG_ROLE
              value: Viewer
            - name: GF_AUTH_DISABLE_LOGIN_FORM
              value: "true"
            - name: GF_USERS_ALLOW_SIGN_UP
              value: "false"
            - name: GF_SERVER_ROOT_URL
              value: https://grafana.kylebradshaw.dev
            - name: GF_DASHBOARDS_DEFAULT_HOME_DASHBOARD_PATH
              value: /var/lib/grafana/dashboards/system-overview.json
```

- [ ] **Step 2: Verify YAML parses**

Run: `python3 -c "import yaml; yaml.safe_load(open('k8s/monitoring/deployments/grafana.yml'))"`
Expected: no output, exit 0.

- [ ] **Step 3: Verify no references to the Secret remain**

Run: `grep -n "grafana-secrets\|GF_SECURITY_ADMIN_PASSWORD" k8s/monitoring/deployments/grafana.yml`
Expected: no matches.

- [ ] **Step 4: Commit**

```bash
git add k8s/monitoring/deployments/grafana.yml
git commit -m "fix(monitoring): remove Grafana admin Secret and set default home dashboard

Drops GF_SECURITY_ADMIN_PASSWORD + secretKeyRef (the root cause of the
recent outage — the Secret was gitignored and missing on the cluster).
Disables the login form entirely since anonymous Viewer is the intended
access model. Sets system-overview.json as the default home dashboard so
the root URL lands directly on the dashboard."
```

---

### Task 2: Delete the obsolete Secret template

**Files:**
- Delete: `k8s/monitoring/secrets/grafana-secrets.yml.template`

- [ ] **Step 1: Remove the file via git**

Run: `git rm k8s/monitoring/secrets/grafana-secrets.yml.template`
Expected: `rm 'k8s/monitoring/secrets/grafana-secrets.yml.template'`

- [ ] **Step 2: Verify the monitoring secrets directory is otherwise empty**

Run: `ls k8s/monitoring/secrets/ 2>/dev/null || true`
Expected: empty output (directory is empty or removed).

- [ ] **Step 3: Confirm nothing else references the template**

Run: `grep -rn "grafana-secrets" k8s/ docs/ scripts/ 2>/dev/null`
Expected: no matches outside `docs/superpowers/specs/2026-04-09-grafana-hardening-design.md` and `docs/superpowers/plans/2026-04-09-grafana-hardening.md` (the spec and this plan may reference it historically — that's fine).

- [ ] **Step 4: Commit**

```bash
git add -u k8s/monitoring/secrets/
git commit -m "chore(monitoring): delete obsolete grafana-secrets template

Grafana no longer uses an admin Secret — login form is disabled and
anonymous Viewer is the only access mode."
```

---

### Task 3: Add Scrape Targets panel to source dashboard JSON

**Files:**
- Modify: `monitoring/grafana/dashboards/system-overview.json:383-384` (insert a new row header + panel at the end of the `panels` array, just before the closing `]`).

This is the source of truth for the dashboard that gets mirrored into the K8s ConfigMap in Task 4.

- [ ] **Step 1: Inspect the current end of the panels array**

Run: `sed -n '380,395p' monitoring/grafana/dashboards/system-overview.json`
Expected output (around line 383 is the `]` that closes `panels`):

```
        {
          "expr": "count(up)",
          "legendFormat": "Total",
          "refId": "A",
          "instant": true
        }
      ]
    }
  ],
  "schemaVersion": 39,
  "tags": ["monitoring", "portfolio"],
  "templating": { "list": [] },
  "time": { "from": "now-1h", "to": "now" },
  "timepicker": {},
  "timezone": "browser",
  "title": "System Overview",
```

- [ ] **Step 2: Insert the new row and table panel**

Find this exact block (the end of the "Total Services" stat panel through the `],` that closes the panels array):

```json
        {
          "expr": "count(up)",
          "legendFormat": "Total",
          "refId": "A",
          "instant": true
        }
      ]
    }
  ],
```

Replace it with:

```json
        {
          "expr": "count(up)",
          "legendFormat": "Total",
          "refId": "A",
          "instant": true
        }
      ]
    },
    {
      "collapsed": false,
      "gridPos": { "h": 1, "w": 24, "x": 0, "y": 23 },
      "id": 103,
      "title": "Scrape Targets",
      "type": "row"
    },
    {
      "title": "Prometheus Scrape Targets",
      "type": "table",
      "gridPos": { "h": 9, "w": 24, "x": 0, "y": 24 },
      "id": 11,
      "datasource": { "type": "prometheus", "uid": "" },
      "description": "Live health of every service and exporter Prometheus scrapes.",
      "fieldConfig": {
        "defaults": {
          "custom": { "align": "left", "displayMode": "color-background" },
          "mappings": [
            { "type": "value", "options": { "0": { "text": "DOWN", "color": "red" } } },
            { "type": "value", "options": { "1": { "text": "UP", "color": "green" } } }
          ],
          "thresholds": {
            "mode": "absolute",
            "steps": [
              { "color": "red", "value": null },
              { "color": "green", "value": 1 }
            ]
          }
        },
        "overrides": [
          {
            "matcher": { "id": "byName", "options": "Time" },
            "properties": [{ "id": "custom.hidden", "value": true }]
          },
          {
            "matcher": { "id": "byName", "options": "instance" },
            "properties": [{ "id": "custom.hidden", "value": true }]
          }
        ]
      },
      "options": {
        "showHeader": true,
        "sortBy": [{ "displayName": "job", "desc": false }]
      },
      "targets": [
        {
          "expr": "up",
          "format": "table",
          "refId": "A",
          "instant": true
        }
      ],
      "transformations": [
        {
          "id": "organize",
          "options": {
            "excludeByName": { "Time": true, "instance": true, "__name__": true },
            "renameByName": { "job": "Job", "Value": "Status" }
          }
        }
      ]
    }
  ],
```

- [ ] **Step 3: Verify the JSON still parses**

Run: `python3 -c "import json; json.load(open('monitoring/grafana/dashboards/system-overview.json'))"`
Expected: no output, exit 0.

- [ ] **Step 4: Verify both new panel IDs are unique**

Run: `python3 -c "import json; d=json.load(open('monitoring/grafana/dashboards/system-overview.json')); ids=[p['id'] for p in d['panels']]; assert len(ids)==len(set(ids)), ids; print('ok', sorted(ids))"`
Expected: `ok [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 100, 101, 102, 103]`

- [ ] **Step 5: Commit**

```bash
git add monitoring/grafana/dashboards/system-overview.json
git commit -m "feat(monitoring): add Scrape Targets panel to system-overview dashboard

Adds a new row with a table listing every Prometheus scrape job and its
live UP/DOWN status. Gives recruiters a concrete view of the services
running in the portfolio stack instead of an abstract 'N running' stat."
```

---

### Task 4: Mirror the dashboard change into the K8s ConfigMap

**Files:**
- Modify: `k8s/monitoring/configmaps/grafana-dashboards.yml`

The ConfigMap embeds the same dashboard JSON verbatim under `data.system-overview.json: |` with 4-space indentation. Easier to regenerate from the source than to hand-edit.

- [ ] **Step 1: Regenerate the ConfigMap from the source JSON**

Run this exact command (creates the file with the updated JSON properly indented):

```bash
python3 - <<'PY'
import json
src = open('monitoring/grafana/dashboards/system-overview.json').read()
# Validate before embedding
json.loads(src)
indented = '\n'.join(('    ' + line) if line else line for line in src.splitlines())
header = (
    "apiVersion: v1\n"
    "kind: ConfigMap\n"
    "metadata:\n"
    "  name: grafana-dashboards\n"
    "  namespace: monitoring\n"
    "data:\n"
    "  system-overview.json: |\n"
)
open('k8s/monitoring/configmaps/grafana-dashboards.yml', 'w').write(header + indented + '\n')
print('regenerated')
PY
```

Expected output: `regenerated`

- [ ] **Step 2: Verify the ConfigMap parses as YAML and the embedded JSON parses as JSON**

Run:

```bash
python3 -c "
import yaml, json
cm = yaml.safe_load(open('k8s/monitoring/configmaps/grafana-dashboards.yml'))
assert cm['kind'] == 'ConfigMap'
assert cm['metadata']['name'] == 'grafana-dashboards'
d = json.loads(cm['data']['system-overview.json'])
ids = [p['id'] for p in d['panels']]
assert 11 in ids and 103 in ids, ids
print('ok', sorted(ids))
"
```

Expected: `ok [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 100, 101, 102, 103]`

- [ ] **Step 3: Verify the two dashboard copies are byte-identical JSON**

Run:

```bash
python3 -c "
import yaml, json
a = json.load(open('monitoring/grafana/dashboards/system-overview.json'))
b = json.loads(yaml.safe_load(open('k8s/monitoring/configmaps/grafana-dashboards.yml'))['data']['system-overview.json'])
assert a == b, 'drift between source JSON and ConfigMap'
print('in sync')
"
```

Expected: `in sync`

- [ ] **Step 4: Commit**

```bash
git add k8s/monitoring/configmaps/grafana-dashboards.yml
git commit -m "feat(k8s): mirror Scrape Targets panel into grafana-dashboards ConfigMap"
```

---

### Task 5: Deploy to the cluster and verify

**Files:** none (operational)

This task runs against the live cluster on the Windows PC. All commands go through SSH. **Do NOT push or merge** — Kyle does that. This task is for verifying the manifests work when applied, after Kyle has merged the branch through staging → main and deployed.

> **Note for Claude:** Only execute this task after Kyle confirms the branch has been merged to main and the normal deploy has run. Until then, stop after Task 4 and hand back to Kyle.

- [ ] **Step 1: Confirm deploy has landed**

Run: `ssh PC@100.79.113.84 "kubectl -n monitoring get deploy grafana -o jsonpath='{.spec.template.spec.containers[0].env}' | python3 -m json.tool"`
Expected: output contains `GF_DASHBOARDS_DEFAULT_HOME_DASHBOARD_PATH` and does NOT contain `GF_SECURITY_ADMIN_PASSWORD`.

If the old env is still present, the deploy hasn't rolled yet. Stop and tell Kyle.

- [ ] **Step 2: Delete the now-orphaned Secret from the cluster**

Run: `ssh PC@100.79.113.84 "kubectl -n monitoring delete secret grafana-secrets --ignore-not-found"`
Expected: either `secret "grafana-secrets" deleted` or `No resources found` — both are fine.

- [ ] **Step 3: Confirm the Grafana pod is healthy**

Run: `ssh PC@100.79.113.84 "kubectl -n monitoring get pods -l app=grafana"`
Expected: exactly one pod, `1/1 Running`, age matches the recent rollout (not 4d+).

- [ ] **Step 4: Hit the health endpoint**

Run: `curl -sS https://grafana.kylebradshaw.dev/api/health`
Expected: JSON body with `"database": "ok"`.

- [ ] **Step 5: Hit the root URL and confirm it renders the dashboard, not the Home page**

Ask Kyle to open `https://grafana.kylebradshaw.dev/` in a browser and confirm:
- No login form appears.
- The landing page is the System Overview dashboard (not the default Grafana Home).
- A new "Scrape Targets" row is visible at the bottom with a table of jobs (at least: `prometheus`, `windows`, `nvidia-gpu`, `qdrant`, `ingestion`, `chat`, `debug`, `gateway-service`, `grafana`) each marked UP or DOWN.

If any panel shows "No data" for System or GPU rows, that's an exporter issue on the Windows host, not a regression from this change — flag to Kyle separately.

- [ ] **Step 6: No commit**

This task makes no code changes.

---

## Self-Review

**Spec coverage:**
- §1 Remove admin Secret → Task 1 (env + login form disable), Task 2 (delete template), Task 5 Step 2 (cluster cleanup). ✓
- §2 Default home dashboard → Task 1 (env var). ✓
- §3 Scrape Targets panel → Task 3 (source JSON), Task 4 (ConfigMap mirror). ✓
- §4 Post-deploy smoke test → Already exists at `frontend/e2e/smoke-prod/smoke.spec.ts:32-39`. Plan §4 notes this; no task needed. ✓
- §5 Rollout → Task 5 covers cluster verification. ✓
- Non-goals respected: no app instrumentation, no alerting, no log aggregation, no scheduled checks, no auth editing. ✓

**Placeholder scan:** No TBDs, TODOs, or "handle edge cases" phrases. All code blocks show exact content. ✓

**Type consistency:** Panel IDs 11 and 103 are new and unique. Task 4 Step 2/3 verify the set matches what Task 3 produced. `system-overview` UID unchanged. ✓

**Risks flagged in spec:** `grafana/grafana:latest` is still used (out of scope, flagged in spec §Risks). Dashboard drift risk is mitigated by Task 4 Step 3 (byte-equality check).
