# RabbitMQ Broker Metrics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose RabbitMQ broker queue depth and consumer metrics to Prometheus for production and QA virtual hosts.

**Architecture:** Enable RabbitMQ's built-in `rabbitmq_prometheus` plugin through the existing repo-managed ConfigMap, expose plugin port `15692` on the broker pod and Service, then add a dedicated Prometheus scrape job for `/metrics/detailed`. QA continues using the production RabbitMQ broker through its `ExternalName` Service, so prod/QA separation comes from RabbitMQ's `vhost` metric label.

**Tech Stack:** Kubernetes manifests, RabbitMQ `3-management-alpine`, RabbitMQ Prometheus plugin, Prometheus static scrape config, Kustomize/YAML validation.

---

## File Structure

- Modify `java/k8s/configmaps/rabbitmq-definitions.yml`: add `enabled_plugins` data with `rabbitmq_management` and `rabbitmq_prometheus`.
- Modify `java/k8s/deployments/rabbitmq.yml`: mount `enabled_plugins` at `/etc/rabbitmq/enabled_plugins` and expose container port `15692`.
- Modify `java/k8s/services/rabbitmq.yml`: add Service port `15692` named `prometheus`.
- Modify `k8s/monitoring/configmaps/prometheus-config.yml`: add a dedicated `rabbitmq` scrape job using `/metrics/detailed` with only queue coarse metrics and queue consumer count.

## Task 1: Enable RabbitMQ Prometheus Plugin

**Files:**
- Modify: `java/k8s/configmaps/rabbitmq-definitions.yml`
- Modify: `java/k8s/deployments/rabbitmq.yml`

- [ ] **Step 1: Add `enabled_plugins` to the RabbitMQ ConfigMap**

Edit `java/k8s/configmaps/rabbitmq-definitions.yml` so the `data:` section ends with:

```yaml
  rabbitmq.conf: |
    management.load_definitions = /etc/rabbitmq/definitions.json
  enabled_plugins: |
    [rabbitmq_management,rabbitmq_prometheus].
```

- [ ] **Step 2: Expose the plugin port on the RabbitMQ container**

Edit `java/k8s/deployments/rabbitmq.yml` so the `ports:` section is:

```yaml
          ports:
            - name: amqp
              containerPort: 5672
            - name: management
              containerPort: 15672
            - name: prometheus
              containerPort: 15692
```

- [ ] **Step 3: Mount `enabled_plugins` into RabbitMQ**

Edit `java/k8s/deployments/rabbitmq.yml` so `volumeMounts:` includes:

```yaml
            - name: definitions
              mountPath: /etc/rabbitmq/enabled_plugins
              subPath: enabled_plugins
```

- [ ] **Step 4: Validate YAML parses**

Run:

```bash
python3 - <<'PY'
from pathlib import Path
import yaml

for path in [
    "java/k8s/configmaps/rabbitmq-definitions.yml",
    "java/k8s/deployments/rabbitmq.yml",
]:
    with Path(path).open() as fh:
        list(yaml.safe_load_all(fh))
    print(f"ok {path}")
PY
```

Expected output:

```text
ok java/k8s/configmaps/rabbitmq-definitions.yml
ok java/k8s/deployments/rabbitmq.yml
```

## Task 2: Expose RabbitMQ Metrics Service Port

**Files:**
- Modify: `java/k8s/services/rabbitmq.yml`

- [ ] **Step 1: Add the Prometheus Service port**

Edit `java/k8s/services/rabbitmq.yml` so `ports:` includes:

```yaml
    - name: prometheus
      port: 15692
      targetPort: 15692
```

- [ ] **Step 2: Validate YAML parses**

Run:

```bash
python3 - <<'PY'
from pathlib import Path
import yaml

path = "java/k8s/services/rabbitmq.yml"
with Path(path).open() as fh:
    list(yaml.safe_load_all(fh))
print(f"ok {path}")
PY
```

Expected output:

```text
ok java/k8s/services/rabbitmq.yml
```

## Task 3: Add Prometheus RabbitMQ Scrape Job

