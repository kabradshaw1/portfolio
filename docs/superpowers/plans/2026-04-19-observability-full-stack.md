# Full-Stack Observability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a comprehensive observability stack completing the three pillars (metrics, logs, traces), adding K8s health and SLO alerts, Kafka consumer lag monitoring, a correlation dashboard, and learning documentation.

**Architecture:** Deploy Loki + Promtail for log aggregation alongside existing Prometheus + Grafana + Jaeger. Add traceID to Go service logs for log-to-trace correlation. Expose Kafka consumer lag as Prometheus metrics. All alerts route to existing Telegram contact point.

**Tech Stack:** Loki 3.0, Promtail 3.0, Grafana alerting YAML, Prometheus client_golang, segmentio/kafka-go, Go slog, Spring Boot Logback

---

## File Structure

### New files (K8s monitoring)
- `k8s/monitoring/statefulsets/loki.yml` — Loki StatefulSet (single-binary mode)
- `k8s/monitoring/configmaps/loki-config.yml` — Loki storage and retention config
- `k8s/monitoring/configmaps/promtail-config.yml` — Promtail scrape and pipeline config
- `k8s/monitoring/daemonsets/promtail.yml` — Promtail DaemonSet
- `k8s/monitoring/services/loki.yml` — Loki ClusterIP service
- `k8s/monitoring/pvc/loki-data.yml` — Loki persistent volume claim
- `k8s/monitoring/rbac/promtail-clusterrole.yml` — Promtail RBAC for pod log access
- `k8s/monitoring/rbac/promtail-clusterrolebinding.yml` — Promtail RBAC binding
- `k8s/monitoring/rbac/promtail-serviceaccount.yml` — Promtail service account

### Modified files (K8s monitoring)
- `k8s/monitoring/kustomization.yaml` — Add all new resources
- `k8s/monitoring/configmaps/grafana-datasource.yml` — Add Loki datasource with Jaeger derived fields
- `k8s/monitoring/configmaps/grafana-alerting.yml` — Add K8s Health + Application SLOs + Kafka alert groups
- `k8s/monitoring/configmaps/grafana-dashboards.yml` — Add Streaming Analytics panels + Observability Overview dashboard
- `k8s/monitoring/deployments/grafana.yml` — Mount Loki datasource file

### Modified files (Go services)
- `go/pkg/tracing/logging.go` — New: slog handler wrapper that injects traceID
- `go/ecommerce-service/internal/middleware/logging.go` — Add traceID extraction from OTel span context
- `go/ecommerce-service/cmd/server/main.go` — Configure slog with traceID handler
- `go/ai-service/cmd/server/main.go` — Configure slog with traceID handler
- `go/analytics-service/cmd/server/main.go` — Configure slog with traceID handler
- `go/auth-service/cmd/server/main.go` — Configure slog with traceID handler
- `go/analytics-service/internal/metrics/prometheus.go` — Add Kafka consumer lag gauge
- `go/analytics-service/internal/consumer/consumer.go` — Record lag metrics from reader stats

### New files (Java services)
- `java/task-service/src/main/resources/logback-spring.xml` — JSON structured logging with traceID
- `java/activity-service/src/main/resources/logback-spring.xml` — Same
- `java/notification-service/src/main/resources/logback-spring.xml` — Same
- `java/gateway-service/src/main/resources/logback-spring.xml` — Same

### New files (Learning docs)
- `docs/adr/observability/01-three-pillars.md`
- `docs/adr/observability/02-prometheus-and-metrics.md`
- `docs/adr/observability/03-loki-and-logs.md`
- `docs/adr/observability/04-jaeger-and-traces.md`
- `docs/adr/observability/05-alerting-and-slos.md`
- `docs/adr/observability/06-correlation.md`

---

## Task 1: Deploy Loki

**Files:**
- Create: `k8s/monitoring/configmaps/loki-config.yml`
- Create: `k8s/monitoring/statefulsets/loki.yml`
- Create: `k8s/monitoring/services/loki.yml`
- Create: `k8s/monitoring/pvc/loki-data.yml`
- Modify: `k8s/monitoring/kustomization.yaml`

- [ ] **Step 1: Create Loki config ConfigMap**

```yaml
# k8s/monitoring/configmaps/loki-config.yml
apiVersion: v1
kind: ConfigMap
metadata:
  name: loki-config
  namespace: monitoring
data:
  loki.yaml: |
    auth_enabled: false

    server:
      http_listen_port: 3100
      grpc_listen_port: 9096
      log_level: warn

    common:
      path_prefix: /loki
      storage:
        filesystem:
          chunks_directory: /loki/chunks
          rules_directory: /loki/rules
      replication_factor: 1
      ring:
        instance_addr: 127.0.0.1
        kvstore:
          store: inmemory

    schema_config:
      configs:
        - from: "2024-01-01"
          store: tsdb
          object_store: filesystem
          schema: v13
          index:
            prefix: index_
            period: 24h

    limits_config:
      retention_period: 168h
      max_query_series: 500
      max_query_parallelism: 2

    compactor:
      working_directory: /loki/compactor
      compaction_interval: 10m
      retention_enabled: true
      retention_delete_delay: 2h
      delete_request_store: filesystem
```

- [ ] **Step 2: Create Loki PVC**

```yaml
# k8s/monitoring/pvc/loki-data.yml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: loki-data
  namespace: monitoring
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 5Gi
```

- [ ] **Step 3: Create Loki StatefulSet**

