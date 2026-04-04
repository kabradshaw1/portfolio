# Unified Kubernetes Deployment — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move all backend services (Python AI + Java microservices + monitoring) into Kubernetes (Minikube) with NGINX Ingress Controller as the unified entry point, replacing Docker Compose as the production deployment.

**Architecture:** Three K8s namespaces (`ai-services`, `java-tasks`, `monitoring`) behind a shared NGINX Ingress Controller. Ollama stays on the host (GPU), accessed via ExternalName service. Docker Compose files remain for local development. CI deploys via SSH + kubectl instead of docker compose.

**Tech Stack:** Minikube, kubectl, NGINX Ingress Controller (Minikube addon), K8s Deployments/Services/ConfigMaps/Secrets/Ingress, GitHub Actions

**Spec:** `docs/superpowers/specs/2026-04-04-nginx-java-integration-design.md`

---

## File Structure

```
k8s/
├── ai-services/
│   ├── namespace.yml
│   ├── configmaps/
│   │   ├── ingestion-config.yml
│   │   ├── chat-config.yml
│   │   └── debug-config.yml
│   ├── deployments/
│   │   ├── ingestion.yml
│   │   ├── chat.yml
│   │   ├── debug.yml
│   │   └── qdrant.yml
│   ├── services/
│   │   ├── ingestion.yml
│   │   ├── chat.yml
│   │   ├── debug.yml
│   │   ├── qdrant.yml
│   │   └── ollama.yml           # ExternalName → host.minikube.internal
│   └── ingress.yml              # /ingestion/*, /chat/*, /debug/*
├── monitoring/
│   ├── namespace.yml
│   ├── configmaps/
│   │   ├── prometheus-config.yml
│   │   ├── grafana-datasource.yml
│   │   └── grafana-dashboard-provider.yml
│   ├── deployments/
│   │   ├── prometheus.yml
│   │   └── grafana.yml
│   ├── services/
│   │   ├── prometheus.yml
│   │   └── grafana.yml
│   └── ingress.yml              # /grafana/*
└── deploy.sh                    # Unified deploy: all 3 namespaces + ingress addon

java/k8s/
├── ingress.yml                  # NEW: /graphql, /graphiql, /api/auth/*, /rabbitmq/, /java/health
├── configmaps/
│   ├── gateway-service-config.yml  # UPDATE: ALLOWED_ORIGINS, GRAPHIQL_ENABLED
│   └── task-service-config.yml     # UPDATE: ALLOWED_ORIGINS

.github/workflows/ci.yml           # UPDATE: deploy job uses kubectl
frontend/.env.local.example         # UPDATE: add NEXT_PUBLIC_GATEWAY_URL
CLAUDE.md                           # UPDATE: document K8s architecture
```

---

## Phase 1: ai-services Namespace

### Task 1: Namespace and ConfigMaps

**Files:**
- Create: `k8s/ai-services/namespace.yml`
- Create: `k8s/ai-services/configmaps/ingestion-config.yml`
- Create: `k8s/ai-services/configmaps/chat-config.yml`
- Create: `k8s/ai-services/configmaps/debug-config.yml`

- [ ] **Step 1: Write namespace**

Create `k8s/ai-services/namespace.yml`:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: ai-services
```

- [ ] **Step 2: Write ingestion ConfigMap**

Create `k8s/ai-services/configmaps/ingestion-config.yml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: ingestion-config
  namespace: ai-services
data:
  OLLAMA_BASE_URL: http://ollama:11434
  EMBEDDING_MODEL: nomic-embed-text
  QDRANT_HOST: qdrant
  QDRANT_PORT: "6333"
  COLLECTION_NAME: documents
  CHUNK_SIZE: "1000"
  CHUNK_OVERLAP: "200"
  MAX_FILE_SIZE_MB: "50"
  ALLOWED_ORIGINS: http://localhost:3000,https://kylebradshaw.dev
```

- [ ] **Step 3: Write chat ConfigMap**

Create `k8s/ai-services/configmaps/chat-config.yml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: chat-config
  namespace: ai-services
data:
  OLLAMA_BASE_URL: http://ollama:11434
  CHAT_MODEL: qwen2.5:14b
  EMBEDDING_MODEL: nomic-embed-text
  QDRANT_HOST: qdrant
  QDRANT_PORT: "6333"
  COLLECTION_NAME: documents
  ALLOWED_ORIGINS: http://localhost:3000,https://kylebradshaw.dev
```

- [ ] **Step 4: Write debug ConfigMap**

Create `k8s/ai-services/configmaps/debug-config.yml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: debug-config
  namespace: ai-services
