# Stage B — Kubernetes Visibility Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give Prometheus persistent storage, RBAC for Kubernetes service discovery, and deploy kube-state-metrics + node-exporter so the dashboard can show running pods, restarts, CPU/mem by namespace, and node health.

**Architecture:** Prometheus gains a PVC (10 GiB, 15-day retention), a ServiceAccount + ClusterRoleBinding for pod/node/service discovery, and a Kubernetes SD scrape job. kube-state-metrics and node-exporter are deployed from upstream manifests (pinned versions, not `latest`). The dead per-service `/health` scrape jobs are replaced with a single `k8s-pods` SD job — but the actual `/metrics` endpoints don't exist until Stages C1–C3, so those targets will show as `down` until then. The old static jobs for in-cluster services are removed; only the Windows host static jobs remain.

**Tech Stack:** Prometheus 2.53, kube-state-metrics v2.13.0, node-exporter v1.8.2, Minikube `standard` StorageClass.

**Parent spec:** `docs/superpowers/specs/2026-04-09-grafana-overhaul-design.md`

---

## Pre-flight context

- Prometheus runs as `deployment/prometheus` in `monitoring` ns, no ServiceAccount, no PVC, no RBAC. Data lives in `emptyDir` (lost on restart).
- Prometheus image is `prom/prometheus:latest`. We pin to `v2.53.0` for reproducibility.
- No kube-state-metrics or node-exporter deployed anywhere.
- StorageClass `standard` (minikube-hostpath) is available, default.
- `k8s/deploy.sh` handles all deploys — new manifests must be wired into it.
- 19 deployments across 4 app namespaces + ingress-nginx + kube-system. 27 pods total.
- Grafana reads dashboards from ConfigMap `grafana-dashboards`. Dashboard JSON changes require re-applying the ConfigMap and restarting Grafana.

---

## Task 1: Create Prometheus RBAC manifests

**Files:**
- Create: `k8s/monitoring/rbac/prometheus-clusterrole.yml`
- Create: `k8s/monitoring/rbac/prometheus-serviceaccount.yml`
- Create: `k8s/monitoring/rbac/prometheus-clusterrolebinding.yml`

- [ ] **Step 1: Create the rbac directory**

```bash
mkdir -p k8s/monitoring/rbac
```

- [ ] **Step 2: Write the ServiceAccount**

Create `k8s/monitoring/rbac/prometheus-serviceaccount.yml`:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: prometheus
  namespace: monitoring
```

- [ ] **Step 3: Write the ClusterRole**

Create `k8s/monitoring/rbac/prometheus-clusterrole.yml`:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: prometheus
rules:
  - apiGroups: [""]
    resources:
      - nodes
      - nodes/metrics
      - services
      - endpoints
      - pods
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources:
      - configmaps
    verbs: ["get"]
  - apiGroups: ["networking.k8s.io"]
    resources:
      - ingresses
    verbs: ["get", "list", "watch"]
  - nonResourceURLs: ["/metrics"]
    verbs: ["get"]
```

- [ ] **Step 4: Write the ClusterRoleBinding**

Create `k8s/monitoring/rbac/prometheus-clusterrolebinding.yml`:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: prometheus
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: prometheus
subjects:
  - kind: ServiceAccount
    name: prometheus
    namespace: monitoring
```

- [ ] **Step 5: Commit**

```bash
git add k8s/monitoring/rbac/
git commit -m "feat(monitoring): add Prometheus RBAC for k8s service discovery

ClusterRole grants get/list/watch on pods, services, endpoints, nodes,
and ingresses. Required for Kubernetes SD scrape config in Stage B.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: Create the Prometheus PVC

**Files:**
- Create: `k8s/monitoring/pvc/prometheus-data.yml`

- [ ] **Step 1: Create the pvc directory**

```bash
mkdir -p k8s/monitoring/pvc
```