```yaml
# k8s/monitoring/statefulsets/loki.yml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: loki
  namespace: monitoring
spec:
  replicas: 1
  serviceName: loki
  selector:
    matchLabels:
      app: loki
  template:
    metadata:
      labels:
        app: loki
    spec:
      containers:
        - name: loki
          image: grafana/loki:3.0.0
          args:
            - -config.file=/etc/loki/loki.yaml
          ports:
            - containerPort: 3100
              name: http
            - containerPort: 9096
              name: grpc
          volumeMounts:
            - name: config
              mountPath: /etc/loki
            - name: data
              mountPath: /loki
          resources:
            requests:
              memory: "128Mi"
              cpu: "100m"
            limits:
              memory: "512Mi"
              cpu: "500m"
          readinessProbe:
            httpGet:
              path: /ready
              port: 3100
            initialDelaySeconds: 15
            periodSeconds: 10
          livenessProbe:
            httpGet:
              path: /ready
              port: 3100
            initialDelaySeconds: 30
            periodSeconds: 30
      volumes:
        - name: config
          configMap:
            name: loki-config
        - name: data
          persistentVolumeClaim:
            claimName: loki-data
```

- [ ] **Step 4: Create Loki service**

```yaml
# k8s/monitoring/services/loki.yml
apiVersion: v1
kind: Service
metadata:
  name: loki
  namespace: monitoring
spec:
  selector:
    app: loki
  ports:
    - port: 3100
      targetPort: 3100
      name: http
  type: ClusterIP
```

- [ ] **Step 5: Add resources to kustomization.yaml**

Add to `k8s/monitoring/kustomization.yaml` resources list:

```yaml
  - pvc/loki-data.yml
  - configmaps/loki-config.yml
  - statefulsets/loki.yml
  - services/loki.yml
```

- [ ] **Step 6: Commit**

```bash
git add k8s/monitoring/configmaps/loki-config.yml k8s/monitoring/statefulsets/loki.yml k8s/monitoring/services/loki.yml k8s/monitoring/pvc/loki-data.yml k8s/monitoring/kustomization.yaml
git commit -m "feat(monitoring): add Loki deployment for log aggregation"
```

---

## Task 2: Deploy Promtail

**Files:**
- Create: `k8s/monitoring/rbac/promtail-serviceaccount.yml`
- Create: `k8s/monitoring/rbac/promtail-clusterrole.yml`
- Create: `k8s/monitoring/rbac/promtail-clusterrolebinding.yml`
- Create: `k8s/monitoring/configmaps/promtail-config.yml`
- Create: `k8s/monitoring/daemonsets/promtail.yml`
- Modify: `k8s/monitoring/kustomization.yaml`

- [ ] **Step 1: Create Promtail RBAC**

```yaml
# k8s/monitoring/rbac/promtail-serviceaccount.yml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: promtail
  namespace: monitoring
```

```yaml
# k8s/monitoring/rbac/promtail-clusterrole.yml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: promtail
rules:
  - apiGroups: [""]
    resources: ["nodes", "nodes/proxy", "services", "endpoints", "pods"]
    verbs: ["get", "watch", "list"]
```

```yaml
# k8s/monitoring/rbac/promtail-clusterrolebinding.yml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: promtail
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: promtail
subjects:
  - kind: ServiceAccount
    name: promtail
    namespace: monitoring
```

- [ ] **Step 2: Create Promtail config**

```yaml
# k8s/monitoring/configmaps/promtail-config.yml
apiVersion: v1
kind: ConfigMap
metadata:
  name: promtail-config
  namespace: monitoring
data:
  promtail.yaml: |
    server:
      http_listen_port: 3101
      grpc_listen_port: 0
      log_level: warn

    positions:
      filename: /tmp/positions.yaml

    clients:
      - url: http://loki.monitoring.svc.cluster.local:3100/loki/api/v1/push

    scrape_configs:
      - job_name: kubernetes-pods
        kubernetes_sd_configs:
          - role: pod
        pipeline_stages:
          - cri: {}
          - json:
              expressions:
                level: level
                msg: msg
                traceID: traceID
          - labels:
              level:
          - output:
              source: msg
        relabel_configs:
          - source_labels: [__meta_kubernetes_pod_label_app]
            target_label: app
          - source_labels: [__meta_kubernetes_namespace]
            target_label: namespace
          - source_labels: [__meta_kubernetes_pod_name]
            target_label: pod
          - source_labels: [__meta_kubernetes_container_name]
            target_label: container
```

- [ ] **Step 3: Create Promtail DaemonSet**

```yaml
# k8s/monitoring/daemonsets/promtail.yml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: promtail
  namespace: monitoring
spec:
  selector:
    matchLabels:
      app: promtail
  template:
    metadata:
      labels:
        app: promtail
    spec:
      serviceAccountName: promtail
      containers:
        - name: promtail
          image: grafana/promtail:3.0.0
          args:
            - -config.file=/etc/promtail/promtail.yaml
          volumeMounts:
            - name: config
              mountPath: /etc/promtail
            - name: pods
              mountPath: /var/log/pods
              readOnly: true
            - name: containers
              mountPath: /var/lib/docker/containers
              readOnly: true
          resources:
            requests:
              memory: "64Mi"
              cpu: "50m"
            limits:
              memory: "128Mi"
              cpu: "200m"
      tolerations:
        - effect: NoSchedule
          operator: Exists
      volumes:
        - name: config
          configMap:
            name: promtail-config
        - name: pods
          hostPath:
            path: /var/log/pods
        - name: containers
          hostPath:
            path: /var/lib/docker/containers
```

- [ ] **Step 4: Add resources to kustomization.yaml**

Add to `k8s/monitoring/kustomization.yaml`:

```yaml
  - rbac/promtail-serviceaccount.yml
  - rbac/promtail-clusterrole.yml
  - rbac/promtail-clusterrolebinding.yml
  - configmaps/promtail-config.yml
  - daemonsets/promtail.yml
```

- [ ] **Step 5: Commit**