data:
  OLLAMA_BASE_URL: http://ollama:11434
  CHAT_MODEL: qwen2.5:14b
  EMBEDDING_MODEL: nomic-embed-text
  QDRANT_HOST: qdrant
  QDRANT_PORT: "6333"
  MAX_AGENT_STEPS: "10"
  MAX_GREP_MATCHES: "20"
  TEST_TIMEOUT_SECONDS: "30"
  ALLOWED_ORIGINS: http://localhost:3000,https://kylebradshaw.dev
```

- [ ] **Step 5: Validate YAML**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer
for f in k8s/ai-services/namespace.yml k8s/ai-services/configmaps/*.yml; do
  python3 -c "import yaml; yaml.safe_load(open('$f'))" && echo "$f: valid"
done
```

- [ ] **Step 6: Commit**

```bash
git add k8s/ai-services/namespace.yml k8s/ai-services/configmaps/
git commit -m "feat(k8s): add ai-services namespace and ConfigMaps"
```

---

### Task 2: Qdrant and Ollama Services

**Files:**
- Create: `k8s/ai-services/deployments/qdrant.yml`
- Create: `k8s/ai-services/services/qdrant.yml`
- Create: `k8s/ai-services/services/ollama.yml`

- [ ] **Step 1: Write Qdrant deployment**

Create `k8s/ai-services/deployments/qdrant.yml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: qdrant
  namespace: ai-services
spec:
  replicas: 1
  selector:
    matchLabels:
      app: qdrant
  template:
    metadata:
      labels:
        app: qdrant
    spec:
      containers:
        - name: qdrant
          image: qdrant/qdrant:latest
          ports:
            - containerPort: 6333
            - containerPort: 6334
          resources:
            requests:
              memory: "256Mi"
              cpu: "100m"
            limits:
              memory: "1Gi"
              cpu: "500m"
          readinessProbe:
            tcpSocket:
              port: 6333
            initialDelaySeconds: 5
            periodSeconds: 10
```

- [ ] **Step 2: Write Qdrant service**

Create `k8s/ai-services/services/qdrant.yml`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: qdrant
  namespace: ai-services
spec:
  selector:
    app: qdrant
  ports:
    - name: rest
      port: 6333
      targetPort: 6333
    - name: grpc
      port: 6334
      targetPort: 6334
```

- [ ] **Step 3: Write Ollama ExternalName service**

Create `k8s/ai-services/services/ollama.yml`:

```yaml
# Ollama runs on the host with GPU (RTX 3090).
# ExternalName service lets pods reach it via http://ollama:11434
apiVersion: v1
kind: Service
metadata:
  name: ollama
  namespace: ai-services
spec:
  type: ExternalName
  externalName: host.minikube.internal
```

- [ ] **Step 4: Commit**

```bash
git add k8s/ai-services/deployments/qdrant.yml k8s/ai-services/services/
git commit -m "feat(k8s): add Qdrant deployment and Ollama ExternalName service"
```

---

### Task 3: Python Service Deployments and Services

**Files:**
- Create: `k8s/ai-services/deployments/ingestion.yml`
- Create: `k8s/ai-services/deployments/chat.yml`
- Create: `k8s/ai-services/deployments/debug.yml`
- Create: `k8s/ai-services/services/ingestion.yml`
- Create: `k8s/ai-services/services/chat.yml`
- Create: `k8s/ai-services/services/debug.yml`

- [ ] **Step 1: Write ingestion deployment**

Create `k8s/ai-services/deployments/ingestion.yml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ingestion
  namespace: ai-services
spec:
  replicas: 1
  selector:
    matchLabels:
      app: ingestion
  template:
    metadata:
      labels:
        app: ingestion
    spec:
      containers:
        - name: ingestion
          image: ghcr.io/kabradshaw1/gen_ai_engineer/ingestion:latest
          ports:
            - containerPort: 8000
          envFrom:
            - configMapRef:
                name: ingestion-config
          resources:
            requests:
              memory: "128Mi"
              cpu: "100m"
            limits:
              memory: "512Mi"
              cpu: "500m"
          readinessProbe:
            httpGet:
              path: /health
              port: 8000
            initialDelaySeconds: 10
            periodSeconds: 10
            timeoutSeconds: 5
```

- [ ] **Step 2: Write ingestion service**

Create `k8s/ai-services/services/ingestion.yml`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: ingestion
  namespace: ai-services
spec:
  selector:
    app: ingestion
  ports:
    - port: 8000
      targetPort: 8000
```

- [ ] **Step 3: Write chat deployment**

Create `k8s/ai-services/deployments/chat.yml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: chat
  namespace: ai-services
spec:
  replicas: 1
  selector:
    matchLabels:
      app: chat
  template:
    metadata:
      labels:
        app: chat
    spec:
      containers:
        - name: chat
          image: ghcr.io/kabradshaw1/gen_ai_engineer/chat:latest
          ports:
            - containerPort: 8000
          envFrom:
            - configMapRef:
                name: chat-config
          resources:
            requests:
              memory: "128Mi"
              cpu: "100m"
            limits:
              memory: "512Mi"
              cpu: "500m"
          readinessProbe:
            httpGet:
              path: /health
              port: 8000
            initialDelaySeconds: 10
            periodSeconds: 10
            timeoutSeconds: 5
```