**Files:**
- Modify: `k8s/monitoring/configmaps/prometheus-config.yml`

- [ ] **Step 1: Add a dedicated RabbitMQ scrape job**

Edit `k8s/monitoring/configmaps/prometheus-config.yml` and add this job under `scrape_configs:` before the generic `k8s-pods` job:

```yaml
      - job_name: "rabbitmq"
        metrics_path: /metrics/detailed
        params:
          family:
            - queue_coarse_metrics
            - queue_consumer_count
        static_configs:
          - targets: ["rabbitmq.java-tasks.svc.cluster.local:15692"]
```

- [ ] **Step 2: Validate YAML parses**

Run:

```bash
python3 - <<'PY'
from pathlib import Path
import yaml

path = "k8s/monitoring/configmaps/prometheus-config.yml"
with Path(path).open() as fh:
    list(yaml.safe_load_all(fh))
print(f"ok {path}")
PY
```

Expected output:

```text
ok k8s/monitoring/configmaps/prometheus-config.yml
```

## Task 4: Render Kustomize Targets

**Files:**
- Verify: `java/k8s/kustomization.yaml`
- Verify: `k8s/monitoring/kustomization.yaml`
- Verify: `k8s/overlays/qa-java/kustomization.yaml`

- [ ] **Step 1: Render affected bases and overlay**

Run:

```bash
kubectl kustomize java/k8s >/tmp/rabbitmq-java-kustomize.yml
kubectl kustomize k8s/monitoring >/tmp/rabbitmq-monitoring-kustomize.yml
kubectl kustomize k8s/overlays/qa-java >/tmp/rabbitmq-qa-java-kustomize.yml
```

Expected: all commands exit `0`.

- [ ] **Step 2: Confirm rendered metrics wiring**

Run:

```bash
rg -n "enabled_plugins|rabbitmq_prometheus|containerPort: 15692|name: prometheus|job_name: \"rabbitmq\"|rabbitmq.java-tasks.svc.cluster.local:15692" \
  /tmp/rabbitmq-java-kustomize.yml \
  /tmp/rabbitmq-monitoring-kustomize.yml
```

Expected: output includes the plugin file, port `15692`, Service port name `prometheus`, RabbitMQ scrape job, and target `rabbitmq.java-tasks.svc.cluster.local:15692`.

- [ ] **Step 3: Confirm QA still points to the shared broker**

Run:

```bash
rg -n "kind: ExternalName|externalName: rabbitmq.java-tasks.svc.cluster.local|name: rabbitmq" /tmp/rabbitmq-qa-java-kustomize.yml
```

Expected: output shows the QA RabbitMQ Service remains an `ExternalName` for `rabbitmq.java-tasks.svc.cluster.local`.

## Task 5: Final Preflight

**Files:**
- Verify: Kubernetes manifest set

- [ ] **Step 1: Run the relevant local preflight**

Run:

```bash
make preflight
```

Expected: PASS. If the full preflight is blocked by local environment limits, report the blocker and include the focused YAML and Kustomize validation results.

- [ ] **Step 2: Commit when working on an allowed branch**

On feature branches or `qa`, run:

```bash
git add \
  docs/superpowers/plans/2026-05-07-rabbitmq-broker-metrics.md \
  java/k8s/configmaps/rabbitmq-definitions.yml \
  java/k8s/deployments/rabbitmq.yml \
  java/k8s/services/rabbitmq.yml \
  k8s/monitoring/configmaps/prometheus-config.yml
git commit -m "feat: expose rabbitmq broker metrics"
```

On `main`, do not push autonomously. Leave changes local unless Kyle explicitly directs the main-branch workflow.

## Self-Review

- Spec coverage: the plan enables `rabbitmq_prometheus`, exposes port `15692`, adds the RabbitMQ Service port, adds `/metrics/detailed` scraping with `queue_coarse_metrics` and `queue_consumer_count`, and preserves QA shared-broker behavior.
- Placeholder scan: no `TBD`, vague edge-case instructions, or missing commands remain.
- Type/name consistency: file paths, port names, metric families, and scrape target names match the Phase 1 spec.