```bash
git add k8s/monitoring/rbac/promtail-serviceaccount.yml k8s/monitoring/rbac/promtail-clusterrole.yml k8s/monitoring/rbac/promtail-clusterrolebinding.yml k8s/monitoring/configmaps/promtail-config.yml k8s/monitoring/daemonsets/promtail.yml k8s/monitoring/kustomization.yaml
git commit -m "feat(monitoring): add Promtail DaemonSet for container log collection"
```

---

## Task 3: Add Loki Datasource to Grafana

**Files:**
- Modify: `k8s/monitoring/configmaps/grafana-datasource.yml`
- Modify: `k8s/monitoring/deployments/grafana.yml`

- [ ] **Step 1: Add Loki and Jaeger datasources to grafana-datasource.yml**

Replace the contents of `k8s/monitoring/configmaps/grafana-datasource.yml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: grafana-datasource
  namespace: monitoring
data:
  datasources.yml: |
    apiVersion: 1
    datasources:
      - name: Prometheus
        type: prometheus
        access: proxy
        url: http://prometheus.monitoring.svc.cluster.local:9090
        isDefault: true
        editable: false
        uid: PBFA97CFB590B2093
      - name: Loki
        type: loki
        access: proxy
        url: http://loki.monitoring.svc.cluster.local:3100
        editable: false
        uid: loki
        jsonData:
          derivedFields:
            - datasourceUid: jaeger
              matcherRegex: '"traceID":"([a-f0-9]+)"'
              name: TraceID
              url: "$${__value.raw}"
              urlDisplayLabel: View Trace
      - name: Jaeger
        type: jaeger
        access: proxy
        url: http://jaeger.monitoring.svc.cluster.local:16686
        editable: false
        uid: jaeger
```

- [ ] **Step 2: Update Grafana deployment volume mount**

The datasource file key changed from `prometheus.yml` to `datasources.yml`. Update the volume mount in `k8s/monitoring/deployments/grafana.yml`:

Change:
```yaml
            - name: datasource
              mountPath: /etc/grafana/provisioning/datasources/prometheus.yml
              subPath: prometheus.yml
```
To:
```yaml
            - name: datasource
              mountPath: /etc/grafana/provisioning/datasources/datasources.yml
              subPath: datasources.yml
```

- [ ] **Step 3: Commit**

```bash
git add k8s/monitoring/configmaps/grafana-datasource.yml k8s/monitoring/deployments/grafana.yml
git commit -m "feat(monitoring): add Loki and Jaeger datasources to Grafana with trace correlation"
```

---

## Task 4: Add Kubernetes Health Alert Rules

**Files:**
- Modify: `k8s/monitoring/configmaps/grafana-alerting.yml`

- [ ] **Step 1: Add Kubernetes Health alert group**

Add a new group after the existing "Infrastructure" group in `k8s/monitoring/configmaps/grafana-alerting.yml`. Insert before the final closing of the `groups:` list. Each rule follows the same 3-query pattern (A: PromQL, B: reduce/last, C: threshold) as the existing rules.

```yaml
      - orgId: 1
        name: Kubernetes Health
        folder: Infrastructure Alerts
        interval: 1m
        rules:
          - uid: container-oom-killed
            title: Container OOM Killed
            condition: C
            data:
              - refId: A
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: PBFA97CFB590B2093
                model:
                  expr: kube_pod_container_status_terminated_reason{reason="OOMKilled"}
                  instant: true
                  refId: A
              - refId: B
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: reduce
                  expression: A
                  reducer: last
                  refId: B
              - refId: C
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: threshold
                  expression: B
                  conditions:
                    - evaluator:
                        type: gt
                        params:
                          - 0
                  refId: C
            for: 0s
            labels:
              severity: critical
            annotations:
              summary: "Container {{ $labels.container }} in pod {{ $labels.pod }} was OOM killed"

          - uid: pod-restart-storm
            title: Pod Restart Storm
            condition: C
            data:
              - refId: A
                relativeTimeRange:
                  from: 1800
                  to: 0
                datasourceUid: PBFA97CFB590B2093
                model:
                  expr: increase(kube_pod_container_status_restarts_total[30m])
                  instant: true
                  refId: A
              - refId: B
                relativeTimeRange:
                  from: 1800
                  to: 0
                datasourceUid: __expr__
                model:
                  type: reduce
                  expression: A
                  reducer: last
                  refId: B
              - refId: C
                relativeTimeRange:
                  from: 1800
                  to: 0
                datasourceUid: __expr__
                model:
                  type: threshold
                  expression: B
                  conditions:
                    - evaluator:
                        type: gt
                        params:
                          - 3
                  refId: C
            for: 5m
            labels:
              severity: warning
            annotations:
              summary: "Pod {{ $labels.pod }} has restarted more than 3 times in 30 minutes"

          - uid: container-memory-high
            title: Container Memory High
            condition: C
            data:
              - refId: A
                relativeTimeRange:
                  from: 600
                  to: 0
                datasourceUid: PBFA97CFB590B2093
                model:
                  expr: >-
                    max by (pod, container, namespace) (
                      container_memory_working_set_bytes{container!=""}
                      / on(pod, container, namespace)
                      kube_pod_container_resource_limits{resource="memory"}
                    )
                  instant: true
                  refId: A
              - refId: B
                relativeTimeRange:
                  from: 600
                  to: 0
                datasourceUid: __expr__
                model:
                  type: reduce
                  expression: A
                  reducer: last
                  refId: B
              - refId: C
                relativeTimeRange:
                  from: 600
                  to: 0
                datasourceUid: __expr__
                model:
                  type: threshold
                  expression: B
                  conditions:
                    - evaluator:
                        type: gt
                        params:
                          - 0.85
                  refId: C
            for: 10m
            labels:
              severity: warning
            annotations:
              summary: "Container {{ $labels.container }} in {{ $labels.namespace }}/{{ $labels.pod }} is using >85% of memory limit"

          - uid: node-memory-pressure
            title: Node Memory Pressure
            condition: C
            data:
              - refId: A
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: PBFA97CFB590B2093
                model:
                  expr: kube_node_status_condition{condition="MemoryPressure",status="true"}
                  instant: true
                  refId: A
              - refId: B
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: reduce
                  expression: A
                  reducer: last
                  refId: B
              - refId: C
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: threshold
                  expression: B
                  conditions:
                    - evaluator:
                        type: gt
                        params:
                          - 0
                  refId: C
            for: 2m
            labels:
              severity: critical
            annotations:
              summary: "Node {{ $labels.node }} is under memory pressure"

          - uid: node-disk-pressure
            title: Node Disk Pressure
            condition: C
            data:
              - refId: A
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: PBFA97CFB590B2093
                model:
                  expr: kube_node_status_condition{condition="DiskPressure",status="true"}
                  instant: true
                  refId: A
              - refId: B
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: reduce
                  expression: A
                  reducer: last
                  refId: B
              - refId: C
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: threshold
                  expression: B
                  conditions:
                    - evaluator:
                        type: gt
                        params:
                          - 0
                  refId: C
            for: 2m
            labels:
              severity: critical
            annotations:
              summary: "Node {{ $labels.node }} is under disk pressure"

          - uid: deployment-replicas-unavailable
            title: Deployment Replicas Unavailable
            condition: C
            data:
              - refId: A
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: PBFA97CFB590B2093
                model:
                  expr: >-
                    kube_deployment_spec_replicas
                    - kube_deployment_status_replicas_available
                  instant: true
                  refId: A
              - refId: B
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: reduce
                  expression: A
                  reducer: last
                  refId: B
              - refId: C
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: threshold
                  expression: B
                  conditions:
                    - evaluator:
                        type: gt
                        params:
                          - 0
                  refId: C
            for: 5m
            labels:
              severity: warning
            annotations:
              summary: "Deployment {{ $labels.deployment }} in {{ $labels.namespace }} has unavailable replicas"
```

