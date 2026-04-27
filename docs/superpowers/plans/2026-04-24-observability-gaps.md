# Observability Gaps Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close three observability gaps identified during the 2026-04-24 Postgres incident: deploy annotations in Grafana, Kubernetes Warning event forwarding to Loki, and per-service Postgres connection tracking.

**Architecture:** Config-only changes across K8s manifests and CI. No application code changes. Section 1 adds Grafana admin password + CI annotation curls. Section 2 adds a new kubernetes-event-exporter Deployment that pushes Warning events to Loki. Section 3 appends `&application_name=<service>` to DATABASE_URL in Go service configmaps and adds a dashboard panel.

**Tech Stack:** Kubernetes manifests (YAML), GitHub Actions CI, Grafana JSON dashboards, opsgenie/kubernetes-event-exporter

---

## File Map

### Section 1: Grafana Deploy Annotations
| File | Action |
|------|--------|
| `k8s/monitoring/deployments/grafana.yml` | Modify — add `GF_SECURITY_ADMIN_PASSWORD` env, re-enable login form |
| `k8s/monitoring/secrets/grafana-secrets.yml.template` | Modify — add `grafana-admin-password` key |
| `.github/workflows/ci.yml` | Modify — add annotation curl after rollout restarts (QA + prod) |

### Section 2: Kubernetes Event Exporter
| File | Action |
|------|--------|
| `k8s/monitoring/deployments/kube-event-exporter.yml` | Create |
| `k8s/monitoring/configmaps/kube-event-exporter-config.yml` | Create |
| `k8s/monitoring/rbac/kube-event-exporter-serviceaccount.yml` | Create |
| `k8s/monitoring/rbac/kube-event-exporter-clusterrole.yml` | Create |
| `k8s/monitoring/rbac/kube-event-exporter-clusterrolebinding.yml` | Create |
| `k8s/monitoring/kustomization.yaml` | Modify — register new resources |

### Section 3: Per-Service Postgres Connection Tracking
| File | Action |
|------|--------|
| `go/k8s/configmaps/auth-service-config.yml` | Modify — append `&application_name=auth-service` to DATABASE_URL |
| `go/k8s/configmaps/order-service-config.yml` | Modify — append `&application_name=order-service` |
| `go/k8s/configmaps/product-service-config.yml` | Modify — append `&application_name=product-service` |
| `go/k8s/configmaps/cart-service-config.yml` | Modify — append `&application_name=cart-service` |
| `go/k8s/configmaps/payment-service-config.yml` | Modify — append `&application_name=payment-service` |
| `go/k8s/configmaps/order-projector-config.yml` | Modify — append `&application_name=order-projector` |
| `k8s/overlays/qa-go/kustomization.yaml` | Modify — append `&application_name=<service>` to QA DATABASE_URL patches |
| `k8s/monitoring/configmaps/grafana-dashboards.yml` | Modify — add "Connections by Service" panel to PostgreSQL dashboard |

---

### Task 1: Grafana Admin Password & Login

**Files:**
- Modify: `k8s/monitoring/secrets/grafana-secrets.yml.template`
- Modify: `k8s/monitoring/deployments/grafana.yml`

- [ ] **Step 1: Add grafana-admin-password to secrets template**

In `k8s/monitoring/secrets/grafana-secrets.yml.template`, add a second key. The secret name stays `grafana-secrets` (new secret, separate from the existing `telegram-bot` secret):

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: grafana-secrets
  namespace: monitoring
type: Opaque
stringData:
  TELEGRAM_BOT_TOKEN: "<your-rotated-telegram-bot-token>"
  grafana-admin-password: "<your-grafana-admin-password>"
```

- [ ] **Step 2: Add GF_SECURITY_ADMIN_PASSWORD env to grafana.yml**

In `k8s/monitoring/deployments/grafana.yml`, add the admin password env var from the new secret, and set `GF_AUTH_DISABLE_LOGIN_FORM` to `"false"` so the admin can log in. Add after the existing `GF_UNIFIED_ALERTING_ENABLED` env var:

```yaml
            - name: GF_SECURITY_ADMIN_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: grafana-secrets
                  key: grafana-admin-password
                  optional: true