- [ ] **Step 4: Write chat service**

Create `k8s/ai-services/services/chat.yml`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: chat
  namespace: ai-services
spec:
  selector:
    app: chat
  ports:
    - port: 8000
      targetPort: 8000
```

- [ ] **Step 5: Write debug deployment**

Create `k8s/ai-services/deployments/debug.yml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: debug
  namespace: ai-services
spec:
  replicas: 1
  selector:
    matchLabels:
      app: debug
  template:
    metadata:
      labels:
        app: debug
    spec:
      containers:
        - name: debug
          image: ghcr.io/kabradshaw1/gen_ai_engineer/debug:latest
          ports:
            - containerPort: 8000
          envFrom:
            - configMapRef:
                name: debug-config
          resources:
            requests:
              memory: "128Mi"
              cpu: "100m"
            limits:
              memory: "512Mi"
              cpu: "500m"
          readinessProbe:
            httpGet:
              path: /health
              port: 8000
            initialDelaySeconds: 10
            periodSeconds: 10
            timeoutSeconds: 5
```

- [ ] **Step 6: Write debug service**

Create `k8s/ai-services/services/debug.yml`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: debug
  namespace: ai-services
spec:
  selector:
    app: debug
  ports:
    - port: 8000
      targetPort: 8000
```

- [ ] **Step 7: Commit**

```bash
git add k8s/ai-services/deployments/ingestion.yml k8s/ai-services/deployments/chat.yml \
        k8s/ai-services/deployments/debug.yml k8s/ai-services/services/ingestion.yml \
        k8s/ai-services/services/chat.yml k8s/ai-services/services/debug.yml
git commit -m "feat(k8s): add Python AI service deployments and services"
```

---

### Task 4: ai-services Ingress

**Files:**
- Create: `k8s/ai-services/ingress.yml`

- [ ] **Step 1: Write Ingress resource**

Create `k8s/ai-services/ingress.yml`:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: ai-services-ingress
  namespace: ai-services
  annotations:
    nginx.ingress.kubernetes.io/use-regex: "true"
    nginx.ingress.kubernetes.io/proxy-buffering: "off"
    nginx.ingress.kubernetes.io/proxy-read-timeout: "300"
    nginx.ingress.kubernetes.io/proxy-body-size: "55m"
    nginx.ingress.kubernetes.io/rewrite-target: /$2
spec:
  ingressClassName: nginx
  rules:
    - http:
        paths:
          - path: /ingestion(/|$)(.*)
            pathType: ImplementationSpecific
            backend:
              service:
                name: ingestion
                port:
                  number: 8000
          - path: /chat(/|$)(.*)
            pathType: ImplementationSpecific
            backend:
              service:
                name: chat
                port:
                  number: 8000
          - path: /debug(/|$)(.*)
            pathType: ImplementationSpecific
            backend:
              service:
                name: debug
                port:
                  number: 8000
```

Note: The `rewrite-target: /$2` strips the path prefix. `/ingestion/health` → `/health` on the service. This matches the current nginx `proxy_pass http://ingestion/;` behavior (trailing slash strips prefix).

The annotations replicate the existing nginx.conf settings: SSE support (proxy-buffering off), 300s timeout for streaming, 55MB upload limit.

- [ ] **Step 2: Commit**

```bash
git add k8s/ai-services/ingress.yml
git commit -m "feat(k8s): add ai-services Ingress with path rewriting"
```

---

## Phase 2: Monitoring Namespace

### Task 5: Prometheus

**Files:**
- Create: `k8s/monitoring/namespace.yml`
- Create: `k8s/monitoring/configmaps/prometheus-config.yml`
- Create: `k8s/monitoring/deployments/prometheus.yml`
- Create: `k8s/monitoring/services/prometheus.yml`

- [ ] **Step 1: Write namespace**

Create `k8s/monitoring/namespace.yml`:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: monitoring
```

- [ ] **Step 2: Write Prometheus ConfigMap**

Create `k8s/monitoring/configmaps/prometheus-config.yml`:

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

      - job_name: "qdrant"
        metrics_path: /healthz
        static_configs:
          - targets: ["qdrant.ai-services.svc.cluster.local:6333"]

      - job_name: "ingestion"
        metrics_path: /health
        static_configs:
          - targets: ["ingestion.ai-services.svc.cluster.local:8000"]

      - job_name: "chat"
        metrics_path: /health
        static_configs:
          - targets: ["chat.ai-services.svc.cluster.local:8000"]

      - job_name: "debug"
        metrics_path: /health
        static_configs:
          - targets: ["debug.ai-services.svc.cluster.local:8000"]

      - job_name: "gateway-service"
        metrics_path: /actuator/health
        static_configs:
          - targets: ["gateway-service.java-tasks.svc.cluster.local:8080"]

      - job_name: "grafana"
        metrics_path: /api/health
        static_configs:
          - targets: ["grafana.monitoring.svc.cluster.local:3000"]
```