- [ ] **Step 2: Commit**

```bash
git add k8s/monitoring/configmaps/grafana-alerting.yml
git commit -m "feat(monitoring): add Kubernetes health alert rules (OOM, restarts, memory/disk pressure)"
```

---

## Task 5: Add Application SLO Alert Rules

**Files:**
- Modify: `k8s/monitoring/configmaps/grafana-alerting.yml`

- [ ] **Step 1: Add Application SLOs alert group**

Add after the Kubernetes Health group in `grafana-alerting.yml`:

```yaml
      - orgId: 1
        name: Application SLOs
        folder: Infrastructure Alerts
        interval: 1m
        rules:
          - uid: go-ai-error-rate
            title: Go AI Service Error Rate High
            condition: C
            data:
              - refId: A
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: PBFA97CFB590B2093
                model:
                  expr: >-
                    sum(rate(http_requests_total{service="go-ai-service",status=~"5.."}[5m]))
                    / sum(rate(http_requests_total{service="go-ai-service"}[5m]))
                    * 100
                  instant: true
                  refId: A
              - refId: B
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: reduce
                  expression: A
                  reducer: last
                  refId: B
              - refId: C
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: threshold
                  expression: B
                  conditions:
                    - evaluator:
                        type: gt
                        params:
                          - 5
                  refId: C
            for: 5m
            labels:
              severity: warning
            annotations:
              summary: "Go AI service error rate is above 5%"

          - uid: go-ecommerce-error-rate
            title: Go Ecommerce Error Rate High
            condition: C
            data:
              - refId: A
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: PBFA97CFB590B2093
                model:
                  expr: >-
                    sum(rate(http_requests_total{service="go-ecommerce-service",status=~"5.."}[5m]))
                    / sum(rate(http_requests_total{service="go-ecommerce-service"}[5m]))
                    * 100
                  instant: true
                  refId: A
              - refId: B
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: reduce
                  expression: A
                  reducer: last
                  refId: B
              - refId: C
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: threshold
                  expression: B
                  conditions:
                    - evaluator:
                        type: gt
                        params:
                          - 2
                  refId: C
            for: 5m
            labels:
              severity: warning
            annotations:
              summary: "Go ecommerce service error rate is above 2%"

          - uid: java-gateway-error-rate
            title: Java Gateway Error Rate High
            condition: C
            data:
              - refId: A
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: PBFA97CFB590B2093
                model:
                  expr: >-
                    sum(rate(http_server_requests_seconds_count{namespace="java-tasks",status=~"5.."}[5m]))
                    / sum(rate(http_server_requests_seconds_count{namespace="java-tasks"}[5m]))
                    * 100
                  instant: true
                  refId: A
              - refId: B
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: reduce
                  expression: A
                  reducer: last
                  refId: B
              - refId: C
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: threshold
                  expression: B
                  conditions:
                    - evaluator:
                        type: gt
                        params:
                          - 2
                  refId: C
            for: 5m
            labels:
              severity: warning
            annotations:
              summary: "Java gateway error rate is above 2%"

          - uid: go-ai-latency-high
            title: Go AI Service Latency High
            condition: C
            data:
              - refId: A
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: PBFA97CFB590B2093
                model:
                  expr: >-
                    histogram_quantile(0.95,
                      sum(rate(http_request_duration_seconds_bucket{service="go-ai-service"}[5m])) by (le)
                    )
                  instant: true
                  refId: A
              - refId: B
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: reduce
                  expression: A
                  reducer: last
                  refId: B
              - refId: C
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: threshold
                  expression: B
                  conditions:
                    - evaluator:
                        type: gt
                        params:
                          - 30
                  refId: C
            for: 5m
            labels:
              severity: warning
            annotations:
              summary: "Go AI service p95 latency is above 30s"

          - uid: go-ecommerce-latency-high
            title: Go Ecommerce Latency High
            condition: C
            data:
              - refId: A
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: PBFA97CFB590B2093
                model:
                  expr: >-
                    histogram_quantile(0.95,
                      sum(rate(http_request_duration_seconds_bucket{service="go-ecommerce-service"}[5m])) by (le)
                    )
                  instant: true
                  refId: A
              - refId: B
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: reduce
                  expression: A
                  reducer: last
                  refId: B
              - refId: C
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: threshold
                  expression: B
                  conditions:
                    - evaluator:
                        type: gt
                        params:
                          - 2
                  refId: C
            for: 5m
            labels:
              severity: warning
            annotations:
              summary: "Go ecommerce service p95 latency is above 2s"

          - uid: java-gateway-latency-high
            title: Java Gateway Latency High
            condition: C
            data:
              - refId: A
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: PBFA97CFB590B2093
                model:
                  expr: >-
                    histogram_quantile(0.95,
                      sum(rate(http_server_requests_seconds_bucket{namespace="java-tasks"}[5m])) by (le)
                    )
                  instant: true
                  refId: A
              - refId: B
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: reduce
                  expression: A
                  reducer: last
                  refId: B
              - refId: C
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: threshold
                  expression: B
                  conditions:
                    - evaluator:
                        type: gt
                        params:
                          - 3
                  refId: C
            for: 5m
            labels:
              severity: warning
            annotations:
              summary: "Java gateway p95 latency is above 3s"
```