- [ ] **Step 2: Write the PVC manifest**

Create `k8s/monitoring/pvc/prometheus-data.yml`:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: prometheus-data
  namespace: monitoring
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi
```

Uses Minikube's default `standard` StorageClass (minikube-hostpath). No `storageClassName` field needed — the default will be used automatically.

- [ ] **Step 3: Commit**

```bash
git add k8s/monitoring/pvc/
git commit -m "feat(monitoring): add 10Gi PVC for Prometheus data persistence

Replaces emptyDir — metrics survive pod restarts. Uses Minikube default
StorageClass (minikube-hostpath).

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: Update Prometheus deployment

**Files:**
- Modify: `k8s/monitoring/deployments/prometheus.yml`

- [ ] **Step 1: Read the current file**

Read `k8s/monitoring/deployments/prometheus.yml` (already read above, but verify before editing).

- [ ] **Step 2: Rewrite the Prometheus deployment**

Replace the entire contents of `k8s/monitoring/deployments/prometheus.yml` with:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: prometheus
  namespace: monitoring
spec:
  replicas: 1
  selector:
    matchLabels:
      app: prometheus
  strategy:
    type: Recreate
  template:
    metadata:
      labels:
        app: prometheus
    spec:
      serviceAccountName: prometheus
      securityContext:
        fsGroup: 65534
        runAsUser: 65534
        runAsNonRoot: true
      containers:
        - name: prometheus
          image: prom/prometheus:v2.53.0
          ports:
            - containerPort: 9090
          args:
            - "--config.file=/etc/prometheus/prometheus.yml"
            - "--storage.tsdb.path=/prometheus"
            - "--storage.tsdb.retention.time=15d"
            - "--storage.tsdb.retention.size=8GB"
            - "--web.enable-lifecycle"
          volumeMounts:
            - name: config
              mountPath: /etc/prometheus/prometheus.yml
              subPath: prometheus.yml
            - name: data
              mountPath: /prometheus
          resources:
            requests:
              memory: "256Mi"
              cpu: "100m"
            limits:
              memory: "512Mi"
              cpu: "500m"
          readinessProbe:
            httpGet:
              path: /-/ready
              port: 9090
            initialDelaySeconds: 5
            periodSeconds: 10
          livenessProbe:
            httpGet:
              path: /-/healthy
              port: 9090
            initialDelaySeconds: 30
            periodSeconds: 15
      volumes:
        - name: config
          configMap:
            name: prometheus-config
        - name: data
          persistentVolumeClaim:
            claimName: prometheus-data
```

Key changes from original:
- `serviceAccountName: prometheus` (RBAC)
- `securityContext` — Prometheus image runs as `nobody` (65534), `fsGroup` ensures PVC is writable
- `strategy: Recreate` — required for RWO PVC (can't have two pods mounting the same volume)
- Pinned image to `v2.53.0`
- Added `--storage.tsdb.path`, `--storage.tsdb.retention.time=15d`, `--storage.tsdb.retention.size=8GB`
- Added PVC volume mount at `/prometheus`
- Added `--web.enable-lifecycle` (allows config reloads via POST)
- Added `livenessProbe`

- [ ] **Step 3: Commit**

```bash
git add k8s/monitoring/deployments/prometheus.yml
git commit -m "feat(monitoring): upgrade Prometheus deployment with PVC, RBAC, retention

Pin to v2.53.0, mount prometheus-data PVC, add serviceAccountName,
set 15d/8GB retention, add lifecycle API and liveness probe.
Recreate strategy required for RWO PVC.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: Update Prometheus scrape config

**Files:**
- Modify: `k8s/monitoring/configmaps/prometheus-config.yml`

- [ ] **Step 1: Read the current file**

Read `k8s/monitoring/configmaps/prometheus-config.yml` (verify before editing).

- [ ] **Step 2: Replace the entire scrape config**