Note: `host.minikube.internal` replaces `host.docker.internal` for reaching services on the Windows host. The `nvidia-gpu-exporter` container still runs via Docker on the host (GPU access). The `debug` service scrape target is added (was missing from the Docker Compose prometheus.yml).

- [ ] **Step 3: Write Prometheus deployment**

Create `k8s/monitoring/deployments/prometheus.yml`:

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
  template:
    metadata:
      labels:
        app: prometheus
    spec:
      containers:
        - name: prometheus
          image: prom/prometheus:latest
          ports:
            - containerPort: 9090
          args:
            - "--config.file=/etc/prometheus/prometheus.yml"
          volumeMounts:
            - name: config
              mountPath: /etc/prometheus/prometheus.yml
              subPath: prometheus.yml
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
      volumes:
        - name: config
          configMap:
            name: prometheus-config
```

- [ ] **Step 4: Write Prometheus service**

Create `k8s/monitoring/services/prometheus.yml`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: prometheus
  namespace: monitoring
spec:
  selector:
    app: prometheus
  ports:
    - port: 9090
      targetPort: 9090
```

- [ ] **Step 5: Commit**

```bash
git add k8s/monitoring/namespace.yml k8s/monitoring/configmaps/prometheus-config.yml \
        k8s/monitoring/deployments/prometheus.yml k8s/monitoring/services/prometheus.yml
git commit -m "feat(k8s): add monitoring namespace with Prometheus"
```

---

### Task 6: Grafana

**Files:**
- Create: `k8s/monitoring/configmaps/grafana-datasource.yml`
- Create: `k8s/monitoring/configmaps/grafana-dashboard-provider.yml`
- Create: `k8s/monitoring/deployments/grafana.yml`
- Create: `k8s/monitoring/services/grafana.yml`

- [ ] **Step 1: Write Grafana datasource ConfigMap**

Create `k8s/monitoring/configmaps/grafana-datasource.yml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: grafana-datasource
  namespace: monitoring
data:
  prometheus.yml: |
    apiVersion: 1
    datasources:
      - name: Prometheus
        type: prometheus
        access: proxy
        url: http://prometheus.monitoring.svc.cluster.local:9090
        isDefault: true
        editable: false
```

- [ ] **Step 2: Write Grafana dashboard provider ConfigMap**

Create `k8s/monitoring/configmaps/grafana-dashboard-provider.yml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: grafana-dashboard-provider
  namespace: monitoring
data:
  dashboard.yml: |
    apiVersion: 1
    providers:
      - name: "default"
        orgId: 1
        folder: ""
        type: file
        disableDeletion: false
        editable: true
        options:
          path: /var/lib/grafana/dashboards
          foldersFromFilesStructure: false
```

- [ ] **Step 3: Write Grafana deployment**

Create `k8s/monitoring/deployments/grafana.yml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: grafana
  namespace: monitoring
spec:
  replicas: 1
  selector:
    matchLabels:
      app: grafana
  template:
    metadata:
      labels:
        app: grafana
    spec:
      containers:
        - name: grafana
          image: grafana/grafana:latest
          ports:
            - containerPort: 3000
          env:
            - name: GF_SECURITY_ADMIN_PASSWORD
              value: admin
            - name: GF_AUTH_ANONYMOUS_ENABLED
              value: "true"
            - name: GF_AUTH_ANONYMOUS_ORG_ROLE
              value: Viewer
            - name: GF_SERVER_ROOT_URL
              value: https://api.kylebradshaw.dev/grafana/
            - name: GF_SERVER_SERVE_FROM_SUB_PATH
              value: "true"
          volumeMounts:
            - name: datasource
              mountPath: /etc/grafana/provisioning/datasources/prometheus.yml
              subPath: prometheus.yml
            - name: dashboard-provider
              mountPath: /etc/grafana/provisioning/dashboards/dashboard.yml
              subPath: dashboard.yml
          resources:
            requests:
              memory: "128Mi"
              cpu: "100m"
            limits:
              memory: "512Mi"
              cpu: "500m"
          readinessProbe:
            httpGet:
              path: /api/health
              port: 3000
            initialDelaySeconds: 10
            periodSeconds: 10
      volumes:
        - name: datasource
          configMap:
            name: grafana-datasource
        - name: dashboard-provider
          configMap:
            name: grafana-dashboard-provider
```