- [ ] **Step 2: Commit**

```bash
git add k8s/monitoring/configmaps/grafana-alerting.yml
git commit -m "feat(monitoring): add application SLO alert rules (error rate + latency)"
```

---

## Task 6: Add TraceID to Go Service Logs

**Files:**
- Create: `go/pkg/tracing/logging.go`
- Modify: `go/ecommerce-service/internal/middleware/logging.go`
- Modify: `go/ecommerce-service/cmd/server/main.go`
- Modify: `go/ai-service/cmd/server/main.go`
- Modify: `go/analytics-service/cmd/server/main.go`
- Modify: `go/auth-service/cmd/server/main.go`

- [ ] **Step 1: Create tracing/logging.go in go/pkg**

This handler wrapper extracts the traceID from the OTel span context and adds it to every slog record.

```go
// go/pkg/tracing/logging.go
package tracing

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// LogHandler wraps an slog.Handler to inject traceID from OpenTelemetry span context.
type LogHandler struct {
	slog.Handler
}

// NewLogHandler creates a handler that adds traceID to log records when a span is active.
func NewLogHandler(h slog.Handler) *LogHandler {
	return &LogHandler{Handler: h}
}

// Handle adds the traceID attribute if a valid span context exists.
func (h *LogHandler) Handle(ctx context.Context, r slog.Record) error {
	sc := trace.SpanContextFromContext(ctx)
	if sc.HasTraceID() {
		r.AddAttrs(slog.String("traceID", sc.TraceID().String()))
	}
	return h.Handler.Handle(ctx, r)
}

// WithAttrs returns a new LogHandler wrapping the inner handler's WithAttrs.
func (h *LogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &LogHandler{Handler: h.Handler.WithAttrs(attrs)}
}

// WithGroup returns a new LogHandler wrapping the inner handler's WithGroup.
func (h *LogHandler) WithGroup(name string) slog.Handler {
	return &LogHandler{Handler: h.Handler.WithGroup(name)}
}
```

- [ ] **Step 2: Run `go mod tidy` in go/pkg**

```bash
cd go/pkg && go mod tidy
```

Verify `go.opentelemetry.io/otel/trace` is already in go.mod (it should be since tracing.go uses it).

- [ ] **Step 3: Add traceID to ecommerce logging middleware**

Modify `go/ecommerce-service/internal/middleware/logging.go` — add traceID from OTel span context:

```go
package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

func Logging() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := uuid.New().String()
		c.Set("requestId", requestID)
		c.Header("X-Request-ID", requestID)
		start := time.Now()
		c.Next()
		latency := time.Since(start)

		attrs := []any{
			"requestId", requestID,
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"latency", latency.String(),
			"ip", c.ClientIP(),
			"userId", c.GetString("userId"),
		}

		sc := trace.SpanContextFromContext(c.Request.Context())
		if sc.HasTraceID() {
			attrs = append(attrs, "traceID", sc.TraceID().String())
		}

		slog.InfoContext(c.Request.Context(), "request", attrs...)
	}
}
```

- [ ] **Step 4: Configure slog with traceID handler in each service main.go**

For each of the four Go services, add the traceID-aware log handler after OTel init. Add this after the tracing init block and before the server/consumer setup:

```go
// Set up structured JSON logging with traceID injection.
slog.SetDefault(slog.New(
    tracing.NewLogHandler(slog.NewJSONHandler(os.Stdout, nil)),
))
```

Add the import `"github.com/kabradshaw1/portfolio/go/pkg/tracing"` and `"os"` if not already present.

Files to modify:
- `go/ecommerce-service/cmd/server/main.go`
- `go/ai-service/cmd/server/main.go`
- `go/analytics-service/cmd/server/main.go`
- `go/auth-service/cmd/server/main.go`

- [ ] **Step 5: Run go mod tidy in all service directories**

```bash
cd go/pkg && go mod tidy
cd go/ecommerce-service && go mod tidy
cd go/ai-service && go mod tidy
cd go/analytics-service && go mod tidy
cd go/auth-service && go mod tidy
```

- [ ] **Step 6: Run tests**