Replace the full contents of `k8s/monitoring/configmaps/prometheus-config.yml` with:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: prometheus-config
  namespace: monitoring
data:
  prometheus.yml: |
    global:
      scrape_interval: 15s
      evaluation_interval: 15s

    scrape_configs:
      - job_name: "prometheus"
        static_configs:
          - targets: ["localhost:9090"]

      - job_name: "windows"
        static_configs:
          - targets: ["host.minikube.internal:9182"]

      - job_name: "nvidia-gpu"
        static_configs:
          - targets: ["host.minikube.internal:9835"]

      - job_name: "kube-state-metrics"
        static_configs:
          - targets: ["kube-state-metrics.monitoring.svc.cluster.local:8080"]

      - job_name: "node-exporter"
        static_configs:
          - targets: ["node-exporter.monitoring.svc.cluster.local:9100"]

      - job_name: "k8s-pods"
        kubernetes_sd_configs:
          - role: pod
        relabel_configs:
          - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scrape]
            action: keep
            regex: "true"
          - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_path]
            action: replace
            target_label: __metrics_path__
            regex: (.+)
          - source_labels: [__address__, __meta_kubernetes_pod_annotation_prometheus_io_port]
            action: replace
            regex: ([^:]+)(?::\d+)?;(\d+)
            replacement: $1:$2
            target_label: __address__
          - source_labels: [__meta_kubernetes_namespace]
            target_label: namespace
          - source_labels: [__meta_kubernetes_pod_name]
            target_label: pod
          - source_labels: [__meta_kubernetes_pod_label_app]
            target_label: service
```

Key changes:
- Removed all per-service static jobs (`qdrant`, `ingestion`, `chat`, `debug`, `gateway-service`, `grafana`) — those were scraping `/health` or `/actuator/health` (JSON, not Prometheus metrics). Services will be discovered via `k8s-pods` SD once they have `/metrics` endpoints and the right pod annotations (Stages C1–C3).
- Added `kube-state-metrics` and `node-exporter` static jobs.
- Added `k8s-pods` Kubernetes SD job with relabel rules for `prometheus.io/scrape`, `prometheus.io/port`, `prometheus.io/path` annotations.
- Kept Windows host static jobs (`windows`, `nvidia-gpu`) — those can't use k8s SD.

- [ ] **Step 3: Commit**

```bash
git add k8s/monitoring/configmaps/prometheus-config.yml
git commit -m "feat(monitoring): replace static scrape jobs with k8s pod SD

Remove dead per-service /health scrape jobs that returned JSON.
Add kube-state-metrics + node-exporter targets. Add k8s-pods SD
job that discovers pods via prometheus.io annotations. Windows host
static jobs retained.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: Deploy kube-state-metrics

**Files:**
- Create: `k8s/monitoring/rbac/kube-state-metrics-clusterrole.yml`
- Create: `k8s/monitoring/rbac/kube-state-metrics-serviceaccount.yml`
- Create: `k8s/monitoring/rbac/kube-state-metrics-clusterrolebinding.yml`
- Create: `k8s/monitoring/deployments/kube-state-metrics.yml`
- Create: `k8s/monitoring/services/kube-state-metrics.yml`

- [ ] **Step 1: Write the ServiceAccount**

Create `k8s/monitoring/rbac/kube-state-metrics-serviceaccount.yml`:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: kube-state-metrics
  namespace: monitoring