Note: `GF_SERVER_SERVE_FROM_SUB_PATH=true` and `GF_SERVER_ROOT_URL` with `/grafana/` path enable Grafana to work behind the Ingress at `/grafana/`. Anonymous access as Viewer lets recruiters browse dashboards without login.

- [ ] **Step 4: Write Grafana service**

Create `k8s/monitoring/services/grafana.yml`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: grafana
  namespace: monitoring
spec:
  selector:
    app: grafana
  ports:
    - port: 3000
      targetPort: 3000
```

- [ ] **Step 5: Commit**

```bash
git add k8s/monitoring/configmaps/grafana-datasource.yml \
        k8s/monitoring/configmaps/grafana-dashboard-provider.yml \
        k8s/monitoring/deployments/grafana.yml k8s/monitoring/services/grafana.yml
git commit -m "feat(k8s): add Grafana with datasource and dashboard provisioning"
```

---

### Task 7: Monitoring Ingress

**Files:**
- Create: `k8s/monitoring/ingress.yml`

- [ ] **Step 1: Write monitoring Ingress**

Create `k8s/monitoring/ingress.yml`:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: monitoring-ingress
  namespace: monitoring
  annotations:
    nginx.ingress.kubernetes.io/use-regex: "true"
    nginx.ingress.kubernetes.io/rewrite-target: /$2
spec:
  ingressClassName: nginx
  rules:
    - http:
        paths:
          - path: /grafana(/|$)(.*)
            pathType: ImplementationSpecific
            backend:
              service:
                name: grafana
                port:
                  number: 3000
```

- [ ] **Step 2: Commit**

```bash
git add k8s/monitoring/ingress.yml
git commit -m "feat(k8s): add monitoring Ingress for Grafana"
```

---

## Phase 3: java-tasks Namespace Updates

### Task 8: Java ConfigMap Updates and Ingress

**Files:**
- Modify: `java/k8s/configmaps/gateway-service-config.yml`
- Modify: `java/k8s/configmaps/task-service-config.yml`
- Create: `java/k8s/ingress.yml`

- [ ] **Step 1: Update gateway-service ConfigMap**

In `java/k8s/configmaps/gateway-service-config.yml`, add `ALLOWED_ORIGINS` with production domain and enable GraphiQL:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: gateway-service-config
  namespace: java-tasks
data:
  TASK_SERVICE_URL: http://task-service:8081
  ACTIVITY_SERVICE_URL: http://activity-service:8082
  NOTIFICATION_SERVICE_URL: http://notification-service:8083
  ALLOWED_ORIGINS: http://localhost:3000,https://kylebradshaw.dev
  GRAPHIQL_ENABLED: "true"
```

- [ ] **Step 2: Update task-service ConfigMap**

In `java/k8s/configmaps/task-service-config.yml`, add production origin to ALLOWED_ORIGINS:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: task-service-config
  namespace: java-tasks
data:
  POSTGRES_HOST: postgres
  POSTGRES_USER: taskuser
  RABBITMQ_HOST: rabbitmq
  RABBITMQ_USER: guest
  RABBITMQ_PASSWORD: guest
  ALLOWED_ORIGINS: http://localhost:3000,https://kylebradshaw.dev
```

- [ ] **Step 3: Write java-tasks Ingress**

Create `java/k8s/ingress.yml`:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: java-tasks-ingress
  namespace: java-tasks
  annotations:
    nginx.ingress.kubernetes.io/use-regex: "true"
spec:
  ingressClassName: nginx
  rules:
    - http:
        paths:
          - path: /graphql
            pathType: Exact
            backend:
              service:
                name: gateway-service
                port:
                  number: 8080
          - path: /graphiql
            pathType: Prefix
            backend:
              service:
                name: gateway-service
                port:
                  number: 8080
          - path: /api/auth/(.*)
            pathType: ImplementationSpecific
            backend:
              service:
                name: task-service
                port:
                  number: 8081
          - path: /java/health
            pathType: Exact
            backend:
              service:
                name: gateway-service
                port:
                  number: 8080
          - path: /rabbitmq(/|$)(.*)
            pathType: ImplementationSpecific
            backend:
              service:
                name: rabbitmq
                port:
                  number: 15672
```

Note: `/graphql` and `/graphiql` use Exact/Prefix — no rewriting needed, the gateway-service expects these paths directly. `/api/auth/(.*)` routes to task-service without rewriting — the task-service AuthController expects `/api/auth/google` and `/api/auth/refresh`. `/java/health` routes to gateway-service — the gateway returns its actuator health at any authenticated endpoint; however, since `/java/health` isn't `/actuator/health`, we need a rewrite annotation. Add this to the `/java/health` path via a separate Ingress or use a snippet:

Actually, simplify: expose `/actuator/health` directly instead of `/java/health`:

Replace the `/java/health` path with:

```yaml
          - path: /actuator/health
            pathType: Exact
            backend:
              service:
                name: gateway-service
                port:
                  number: 8080