```bash
make preflight-go
```

Expected: All lint and tests pass.

- [ ] **Step 7: Commit**

```bash
git add go/pkg/tracing/logging.go go/ecommerce-service/internal/middleware/logging.go go/ecommerce-service/cmd/server/main.go go/ai-service/cmd/server/main.go go/analytics-service/cmd/server/main.go go/auth-service/cmd/server/main.go
git commit -m "feat(go): add traceID injection to structured logs for Loki-Jaeger correlation"
```

---

## Task 7: Add Structured JSON Logging to Java Services

**Files:**
- Create: `java/task-service/src/main/resources/logback-spring.xml`
- Create: `java/activity-service/src/main/resources/logback-spring.xml`
- Create: `java/notification-service/src/main/resources/logback-spring.xml`
- Create: `java/gateway-service/src/main/resources/logback-spring.xml`

- [ ] **Step 1: Add logstash-logback-encoder dependency**

Add to `java/build.gradle` (root project) or each service's `build.gradle` in the `dependencies` block:

```groovy
implementation 'net.logstash.logback:logstash-logback-encoder:7.4'
```

Check the existing build.gradle structure to determine whether dependencies are declared at root or per-service level.

- [ ] **Step 2: Create logback-spring.xml for each Java service**

Create the same file in all four services. This configures JSON output with Spring's traceID (Micrometer Tracing / Spring Boot default MDC):

```xml
<?xml version="1.0" encoding="UTF-8"?>
<configuration>
    <appender name="JSON" class="ch.qos.logback.core.ConsoleAppender">
        <encoder class="net.logstash.logback.encoder.LogstashEncoder">
            <includeMdcKeyName>traceId</includeMdcKeyName>
            <includeMdcKeyName>spanId</includeMdcKeyName>
            <fieldNames>
                <timestamp>timestamp</timestamp>
                <message>msg</message>
                <level>level</level>
                <logger>logger</logger>
            </fieldNames>
        </encoder>
    </appender>

    <root level="INFO">
        <appender-ref ref="JSON" />
    </root>

    <logger name="org.springframework" level="WARN" />
    <logger name="org.hibernate" level="WARN" />
</configuration>
```

Files:
- `java/task-service/src/main/resources/logback-spring.xml`
- `java/activity-service/src/main/resources/logback-spring.xml`
- `java/notification-service/src/main/resources/logback-spring.xml`
- `java/gateway-service/src/main/resources/logback-spring.xml`

- [ ] **Step 3: Commit**

```bash
git add java/*/src/main/resources/logback-spring.xml java/build.gradle
git commit -m "feat(java): add structured JSON logging with logback for Loki ingestion"
```

---

## Task 8: Add Kafka Consumer Lag Metrics

**Files:**
- Modify: `go/analytics-service/internal/metrics/prometheus.go`
- Modify: `go/analytics-service/internal/consumer/consumer.go`

- [ ] **Step 1: Add lag gauge to prometheus.go**

Add the following metric to `go/analytics-service/internal/metrics/prometheus.go`:

```go
// ConsumerLag tracks how far behind the consumer is per topic.
ConsumerLag = promauto.NewGaugeVec(prometheus.GaugeOpts{
    Name: "kafka_consumer_lag",
    Help: "Number of messages the consumer is behind per topic.",
}, []string{"topic"})

// ConsumerErrors counts Kafka read errors.
ConsumerErrors = promauto.NewCounter(prometheus.CounterOpts{
    Name: "kafka_consumer_errors_total",
    Help: "Total Kafka consumer read errors.",
})
```

- [ ] **Step 2: Record lag metrics in consumer.go**

Add a method to `Consumer` that reads stats from the kafka.Reader and publishes them, then call it periodically. Add a goroutine in `Run()`:

In `consumer.go`, add after the `c.connected.Store(true)` line inside the fetch loop:

```go
// Record consumer lag from reader stats.
stats := c.reader.Stats()
for _, topicStats := range []struct {
    topic string
    lag   int64
}{
    {TopicOrders, stats.Lag},
    {TopicCart, stats.Lag},
    {TopicViews, stats.Lag},
} {
    metrics.ConsumerLag.WithLabelValues(topicStats.topic).Set(float64(topicStats.lag))
}
```

Note: `kafka.Reader.Stats()` returns aggregate stats when using consumer groups. The `Lag` field represents total lag across all assigned partitions. Since we have a single consumer with 3 group topics, we record the aggregate lag. For per-topic lag, we'd need separate readers — this is sufficient for alerting.

Simplify to just record the aggregate:

```go
stats := c.reader.Stats()
metrics.ConsumerLag.WithLabelValues("aggregate").Set(float64(stats.Lag))
```

Also increment error counter on fetch errors — add in the error branch:

```go
metrics.ConsumerErrors.Inc()
```

- [ ] **Step 3: Run tests**

```bash
cd go/analytics-service && go test ./... -race -v
```

Expected: All tests pass.

- [ ] **Step 4: Commit**

```bash
git add go/analytics-service/internal/metrics/prometheus.go go/analytics-service/internal/consumer/consumer.go
git commit -m "feat(analytics): expose Kafka consumer lag as Prometheus metrics"
```

---

## Task 9: Add Kafka Lag Alert Rule

**Files:**
- Modify: `k8s/monitoring/configmaps/grafana-alerting.yml`

- [ ] **Step 1: Add Streaming Analytics alert group**

Add after the Application SLOs group:

```yaml
      - orgId: 1
        name: Streaming Analytics
        folder: Infrastructure Alerts
        interval: 1m
        rules:
          - uid: kafka-consumer-lag-high
            title: Kafka Consumer Lag High
            condition: C
            data:
              - refId: A
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: PBFA97CFB590B2093
                model:
                  expr: kafka_consumer_lag
                  instant: true
                  refId: A
              - refId: B
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: reduce
                  expression: A
                  reducer: last
                  refId: B
              - refId: C
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: threshold
                  expression: B
                  conditions:
                    - evaluator:
                        type: gt
                        params:
                          - 1000
                  refId: C
            for: 5m
            labels:
              severity: warning
            annotations:
              summary: "Kafka consumer lag is above 1000 messages"
```

- [ ] **Step 2: Commit**

```bash
git add k8s/monitoring/configmaps/grafana-alerting.yml
git commit -m "feat(monitoring): add Kafka consumer lag alert rule"
```

---

## Task 10: Add Streaming Analytics Dashboard Panels

**Files:**
- Modify: `k8s/monitoring/configmaps/grafana-dashboards.yml`

- [ ] **Step 1: Add Streaming Analytics row to go-services.json**

In `k8s/monitoring/configmaps/grafana-dashboards.yml`, locate the go-services.json dashboard section. After the last panel in the "AI Service Agent" row (around line 1490), add a new row and 3 panels:

**Row panel:**
```json
{
  "collapsed": false,
  "gridPos": { "h": 1, "w": 24, "x": 0, "y": 35 },
  "id": 22,
  "title": "Streaming Analytics",
  "type": "row"
}
```

**Panel 1 — Consumer Lag:**
```json
{
  "datasource": { "type": "prometheus", "uid": "PBFA97CFB590B2093" },
  "fieldConfig": { "defaults": { "unit": "short" } },
  "gridPos": { "h": 6, "w": 8, "x": 0, "y": 36 },
  "id": 23,
  "title": "Kafka Consumer Lag",
  "type": "timeseries",
  "targets": [
    {
      "expr": "kafka_consumer_lag",
      "legendFormat": "{{ topic }}",
      "refId": "A"
    }
  ]
}
```

**Panel 2 — Consumption Rate:**
```json
{
  "datasource": { "type": "prometheus", "uid": "PBFA97CFB590B2093" },
  "fieldConfig": { "defaults": { "unit": "ops" } },
  "gridPos": { "h": 6, "w": 8, "x": 8, "y": 36 },
  "id": 24,
  "title": "Events Consumed Rate",
  "type": "timeseries",
  "targets": [
    {
      "expr": "sum(rate(analytics_events_consumed_total[5m])) by (topic)",
      "legendFormat": "{{ topic }}",
      "refId": "A"
    }
  ]
}
```

**Panel 3 — Consumer Errors:**
```json
{
  "datasource": { "type": "prometheus", "uid": "PBFA97CFB590B2093" },
  "fieldConfig": { "defaults": { "unit": "short" } },
  "gridPos": { "h": 6, "w": 8, "x": 16, "y": 36 },
  "id": 25,
  "title": "Consumer Errors",
  "type": "stat",
  "targets": [
    {
      "expr": "increase(kafka_consumer_errors_total[1h])",
      "legendFormat": "errors",
      "refId": "A"
    }
  ]
}
```

- [ ] **Step 2: Commit**

```bash
git add k8s/monitoring/configmaps/grafana-dashboards.yml
git commit -m "feat(monitoring): add Streaming Analytics panels to Go services dashboard"
```

---

## Task 11: Create Observability Overview Correlation Dashboard

**Files:**
- Modify: `k8s/monitoring/configmaps/grafana-dashboards.yml`

- [ ] **Step 1: Add observability-overview.json dashboard**

In `k8s/monitoring/configmaps/grafana-dashboards.yml`, add a new key under `data:`:

```yaml
  observability-overview.json: |
```

The dashboard JSON has 3 rows:

**Row 1 — Service Health (Prometheus):** Error rate and p95 latency panels per service stack, using the same metrics as the SLO alerts.

**Row 2 — Logs (Loki):** A logs panel querying Loki filtered by namespace, showing error/warn level logs. Template variable `$namespace` lets you switch between namespaces.

**Row 3 — Recent Traces (Jaeger):** A table panel showing recent traces from the Jaeger datasource.

The full dashboard JSON should include:
- `templating.list` with a `namespace` variable (values: `go-ecommerce`, `java-tasks`, `ai-services`)
- Time range linked panels (clicking a time range on row 1 filters rows 2 and 3)
- Loki panel targets using `{namespace="$namespace"} | json | level=~"error|warn"`
- Dashboard uid: `observability-overview`
- Dashboard title: `Observability Overview`

Construct the complete JSON following the same format as the existing dashboards in the ConfigMap (annotations, editable, panels array, templating, time, etc.).

- [ ] **Step 2: Commit**

```bash
git add k8s/monitoring/configmaps/grafana-dashboards.yml
git commit -m "feat(monitoring): add Observability Overview correlation dashboard"
```

---

## Task 12: Write Learning Documentation

**Files:**
- Create: `docs/adr/observability/01-three-pillars.md`
- Create: `docs/adr/observability/02-prometheus-and-metrics.md`
- Create: `docs/adr/observability/03-loki-and-logs.md`
- Create: `docs/adr/observability/04-jaeger-and-traces.md`
- Create: `docs/adr/observability/05-alerting-and-slos.md`
- Create: `docs/adr/observability/06-correlation.md`

- [ ] **Step 1: Write 01-three-pillars.md**

Cover:
- What are metrics, logs, and traces — with concrete examples from this project
- Monitoring vs observability — the difference between "is it up?" and "why is it slow?"
- When to reach for each pillar: metrics for detection, logs for investigation, traces for distributed path analysis
- The "three pillars" model and its limitations (they overlap, correlation is what makes them powerful)
- Reference: this project's stack (Prometheus for metrics, Loki for logs, Jaeger for traces, Grafana as the unified UI)