```

- [ ] **Step 2: Write the ClusterRole**

Create `k8s/monitoring/rbac/kube-state-metrics-clusterrole.yml`:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kube-state-metrics
rules:
  - apiGroups: [""]
    resources:
      - configmaps
      - secrets
      - nodes
      - pods
      - services
      - serviceaccounts
      - resourcequotas
      - replicationcontrollers
      - limitranges
      - persistentvolumeclaims
      - persistentvolumes
      - namespaces
      - endpoints
    verbs: ["list", "watch"]
  - apiGroups: ["apps"]
    resources:
      - statefulsets
      - daemonsets
      - deployments
      - replicasets
    verbs: ["list", "watch"]
  - apiGroups: ["batch"]
    resources:
      - cronjobs
      - jobs
    verbs: ["list", "watch"]
  - apiGroups: ["autoscaling"]
    resources:
      - horizontalpodautoscalers
    verbs: ["list", "watch"]
  - apiGroups: ["networking.k8s.io"]
    resources:
      - networkpolicies
      - ingresses
    verbs: ["list", "watch"]
  - apiGroups: ["coordination.k8s.io"]
    resources:
      - leases
    verbs: ["list", "watch"]
  - apiGroups: ["authentication.k8s.io"]
    resources:
      - tokenreviews
    verbs: ["create"]
  - apiGroups: ["authorization.k8s.io"]
    resources:
      - subjectaccessreviews
    verbs: ["create"]
```

- [ ] **Step 3: Write the ClusterRoleBinding**

Create `k8s/monitoring/rbac/kube-state-metrics-clusterrolebinding.yml`:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kube-state-metrics
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: kube-state-metrics
subjects:
  - kind: ServiceAccount
    name: kube-state-metrics
    namespace: monitoring
```

- [ ] **Step 4: Write the Deployment**

Create `k8s/monitoring/deployments/kube-state-metrics.yml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kube-state-metrics
  namespace: monitoring
spec:
  replicas: 1
  selector:
    matchLabels:
      app: kube-state-metrics
  template:
    metadata:
      labels:
        app: kube-state-metrics
    spec:
      serviceAccountName: kube-state-metrics
      securityContext:
        runAsUser: 65534
        runAsNonRoot: true
        fsGroup: 65534
      containers:
        - name: kube-state-metrics
          image: registry.k8s.io/kube-state-metrics/kube-state-metrics:v2.13.0
          ports:
            - name: http-metrics
              containerPort: 8080
            - name: telemetry
              containerPort: 8081
          readinessProbe:
            httpGet:
              path: /healthz
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 10
          livenessProbe:
            httpGet:
              path: /healthz
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 10
          resources:
            requests:
              memory: "64Mi"
              cpu: "50m"
            limits:
              memory: "128Mi"
              cpu: "100m"
```

- [ ] **Step 5: Write the Service**

Create `k8s/monitoring/services/kube-state-metrics.yml`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: kube-state-metrics
  namespace: monitoring
spec:
  selector:
    app: kube-state-metrics
  ports:
    - name: http-metrics
      port: 8080
      targetPort: http-metrics
    - name: telemetry
      port: 8081
      targetPort: telemetry
```

- [ ] **Step 6: Commit**

```bash
git add k8s/monitoring/rbac/kube-state-metrics-*.yml k8s/monitoring/deployments/kube-state-metrics.yml k8s/monitoring/services/kube-state-metrics.yml
git commit -m "feat(monitoring): add kube-state-metrics deployment

Pinned to v2.13.0. Own ServiceAccount + ClusterRole with list/watch
on all standard resource types. Exposes :8080 for metrics.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 6: Deploy node-exporter

**Files:**
- Create: `k8s/monitoring/daemonsets/node-exporter.yml`
- Create: `k8s/monitoring/services/node-exporter.yml`

- [ ] **Step 1: Create daemonsets directory**

```bash
mkdir -p k8s/monitoring/daemonsets
```

- [ ] **Step 2: Write the DaemonSet**

Create `k8s/monitoring/daemonsets/node-exporter.yml`:

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: node-exporter
  namespace: monitoring
spec:
  selector:
    matchLabels:
      app: node-exporter
  template:
    metadata:
      labels:
        app: node-exporter
    spec:
      hostNetwork: true
      hostPID: true
      containers:
        - name: node-exporter
          image: prom/node-exporter:v1.8.2
          args:
            - "--path.procfs=/host/proc"
            - "--path.sysfs=/host/sys"
            - "--path.rootfs=/host/root"
            - "--collector.filesystem.mount-points-exclude=^/(dev|proc|sys|var/lib/docker/.+|var/lib/kubelet/.+)($|/)"
          ports:
            - name: metrics
              containerPort: 9100
              hostPort: 9100
          volumeMounts:
            - name: proc
              mountPath: /host/proc
              readOnly: true
            - name: sys
              mountPath: /host/sys
              readOnly: true
            - name: root
              mountPath: /host/root
              mountPropagation: HostToContainer
              readOnly: true
          resources:
            requests:
              memory: "32Mi"
              cpu: "25m"
            limits:
              memory: "64Mi"
              cpu: "100m"
      tolerations:
        - effect: NoSchedule
          operator: Exists
      volumes:
        - name: proc
          hostPath:
            path: /proc
        - name: sys
          hostPath:
            path: /sys
        - name: root
          hostPath:
            path: /
```