```

The gateway-service already permits `/actuator/health` without auth in its SecurityConfig. This avoids needing a rewrite.

- [ ] **Step 4: Commit**

```bash
git add java/k8s/configmaps/gateway-service-config.yml java/k8s/configmaps/task-service-config.yml \
        java/k8s/ingress.yml
git commit -m "feat(k8s): add java-tasks Ingress and update ConfigMaps with CORS/GraphiQL"
```

---

## Phase 4: Deployment and CI/CD

### Task 9: Unified Deploy Script

**Files:**
- Create: `k8s/deploy.sh`

- [ ] **Step 1: Write deploy script**

Create `k8s/deploy.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

# Deploy all services to Minikube
# Prerequisites: minikube running, kubectl configured
# Usage: ./k8s/deploy.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "==> Enabling NGINX Ingress Controller..."
minikube addons enable ingress 2>/dev/null || true

echo "==> Creating namespaces..."
kubectl apply -f "$SCRIPT_DIR/ai-services/namespace.yml"
kubectl apply -f "$REPO_DIR/java/k8s/namespace.yml"
kubectl apply -f "$SCRIPT_DIR/monitoring/namespace.yml"

echo "==> Applying secrets..."
kubectl apply -f "$REPO_DIR/java/k8s/secrets/java-secrets.yml"

echo "==> Applying ConfigMaps..."
kubectl apply -f "$SCRIPT_DIR/ai-services/configmaps/"
kubectl apply -f "$REPO_DIR/java/k8s/configmaps/"
kubectl apply -f "$SCRIPT_DIR/monitoring/configmaps/"

echo "==> Deploying ai-services (Qdrant + Ollama)..."
kubectl apply -f "$SCRIPT_DIR/ai-services/services/ollama.yml"
kubectl apply -f "$SCRIPT_DIR/ai-services/deployments/qdrant.yml"
kubectl apply -f "$SCRIPT_DIR/ai-services/services/qdrant.yml"

echo "==> Waiting for Qdrant..."
kubectl wait --for=condition=available --timeout=120s deployment/qdrant -n ai-services

echo "==> Deploying ai-services (Python services)..."
kubectl apply -f "$SCRIPT_DIR/ai-services/deployments/ingestion.yml"
kubectl apply -f "$SCRIPT_DIR/ai-services/deployments/chat.yml"
kubectl apply -f "$SCRIPT_DIR/ai-services/deployments/debug.yml"
kubectl apply -f "$SCRIPT_DIR/ai-services/services/ingestion.yml"
kubectl apply -f "$SCRIPT_DIR/ai-services/services/chat.yml"
kubectl apply -f "$SCRIPT_DIR/ai-services/services/debug.yml"

echo "==> Deploying java-tasks infrastructure..."
kubectl apply -f "$REPO_DIR/java/k8s/deployments/postgres.yml"
kubectl apply -f "$REPO_DIR/java/k8s/deployments/mongodb.yml"
kubectl apply -f "$REPO_DIR/java/k8s/deployments/redis.yml"
kubectl apply -f "$REPO_DIR/java/k8s/deployments/rabbitmq.yml"
kubectl apply -f "$REPO_DIR/java/k8s/services/postgres.yml"
kubectl apply -f "$REPO_DIR/java/k8s/services/mongodb.yml"
kubectl apply -f "$REPO_DIR/java/k8s/services/redis.yml"
kubectl apply -f "$REPO_DIR/java/k8s/services/rabbitmq.yml"

echo "==> Waiting for java-tasks infrastructure..."
kubectl wait --for=condition=available --timeout=120s deployment/postgres -n java-tasks
kubectl wait --for=condition=available --timeout=120s deployment/mongodb -n java-tasks
kubectl wait --for=condition=available --timeout=120s deployment/redis -n java-tasks
kubectl wait --for=condition=available --timeout=120s deployment/rabbitmq -n java-tasks

echo "==> Deploying java-tasks application services..."
kubectl apply -f "$REPO_DIR/java/k8s/deployments/task-service.yml"
kubectl apply -f "$REPO_DIR/java/k8s/deployments/activity-service.yml"
kubectl apply -f "$REPO_DIR/java/k8s/deployments/notification-service.yml"
kubectl apply -f "$REPO_DIR/java/k8s/deployments/gateway-service.yml"
kubectl apply -f "$REPO_DIR/java/k8s/services/task-service.yml"
kubectl apply -f "$REPO_DIR/java/k8s/services/activity-service.yml"
kubectl apply -f "$REPO_DIR/java/k8s/services/notification-service.yml"
kubectl apply -f "$REPO_DIR/java/k8s/services/gateway-service.yml"