```

And change:
```yaml
            - name: GF_AUTH_DISABLE_LOGIN_FORM
              value: "false"
```

Anonymous Viewer access stays enabled so dashboards are still publicly viewable.

- [ ] **Step 3: Commit**

```bash
git add k8s/monitoring/secrets/grafana-secrets.yml.template k8s/monitoring/deployments/grafana.yml
git commit -m "feat(monitoring): add Grafana admin password for API access"
```

---

### Task 2: CI Deploy Annotations

**Files:**
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Add annotation curl to QA deploy**

In `.github/workflows/ci.yml`, in the `deploy-qa` job, after the three `kubectl rollout restart` commands for QA namespaces (after line 1226, before the rollout status checks), add:

```bash
          # Post deploy annotations to Grafana
          SHA="${{ github.sha }}"
          SHORT_SHA="${SHA:0:7}"
          for NS in ai-services-qa java-tasks-qa go-ecommerce-qa; do
            $SSH "curl -sf -X POST 'http://grafana.monitoring.svc.cluster.local:3000/api/annotations' \
              -H 'Content-Type: application/json' \
              -d '{\"text\":\"Deploy: ${NS} (sha:${SHORT_SHA})\",\"tags\":[\"qa-deploy\",\"${NS}\"]}'" || echo "::warning::Grafana annotation failed for ${NS} (non-fatal)"
          done
```

Note: This uses Grafana's anonymous auth (which is set to Viewer role). The annotation API requires at least Editor role, so this will need the `GRAFANA_API_KEY` bearer token once the service account is created. For now, use anonymous — if it fails (403), it's non-fatal. The spec calls for adding `GRAFANA_API_KEY` as a GitHub secret after manual service account creation; update the curl to add `-H "Authorization: Bearer $GRAFANA_API_KEY"` once that's done.

Actually, per the spec: we should use the API key from the start. Add `GRAFANA_API_KEY: ${{ secrets.GRAFANA_API_KEY }}` to the env block and use it:

```bash
          # Post deploy annotations to Grafana
          SHA="${{ github.sha }}"
          SHORT_SHA="${SHA:0:7}"
          if [ -n "${GRAFANA_API_KEY:-}" ]; then
            for NS in ai-services-qa java-tasks-qa go-ecommerce-qa; do
              $SSH "curl -sf -X POST 'http://grafana.monitoring.svc.cluster.local:3000/api/annotations' \
                -H 'Authorization: Bearer ${GRAFANA_API_KEY}' \
                -H 'Content-Type: application/json' \
                -d '{\"text\":\"Deploy: ${NS} (sha:${SHORT_SHA})\",\"tags\":[\"qa-deploy\",\"${NS}\"]}'" || echo "::warning::Grafana annotation failed for ${NS} (non-fatal)"
            done
          fi
```

Add `GRAFANA_API_KEY` to the `env:` block of the QA deploy step (alongside `SSH_PRIVATE_KEY`, `STRIPE_SECRET_KEY`, etc.):
```yaml
          GRAFANA_API_KEY: ${{ secrets.GRAFANA_API_KEY }}
```

- [ ] **Step 2: Add annotation curl to prod deploy**

Same pattern in the `deploy-prod` job. Add after the final `kubectl rollout restart` blocks (before monitoring health checks, around line 1427). The namespaces are `ai-services`, `java-tasks`, `go-ecommerce` and the tag is `deploy` (not `qa-deploy`):

```bash
          # Post deploy annotations to Grafana
          SHA="${{ github.sha }}"
          SHORT_SHA="${SHA:0:7}"
          if [ -n "${GRAFANA_API_KEY:-}" ]; then
            for NS in ai-services java-tasks go-ecommerce; do
              $SSH "curl -sf -X POST 'http://grafana.monitoring.svc.cluster.local:3000/api/annotations' \
                -H 'Authorization: Bearer ${GRAFANA_API_KEY}' \
                -H 'Content-Type: application/json' \
                -d '{\"text\":\"Deploy: ${NS} (sha:${SHORT_SHA})\",\"tags\":[\"deploy\",\"${NS}\"]}'" || echo "::warning::Grafana annotation failed for ${NS} (non-fatal)"
            done
          fi