- [ ] **Step 3: Write the Service**

Create `k8s/monitoring/services/node-exporter.yml`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: node-exporter
  namespace: monitoring
spec:
  selector:
    app: node-exporter
  ports:
    - name: metrics
      port: 9100
      targetPort: metrics
```

- [ ] **Step 4: Commit**

```bash
git add k8s/monitoring/daemonsets/ k8s/monitoring/services/node-exporter.yml
git commit -m "feat(monitoring): add node-exporter DaemonSet

Pinned to v1.8.2. Host network + PID for accurate metrics.
Mounts /proc, /sys, / read-only.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 7: Update deploy.sh

**Files:**
- Modify: `k8s/deploy.sh`

- [ ] **Step 1: Read the current file**

Read `k8s/deploy.sh` (already read above, verify before editing).

- [ ] **Step 2: Add RBAC, PVC, kube-state-metrics, and node-exporter to the deploy script**

Insert the following after the `echo "==> Applying ConfigMaps..."` block (after line 38) and before `echo "==> Deploying ai-services (Qdrant + Ollama)..."` (line 40):

```bash

echo "==> Applying monitoring RBAC..."
kubectl apply -f "$SCRIPT_DIR/monitoring/rbac/"

echo "==> Applying monitoring PVCs..."
kubectl apply -f "$SCRIPT_DIR/monitoring/pvc/"
```

Then replace the existing monitoring deploy section (lines 88–92):

```bash
echo "==> Deploying monitoring..."
kubectl apply -f "$SCRIPT_DIR/monitoring/deployments/prometheus.yml"
kubectl apply -f "$SCRIPT_DIR/monitoring/services/prometheus.yml"
kubectl apply -f "$SCRIPT_DIR/monitoring/deployments/grafana.yml"
kubectl apply -f "$SCRIPT_DIR/monitoring/services/grafana.yml"
```

with:

```bash
echo "==> Deploying monitoring..."
kubectl apply -f "$SCRIPT_DIR/monitoring/deployments/prometheus.yml"
kubectl apply -f "$SCRIPT_DIR/monitoring/services/prometheus.yml"
kubectl apply -f "$SCRIPT_DIR/monitoring/deployments/kube-state-metrics.yml"
kubectl apply -f "$SCRIPT_DIR/monitoring/services/kube-state-metrics.yml"
kubectl apply -f "$SCRIPT_DIR/monitoring/daemonsets/node-exporter.yml"
kubectl apply -f "$SCRIPT_DIR/monitoring/services/node-exporter.yml"
kubectl apply -f "$SCRIPT_DIR/monitoring/deployments/grafana.yml"
kubectl apply -f "$SCRIPT_DIR/monitoring/services/grafana.yml"
```

And in the wait section, after the `prometheus` wait (line 111), add:

```bash
kubectl wait --for=condition=available --timeout=120s deployment/kube-state-metrics -n monitoring
```