echo "==> Deploying monitoring..."
kubectl apply -f "$SCRIPT_DIR/monitoring/deployments/prometheus.yml"
kubectl apply -f "$SCRIPT_DIR/monitoring/services/prometheus.yml"
kubectl apply -f "$SCRIPT_DIR/monitoring/deployments/grafana.yml"
kubectl apply -f "$SCRIPT_DIR/monitoring/services/grafana.yml"

echo "==> Applying Ingress resources..."
kubectl apply -f "$SCRIPT_DIR/ai-services/ingress.yml"
kubectl apply -f "$REPO_DIR/java/k8s/ingress.yml"
kubectl apply -f "$SCRIPT_DIR/monitoring/ingress.yml"

echo "==> Waiting for all application services..."
kubectl wait --for=condition=available --timeout=180s deployment/ingestion -n ai-services
kubectl wait --for=condition=available --timeout=180s deployment/chat -n ai-services
kubectl wait --for=condition=available --timeout=180s deployment/debug -n ai-services
kubectl wait --for=condition=available --timeout=180s deployment/task-service -n java-tasks
kubectl wait --for=condition=available --timeout=180s deployment/activity-service -n java-tasks
kubectl wait --for=condition=available --timeout=180s deployment/notification-service -n java-tasks
kubectl wait --for=condition=available --timeout=180s deployment/gateway-service -n java-tasks
kubectl wait --for=condition=available --timeout=120s deployment/prometheus -n monitoring
kubectl wait --for=condition=available --timeout=120s deployment/grafana -n monitoring

echo ""
echo "==> All services deployed!"
echo ""
echo "    Namespaces:"
echo "      ai-services  — Python AI services + Qdrant"
echo "      java-tasks   — Java microservices + databases"
echo "      monitoring   — Prometheus + Grafana"
echo ""
echo "    Next steps:"
echo "      1. Run 'minikube tunnel' in a separate terminal (requires sudo)"
echo "      2. Access services at http://localhost/"
echo ""
echo "    Endpoints (via Ingress):"
echo "      /ingestion/*    — Document ingestion API"
echo "      /chat/*         — RAG chat API"
echo "      /debug/*        — Debug assistant API"
echo "      /graphql        — Java GraphQL API"
echo "      /graphiql       — GraphQL IDE"
echo "      /api/auth/*     — OAuth authentication"
echo "      /grafana/       — Monitoring dashboards"
echo "      /rabbitmq/      — Message broker UI"
echo ""
echo "    Verify: kubectl get ingress --all-namespaces"
```

- [ ] **Step 2: Make executable**

```bash
chmod +x /Users/kylebradshaw/repos/gen_ai_engineer/k8s/deploy.sh
```

- [ ] **Step 3: Commit**

```bash
git add k8s/deploy.sh
git commit -m "feat(k8s): add unified deploy script for all namespaces"
```

---

### Task 10: Update CI/CD Pipeline

**Files:**
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Update deploy job**

In `.github/workflows/ci.yml`, replace the deploy job's SSH script (the `script:` block inside the `appleboy/ssh-action` step) with kubectl-based deployment:

Find the current script block:

```yaml
          script: |
            cd ${{ secrets.DEPLOY_PATH }}
            git pull origin main
            docker compose pull ingestion chat debug
            docker compose up -d
            cd java
            docker compose pull task-service activity-service notification-service gateway-service 2>/dev/null || true
            docker compose up -d 2>/dev/null || true
```

Replace with:

```yaml
          script: |
            cd ${{ secrets.DEPLOY_PATH }}
            git pull origin main
            kubectl apply -f k8s/ai-services/ --recursive
            kubectl apply -f java/k8s/ --recursive
            kubectl apply -f k8s/monitoring/ --recursive
            kubectl rollout restart deployment -n ai-services
            kubectl rollout restart deployment -n java-tasks
            kubectl rollout status deployment -n ai-services --timeout=180s
            kubectl rollout status deployment -n java-tasks --timeout=180s
```

- [ ] **Step 2: Add Java GraphQL smoke test environment variable**

In the `smoke-production` job, add the GraphQL URL to the environment:

Find the current env block:

```yaml
        env:
          SMOKE_FRONTEND_URL: https://kylebradshaw.dev
          SMOKE_API_URL: https://api.kylebradshaw.dev
```

Replace with:

```yaml
        env:
          SMOKE_FRONTEND_URL: https://kylebradshaw.dev
          SMOKE_API_URL: https://api.kylebradshaw.dev
          SMOKE_GRAPHQL_URL: https://api.kylebradshaw.dev/graphql