```

Add `GRAFANA_API_KEY` to the prod deploy `env:` block:
```yaml
          GRAFANA_API_KEY: ${{ secrets.GRAFANA_API_KEY }}
```

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "feat(ci): post Grafana deploy annotations after QA and prod rollouts"
```

---

### Task 3: Kubernetes Event Exporter RBAC

**Files:**
- Create: `k8s/monitoring/rbac/kube-event-exporter-serviceaccount.yml`
- Create: `k8s/monitoring/rbac/kube-event-exporter-clusterrole.yml`
- Create: `k8s/monitoring/rbac/kube-event-exporter-clusterrolebinding.yml`

- [ ] **Step 1: Create ServiceAccount**

Create `k8s/monitoring/rbac/kube-event-exporter-serviceaccount.yml`:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: kube-event-exporter
  namespace: monitoring
```

- [ ] **Step 2: Create ClusterRole**

Create `k8s/monitoring/rbac/kube-event-exporter-clusterrole.yml`:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kube-event-exporter
rules:
  - apiGroups: [""]
    resources:
      - events
    verbs: ["get", "watch", "list"]
```

- [ ] **Step 3: Create ClusterRoleBinding**

Create `k8s/monitoring/rbac/kube-event-exporter-clusterrolebinding.yml`:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kube-event-exporter
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: kube-event-exporter
subjects:
  - kind: ServiceAccount
    name: kube-event-exporter
    namespace: monitoring
```

- [ ] **Step 4: Commit**

```bash
git add k8s/monitoring/rbac/kube-event-exporter-serviceaccount.yml \
        k8s/monitoring/rbac/kube-event-exporter-clusterrole.yml \
        k8s/monitoring/rbac/kube-event-exporter-clusterrolebinding.yml
git commit -m "feat(monitoring): add RBAC for kubernetes-event-exporter"
```

---

### Task 4: Kubernetes Event Exporter ConfigMap & Deployment

**Files:**
- Create: `k8s/monitoring/configmaps/kube-event-exporter-config.yml`
- Create: `k8s/monitoring/deployments/kube-event-exporter.yml`

- [ ] **Step 1: Create ConfigMap with Warning filter + Loki sink**

Create `k8s/monitoring/configmaps/kube-event-exporter-config.yml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kube-event-exporter-config
  namespace: monitoring
data:
  config.yaml: |
    logLevel: error
    logFormat: json
    route:
      routes:
        - match:
            - receiver: "loki"
    receivers:
      - name: "loki"
        webhook:
          endpoint: "http://loki.monitoring.svc.cluster.local:3100/loki/api/v1/push"
          headers:
            Content-Type: "application/json"
          layout:
            streams:
              - stream:
                  job: "kube-event-exporter"
                  namespace: "{{ .InvolvedObject.Namespace }}"
                  reason: "{{ .Reason }}"
                  kind: "{{ .InvolvedObject.Kind }}"
                  name: "{{ .InvolvedObject.Name }}"
                values:
                  - - "{{ .GetTimestampMs }}000000"
                    - "{{ .Reason }}: {{ .Message }} ({{ .InvolvedObject.Kind }}/{{ .InvolvedObject.Name }} in {{ .InvolvedObject.Namespace }})"
```

- [ ] **Step 2: Create Deployment**

Create `k8s/monitoring/deployments/kube-event-exporter.yml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kube-event-exporter
  namespace: monitoring
spec:
  replicas: 1
  selector:
    matchLabels:
      app: kube-event-exporter
  template:
    metadata:
      labels:
        app: kube-event-exporter
    spec:
      serviceAccountName: kube-event-exporter
      containers:
        - name: kube-event-exporter
          image: ghcr.io/resmoio/kubernetes-event-exporter:v1.7
          args:
            - -conf=/data/config.yaml
          volumeMounts:
            - name: config
              mountPath: /data
          resources:
            requests:
              memory: "32Mi"
              cpu: "10m"
            limits:
              memory: "128Mi"
              cpu: "100m"
      volumes:
        - name: config
          configMap:
            name: kube-event-exporter-config