- [ ] **Step 2: Write 02-prometheus-and-metrics.md**

Cover:
- Pull vs push model — why Prometheus scrapes targets instead of receiving pushes
- The four metric types with examples from this project:
  - Counter: `http_requests_total`, `analytics_events_consumed_total`
  - Gauge: `kafka_consumer_lag`, `go_goroutines`
  - Histogram: `http_request_duration_seconds_bucket` — explain how buckets work
  - Summary: when to use vs histogram (hint: almost never)
- PromQL basics using actual queries from our dashboards/alerts
- Infrastructure metrics: what kube-state-metrics and node-exporter provide
- How Prometheus discovers scrape targets (static config vs kubernetes_sd_configs)

- [ ] **Step 3: Write 03-loki-and-logs.md**

Cover:
- Why centralized logging matters (the OOM kill that started this project — you had to SSH in to see it)
- Loki architecture: distributor → ingester → storage, but in single-binary mode it's all one process
- Label-based indexing vs ELK full-text indexing — trade-offs (Loki: cheap storage, slow grep; ELK: fast search, expensive storage)
- LogQL query language with examples: `{namespace="go-ecommerce"} | json | level="error"`, rate queries
- Promtail: how it discovers and tails pod logs, pipeline stages for parsing JSON
- Structured logging: why JSON logs with consistent field names (level, msg, traceID) matter

- [ ] **Step 4: Write 04-jaeger-and-traces.md**

Cover:
- What a span is (unit of work with start time, duration, tags, parent)
- What a trace is (tree of spans sharing a traceID)
- Context propagation: W3C traceparent header, how traceID flows across HTTP calls
- This project's setup: Go SDK + otelgin middleware + otelhttp client transport
- Kafka trace propagation: `InjectKafka`/`ExtractKafka` in `go/pkg/tracing/kafka.go` — how traceID survives async message passing
- The request flow: HTTP request → gateway → ecommerce → Kafka message → analytics-service, all sharing one traceID
- When tracing helps (latency debugging, dependency mapping) vs when it's noise (high-volume background jobs)

- [ ] **Step 5: Write 05-alerting-and-slos.md**

Cover:
- Symptom-based vs cause-based alerting — "error rate is high" vs "disk is 80% full"
- Why symptom-based wins (fewer alerts, catches unknown unknowns)
- The RED method: Rate, Errors, Duration — the standard for request-driven services
- What an SLO is: a target for user-facing reliability (e.g., "99.9% of requests succeed")
- SLI vs SLO vs SLA — the measurement, the target, the contract
- This project's SLOs: why 5% error rate for AI service (LLM failures are expected) vs 2% for ecommerce (reliability critical)
- Alert fatigue: why pending periods matter, why not everything should be critical
- Our alert hierarchy: Infrastructure (node pressure) → Kubernetes Health (OOM, restarts) → Application SLOs (error rate, latency) → Streaming (Kafka lag)

- [ ] **Step 6: Write 06-correlation.md**

Cover:
- The observability workflow: alert fires → metrics dashboard → logs → traces
- Walk through a concrete scenario using this project:
  1. Telegram alert: "Go ecommerce error rate above 2%"
  2. Open Grafana Observability Overview dashboard, see the spike in the metrics row
  3. Scroll to logs row, filter by `level="error"`, see the error messages
  4. Find a log line with `traceID`, click it
  5. Jaeger shows the full trace: HTTP request → ecommerce handler → Redis timeout → circuit breaker trip
  6. Root cause identified: Redis was restarting
- How the correlation works technically: Promtail extracts traceID → Grafana derived fields → Jaeger link
- Kafka's observability challenge: producer and consumer are decoupled in time, but traceID bridges them
- The "unknown unknowns" argument: metrics tell you something is wrong, logs tell you what, traces tell you where

- [ ] **Step 7: Commit all docs**

```bash
git add docs/adr/observability/
git commit -m "docs: add observability learning guides (three pillars, Prometheus, Loki, Jaeger, SLOs, correlation)"
```

---

## Task 13: Deploy and Verify

- [ ] **Step 1: Run preflight checks**

```bash
make preflight-go
```

Expected: All Go lint and tests pass.

- [ ] **Step 2: Push feature branch**

```bash
git push -u origin agent/feat-observability-full-stack
```

- [ ] **Step 3: Watch CI**

Monitor the GitHub Actions run. Wait for all checks to complete.

- [ ] **Step 4: If CI passes, create PR to qa**

```bash
gh pr create --base qa --title "feat: full-stack observability (Loki, alerts, SLOs, Kafka monitoring, correlation)" --body "$(cat <<'EOF'
## Summary
- Deploy Loki + Promtail for centralized log aggregation
- Add 6 Kubernetes health alerts (OOM, restarts, memory/disk pressure, stuck deployments)
- Add 6 application SLO alerts (error rate + p95 latency for Go AI, Go ecommerce, Java gateway)
- Add Kafka consumer lag metrics and alerting
- Add traceID to Go service logs for log-to-trace correlation
- Add structured JSON logging to Java services
- Create Observability Overview correlation dashboard (metrics → logs → traces)
- Add 6 learning docs covering observability fundamentals

## Test plan
- [ ] Verify Loki and Promtail pods are running in monitoring namespace
- [ ] Query `{namespace="go-ecommerce"}` in Grafana Explore with Loki datasource
- [ ] Verify all alert rules appear in Grafana Alerting UI
- [ ] Check Go services dashboard has Streaming Analytics row
- [ ] Open Observability Overview dashboard, verify 3 rows render
- [ ] Click a traceID in logs panel, verify Jaeger trace opens
- [ ] Review learning docs in docs/adr/observability/

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 5: If CI fails, debug and fix**

Check the specific failure, fix it, push, and re-run from step 3.