(node-exporter is a DaemonSet — no `condition=available` to wait for; it'll be ready before Grafana.)

- [ ] **Step 3: Commit**

```bash
git add k8s/deploy.sh
git commit -m "feat(monitoring): wire RBAC, PVC, kube-state-metrics, node-exporter into deploy.sh

Adds kubectl apply for rbac/, pvc/, kube-state-metrics deployment+service,
node-exporter daemonset+service. Adds wait for kube-state-metrics.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 8: Deploy to Minikube and verify

This task is executed over SSH on the Windows PC. No repo changes.

- [ ] **Step 1: Apply RBAC**

```bash
ssh PC@100.79.113.84 'kubectl apply -f -' < k8s/monitoring/rbac/prometheus-serviceaccount.yml
ssh PC@100.79.113.84 'kubectl apply -f -' < k8s/monitoring/rbac/prometheus-clusterrole.yml
ssh PC@100.79.113.84 'kubectl apply -f -' < k8s/monitoring/rbac/prometheus-clusterrolebinding.yml
ssh PC@100.79.113.84 'kubectl apply -f -' < k8s/monitoring/rbac/kube-state-metrics-serviceaccount.yml
ssh PC@100.79.113.84 'kubectl apply -f -' < k8s/monitoring/rbac/kube-state-metrics-clusterrole.yml
ssh PC@100.79.113.84 'kubectl apply -f -' < k8s/monitoring/rbac/kube-state-metrics-clusterrolebinding.yml
```

Expected: 6 `created` or `configured` lines.

- [ ] **Step 2: Apply PVC**

```bash
ssh PC@100.79.113.84 'kubectl apply -f -' < k8s/monitoring/pvc/prometheus-data.yml
```

Expected: `persistentvolumeclaim/prometheus-data created`.

Verify:
```bash
ssh PC@100.79.113.84 'kubectl get pvc -n monitoring'
```

Expected: `prometheus-data` with status `Bound` (minikube-hostpath binds immediately).

- [ ] **Step 3: Apply updated Prometheus config**

```bash
ssh PC@100.79.113.84 'kubectl apply -f -' < k8s/monitoring/configmaps/prometheus-config.yml
```

- [ ] **Step 4: Apply updated Prometheus deployment**

```bash
ssh PC@100.79.113.84 'kubectl apply -f -' < k8s/monitoring/deployments/prometheus.yml
```

Expected: `deployment.apps/prometheus configured`. The deployment will recreate the pod (strategy: Recreate).

Wait for rollout:
```bash
ssh PC@100.79.113.84 'kubectl rollout status deployment/prometheus -n monitoring --timeout=120s'
```

Expected: `deployment "prometheus" successfully rolled out`.

- [ ] **Step 5: Deploy kube-state-metrics**

```bash
ssh PC@100.79.113.84 'kubectl apply -f -' < k8s/monitoring/deployments/kube-state-metrics.yml
ssh PC@100.79.113.84 'kubectl apply -f -' < k8s/monitoring/services/kube-state-metrics.yml
ssh PC@100.79.113.84 'kubectl rollout status deployment/kube-state-metrics -n monitoring --timeout=120s'
```

- [ ] **Step 6: Deploy node-exporter**

```bash
ssh PC@100.79.113.84 'kubectl apply -f -' < k8s/monitoring/daemonsets/node-exporter.yml
ssh PC@100.79.113.84 'kubectl apply -f -' < k8s/monitoring/services/node-exporter.yml
```

Verify pod running:
```bash
ssh PC@100.79.113.84 'kubectl get pods -n monitoring -l app=node-exporter'
```

Expected: 1 pod, `Running`.

- [ ] **Step 7: Verify all monitoring pods healthy**

```bash
ssh PC@100.79.113.84 'kubectl get pods -n monitoring'
```

Expected: 4 pods (prometheus, grafana, kube-state-metrics, node-exporter), all `Running`.

- [ ] **Step 8: Verify Prometheus targets**

```bash
ssh PC@100.79.113.84 'kubectl -n monitoring exec deploy/prometheus -- wget -qO- "http://localhost:9090/api/v1/targets?state=active"' 2>&1 | python3 -c '
import sys, json
d = json.load(sys.stdin)
for t in d["data"]["activeTargets"]:
    print(f"{t[\"labels\"][\"job\"]:25s} {t[\"health\"]:5s} {t.get(\"lastError\", \"\")[:60]}")
'
```

Expected:
- `prometheus` — `up`
- `windows` — `up`
- `nvidia-gpu` — `up`
- `kube-state-metrics` — `up`
- `node-exporter` — `up`
- `k8s-pods` — no targets yet (no pods have `prometheus.io/scrape` annotation; this is correct until Stages C1–C3)

- [ ] **Step 9: Verify kube-state-metrics data**

```bash
ssh PC@100.79.113.84 'kubectl -n monitoring exec deploy/prometheus -- wget -qO- "http://localhost:9090/api/v1/query?query=count(kube_pod_status_phase{phase=\"Running\"})"' 2>&1 | python3 -c '
import sys, json
d = json.load(sys.stdin)
r = d["data"]["result"]
print("Running pods:", r[0]["value"][1]) if r else print("EMPTY")
'
```

Expected: `Running pods: 27` (or close — matches `kubectl get pods --all-namespaces --no-headers | wc -l`).

- [ ] **Step 10: Verify Prometheus PVC is in use**

```bash
ssh PC@100.79.113.84 'kubectl -n monitoring exec deploy/prometheus -- df -h /prometheus'
```

Expected: a mounted filesystem with ~10G size (not an emptyDir overlay).

- [ ] **Step 11: Verify node-exporter data**

```bash
ssh PC@100.79.113.84 'kubectl -n monitoring exec deploy/prometheus -- wget -qO- "http://localhost:9090/api/v1/query?query=node_memory_MemTotal_bytes"' 2>&1 | python3 -c '
import sys, json
d = json.load(sys.stdin)
r = d["data"]["result"]
if r:
    gb = float(r[0]["value"][1]) / 1024**3
    print(f"Node memory: {gb:.1f} GiB")
else:
    print("EMPTY")
'
```

Expected: a non-zero GiB value matching the Minikube VM's allocated memory.

---

## Acceptance criteria

All must be true:

1. `kubectl get pods -n monitoring` shows 4 pods, all `Running`.
2. Prometheus targets page shows `prometheus`, `windows`, `nvidia-gpu`, `kube-state-metrics`, `node-exporter` all `up`.
3. PromQL `count(kube_pod_status_phase{phase="Running"})` returns a count matching actual running pods.
4. PromQL `node_memory_MemTotal_bytes` returns a non-zero value.
5. PromQL `nvidia_smi_memory_used_bytes` still returns data (didn't break Stage A).
6. `kubectl get pvc -n monitoring` shows `prometheus-data` as `Bound`.
7. Prometheus pod restart doesn't lose metric history (can verify by checking `prometheus_tsdb_head_series` after a manual restart, but optional).

---

## Rollback

If Prometheus fails to start after changes:

```bash
# Revert to the original deployment (no PVC, no SA)
kubectl delete pvc prometheus-data -n monitoring  # loses data
kubectl delete clusterrolebinding prometheus
kubectl delete clusterrole prometheus
kubectl delete sa prometheus -n monitoring
# Re-apply the original deployment YAML from git (main branch)
```

If kube-state-metrics or node-exporter cause issues, they can be deleted independently without affecting Prometheus:

```bash
kubectl delete deployment kube-state-metrics -n monitoring
kubectl delete daemonset node-exporter -n monitoring
kubectl delete service kube-state-metrics node-exporter -n monitoring
```