```

Note: The image is `ghcr.io/resmoio/kubernetes-event-exporter:v1.7` — this is the maintained fork (opsgenie/kubernetes-event-exporter is archived, resmoio is the active continuation).

- [ ] **Step 3: Commit**

```bash
git add k8s/monitoring/configmaps/kube-event-exporter-config.yml \
        k8s/monitoring/deployments/kube-event-exporter.yml
git commit -m "feat(monitoring): add kubernetes-event-exporter deployment with Loki sink"
```

---

### Task 5: Register Event Exporter in Kustomization

**Files:**
- Modify: `k8s/monitoring/kustomization.yaml`

- [ ] **Step 1: Add new resources**

Add the 4 new files to `k8s/monitoring/kustomization.yaml`. Insert RBAC entries after the existing promtail RBAC block, the configmap after existing configmaps, and the deployment after existing deployments:

After `- rbac/promtail-serviceaccount.yml`:
```yaml
  - rbac/kube-event-exporter-serviceaccount.yml
  - rbac/kube-event-exporter-clusterrole.yml
  - rbac/kube-event-exporter-clusterrolebinding.yml
```

After `- configmaps/prometheus-config.yml`:
```yaml
  - configmaps/kube-event-exporter-config.yml
```

After `- deployments/prometheus.yml`:
```yaml
  - deployments/kube-event-exporter.yml
```

- [ ] **Step 2: Commit**

```bash
git add k8s/monitoring/kustomization.yaml
git commit -m "feat(monitoring): register kube-event-exporter in kustomization"
```

---

### Task 6: Per-Service application_name in Prod ConfigMaps

**Files:**
- Modify: `go/k8s/configmaps/auth-service-config.yml`
- Modify: `go/k8s/configmaps/order-service-config.yml`
- Modify: `go/k8s/configmaps/product-service-config.yml`
- Modify: `go/k8s/configmaps/cart-service-config.yml`
- Modify: `go/k8s/configmaps/payment-service-config.yml`
- Modify: `go/k8s/configmaps/order-projector-config.yml`

- [ ] **Step 1: Append application_name to each DATABASE_URL**

For each configmap, append `&application_name=<service>` to the end of the DATABASE_URL value. The `?sslmode=disable` is the first query param, so `&` is correct:

| File | Old suffix | New suffix |
|------|-----------|------------|
| auth-service-config.yml | `authdb?sslmode=disable` | `authdb?sslmode=disable&application_name=auth-service` |
| order-service-config.yml | `orderdb?sslmode=disable` | `orderdb?sslmode=disable&application_name=order-service` |
| product-service-config.yml | `productdb?sslmode=disable` | `productdb?sslmode=disable&application_name=product-service` |
| cart-service-config.yml | `cartdb?sslmode=disable` | `cartdb?sslmode=disable&application_name=cart-service` |
| payment-service-config.yml | `paymentdb?sslmode=disable` | `paymentdb?sslmode=disable&application_name=payment-service` |
| order-projector-config.yml | `projectordb?sslmode=disable` | `projectordb?sslmode=disable&application_name=order-projector` |

- [ ] **Step 2: Commit**

```bash
git add go/k8s/configmaps/auth-service-config.yml \
        go/k8s/configmaps/order-service-config.yml \
        go/k8s/configmaps/product-service-config.yml \
        go/k8s/configmaps/cart-service-config.yml \
        go/k8s/configmaps/payment-service-config.yml \
        go/k8s/configmaps/order-projector-config.yml