```

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "feat(ci): update deploy job to use kubectl instead of docker compose"
```

---

### Task 11: Frontend Environment and Documentation

**Files:**
- Modify: `frontend/.env.local.example`
- Modify: `CLAUDE.md`

- [ ] **Step 1: Update frontend env example**

Read the current `frontend/.env.local.example`, then add the Java gateway and Google OAuth variables. The file should contain:

```
# Python AI services (ingestion, chat, debug)
NEXT_PUBLIC_API_URL=http://localhost:8000

# Java GraphQL gateway (task management)
# Production (Vercel): https://api.kylebradshaw.dev
NEXT_PUBLIC_GATEWAY_URL=http://localhost:8080

# Google OAuth (for Java task management login)
NEXT_PUBLIC_GOOGLE_CLIENT_ID=
```

- [ ] **Step 2: Update CLAUDE.md Infrastructure section**

In `CLAUDE.md`, update the Infrastructure section to document the K8s deployment. Find the current Infrastructure section and replace it with:

```markdown
## Infrastructure

- **Mac (dev machine):** Code editing, frontend dev server, no GPU
- **Windows (PC@100.79.113.84 via Tailscale):** Ollama (RTX 3090), Minikube (all backend services)
- **SSH:** `ssh PC@100.79.113.84` — key-based auth configured
- **Minikube:** All backend services run in Kubernetes on the Windows PC
  - `ai-services` namespace: Python AI services + Qdrant
  - `java-tasks` namespace: Java microservices + databases
  - `monitoring` namespace: Prometheus + Grafana
  - NGINX Ingress Controller routes all traffic by path
  - `minikube tunnel` exposes Ingress on localhost:80
- **Ollama:** Runs natively on Windows (GPU access), reached from K8s via ExternalName service
- **Local dev:** Docker Compose for both stacks (no Minikube needed for development)
  - SSH tunnel forwards `localhost:8000` to Windows nginx gateway
  ```bash
  ssh -f -N -L 8000:localhost:8000 PC@100.79.113.84
  ```
- **Frontend:** `npm run dev` in `frontend/`, points to `localhost:8000` via tunnel
- **Production:** Frontend on Vercel (`https://kylebradshaw.dev`), backend via Cloudflare Tunnel:
  - `https://api.kylebradshaw.dev` → Windows PC localhost:80 (Minikube Ingress)
  - Ingress routes by path: `/ingestion/*`, `/chat/*`, `/debug/*` → Python services; `/graphql`, `/api/auth/*` → Java services; `/grafana/*` → monitoring
  - Cloudflared installed as Windows service (auto-starts on boot)
  - `minikube tunnel` must be running as background process
```

- [ ] **Step 3: Update CLAUDE.md Project Structure**

Add the `k8s/` directory to the project structure section. Find the existing structure and add after the `nginx/` entry:

```markdown
k8s/                    # Kubernetes manifests — production deployment (Minikube)
├── ai-services/        # Python AI services + Qdrant namespace
├── monitoring/         # Prometheus + Grafana namespace
└── deploy.sh           # Unified deploy script for all namespaces
```

- [ ] **Step 4: Update CLAUDE.md Current State**

Add K8s deployment info to the Current State section. Add this bullet:

```markdown
- **K8s Deployment:** All services in Minikube (3 namespaces), NGINX Ingress Controller, unified deploy script
```

- [ ] **Step 5: Commit**

```bash
git add frontend/.env.local.example CLAUDE.md
git commit -m "docs: update frontend env example and CLAUDE.md for K8s deployment"
```

---

## Summary

**11 tasks** across 4 phases:

**Phase 1 — ai-services namespace:**
- Namespace + ConfigMaps (ingestion, chat, debug)
- Qdrant deployment + Ollama ExternalName service
- Python service deployments + services (3 services)
- Ingress with path rewriting and SSE/upload annotations

**Phase 2 — monitoring namespace:**
- Prometheus with cross-namespace scrape config
- Grafana with datasource provisioning and sub-path support
- Monitoring Ingress for Grafana

**Phase 3 — java-tasks updates:**
- ConfigMap updates (CORS, GraphiQL)
- Ingress for GraphQL, auth, RabbitMQ, health

**Phase 4 — deployment and CI/CD:**
- Unified deploy script (all 3 namespaces)
- CI deploy job updated to use kubectl
- Frontend env + CLAUDE.md documentation

**Not included (out of scope):**
- Helm charts (plain manifests for portfolio clarity)
- Persistent volumes (ephemeral storage acceptable for demo)
- ArgoCD/GitOps (SSH + kubectl is sufficient)
- Ollama in K8s (GPU passthrough impractical)
- Removing Docker Compose files (kept for local dev)
- Adding Java GraphQL smoke test code (env var added, test implementation is separate work)