git commit -m "feat(go): add application_name to DATABASE_URL for pg_stat_activity tracking"
```

---

### Task 7: Per-Service application_name in QA Overlay

**Files:**
- Modify: `k8s/overlays/qa-go/kustomization.yaml`

- [ ] **Step 1: Append application_name to each QA DATABASE_URL patch**

In `k8s/overlays/qa-go/kustomization.yaml`, update each ConfigMap patch that has a DATABASE_URL value. Append `&application_name=<service>` to the end:

| Target ConfigMap | Old DATABASE_URL value suffix | New suffix |
|-----------------|------------------------------|------------|
| auth-service-config | `authdb_qa?sslmode=disable` | `authdb_qa?sslmode=disable&application_name=auth-service` |
| order-service-config | `orderdb_qa?sslmode=disable` | `orderdb_qa?sslmode=disable&application_name=order-service` |
| product-service-config | `productdb_qa?sslmode=disable` | `productdb_qa?sslmode=disable&application_name=product-service` |
| cart-service-config | `cartdb_qa?sslmode=disable` | `cartdb_qa?sslmode=disable&application_name=cart-service` |
| payment-service-config | `paymentdb_qa?sslmode=disable` | `paymentdb_qa?sslmode=disable&application_name=payment-service` |
| order-projector-config | `projectordb_qa?sslmode=disable` | `projectordb_qa?sslmode=disable&application_name=order-projector` |

- [ ] **Step 2: Commit**

```bash
git add k8s/overlays/qa-go/kustomization.yaml
git commit -m "feat(qa): add application_name to QA DATABASE_URL patches"
```

---

### Task 8: Connections by Service Dashboard Panel

**Files:**
- Modify: `k8s/monitoring/configmaps/grafana-dashboards.yml`

- [ ] **Step 1: Add "Connections by Service" bar gauge panel to PostgreSQL dashboard**

In the `postgresql.json` section of `k8s/monitoring/configmaps/grafana-dashboards.yml`, add a new panel after the existing "Deadlocks" panel (id 6). Insert before the closing `]` of the `panels` array (before line 3470):

```json
        ,
        {
          "title": "Connections by Service",
          "type": "bargauge",
          "gridPos": { "h": 8, "w": 12, "x": 0, "y": 16 },
          "id": 7,
          "datasource": { "type": "prometheus", "uid": "" },
          "targets": [
            {
              "expr": "pg_stat_activity_count{datname!~\"template.*|postgres\"}",
              "legendFormat": "{{application_name}}",
              "refId": "A"
            }
          ],
          "fieldConfig": {
            "defaults": {
              "unit": "short",
              "thresholds": {
                "steps": [
                  { "color": "green", "value": null },
                  { "color": "yellow", "value": 10 },
                  { "color": "red", "value": 20 }
                ]
              }
            },
            "overrides": []
          },
          "options": {
            "orientation": "horizontal",
            "displayMode": "gradient"
          }
        }
```

- [ ] **Step 2: Commit**

```bash
git add k8s/monitoring/configmaps/grafana-dashboards.yml
git commit -m "feat(monitoring): add Connections by Service panel to PostgreSQL dashboard"
```

---

### Task 9: Final Verification

- [ ] **Step 1: Validate kustomize builds**

```bash
kubectl kustomize k8s/monitoring/ > /dev/null && echo "monitoring OK"
kubectl kustomize k8s/overlays/qa-go/ > /dev/null && echo "qa-go OK"
```

Both should succeed without errors.

- [ ] **Step 2: Verify no YAML syntax issues**

Spot-check that all new YAML files parse correctly:
```bash
python3 -c "import yaml; yaml.safe_load(open('k8s/monitoring/deployments/kube-event-exporter.yml'))"
python3 -c "import yaml; yaml.safe_load(open('k8s/monitoring/configmaps/kube-event-exporter-config.yml'))"
python3 -c "import yaml; yaml.safe_load(open('k8s/monitoring/rbac/kube-event-exporter-serviceaccount.yml'))"
python3 -c "import yaml; yaml.safe_load(open('k8s/monitoring/rbac/kube-event-exporter-clusterrole.yml'))"
python3 -c "import yaml; yaml.safe_load(open('k8s/monitoring/rbac/kube-event-exporter-clusterrolebinding.yml'))"
```

- [ ] **Step 3: Squash into a single feature commit if desired, or leave as incremental commits**
