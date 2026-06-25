# MongoDB durable bootstrap + RabbitMQ parity — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `java-tasks/mongodb` and `rabbitmq` survive pod restarts (durable storage) and self-heal their external Story users without manual out-of-band provisioning.

**Architecture:** Mirror the existing `postgres` durability pattern — `Deployment` + `strategy: Recreate` + a `PersistentVolumeClaim` mounted at the data dir, plus init-time user bootstrap. Mongo keeps auth OFF and creates the `story_chat` app user via a `/docker-entrypoint-initdb.d` shell script. RabbitMQ gets a PVC for mnesia and an initContainer that renders `definitions.json` from a committed template, injecting the `story` user password from the out-of-band `java-secrets` Secret so no real secret is committed.

**Tech Stack:** Kubernetes manifests, Kustomize (via `kubectl kustomize`), `mongo:7`, `rabbitmq:3-management-alpine`, `busybox:1.36` (initContainer), pre-commit `gitleaks`.

## Global Constraints

- Namespace for all resources: `java-tasks`.
- **No real secret values committed.** Passwords come from the `java-secrets` Secret (applied out-of-band by `deploy.sh`, not in git) via `secretKeyRef`. New keys: `mongo-story-chat-password`, `rabbit-story-password`.
- **Do not enable MongoDB auth.** Do not set `MONGO_INITDB_ROOT_USERNAME`/`MONGO_INITDB_ROOT_PASSWORD`; `activity-service` connects unauthenticated and must stay working.
- Mongo and Rabbit stay `Deployment` (not StatefulSet) with `strategy: { type: Recreate }` — a single `ReadWriteOnce` volume cannot be held by two pods at once.
- PVC convention: `accessModes: [ReadWriteOnce]`, `storageClassName: standard`, `storage: 2Gi` (matches `postgres-pvc.yml`).
- Every new resource must be registered in `java/k8s/kustomization.yaml` and, if it is in-cluster-only infra, deleted in `java/k8s/overlays/aws/patches/remove-infra.yaml` (AWS uses Atlas / Amazon MQ).
- Validation command (base): `kubectl kustomize java/k8s`. Validation command (AWS overlay): `kubectl kustomize java/k8s/overlays/aws`. Both must render without error after every task.
- Commit messages end with the issue ref `(#393)` and the Co-Authored-By trailer used in this repo.

---

### Task 1: MongoDB durable bootstrap

**Files:**
- Create: `java/k8s/volumes/mongodb-pvc.yml`
- Create: `java/k8s/configmaps/mongodb-initdb.yml`
- Modify: `java/k8s/deployments/mongodb.yml` (full rewrite below)
- Modify: `java/k8s/kustomization.yaml` (register two new resources)
- Modify: `java/k8s/overlays/aws/patches/remove-infra.yaml` (delete the two new in-cluster-only resources)

**Interfaces:**
- Consumes: `java-secrets` Secret key `mongo-story-chat-password` (documented in Task 3; not required to exist for kustomize rendering).
- Produces: PVC `mongodb-data`, ConfigMap `mongodb-initdb`. No later task depends on these.

- [ ] **Step 1: Confirm the current state fails the goal**

Run: `kubectl kustomize java/k8s | grep -A3 'name: mongodb' | grep -c 'persistentVolumeClaim'`
Expected: `0` (mongodb has no PVC today).

- [ ] **Step 2: Create the PVC**

Create `java/k8s/volumes/mongodb-pvc.yml`:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: mongodb-data
  namespace: java-tasks
spec:
  accessModes:
    - ReadWriteOnce
  storageClassName: standard
  resources:
    requests:
      storage: 2Gi
```

- [ ] **Step 3: Create the init-script ConfigMap**

Create `java/k8s/configmaps/mongodb-initdb.yml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: mongodb-initdb
  namespace: java-tasks
data:
  # Runs once, on first boot against a fresh PVC. Creates the external Story
  # app user (story_chat) so it survives restarts. Mongo runs with auth
  # disabled; clients that supply credentials still authenticate against a
  # created user. Password is injected from java-secrets via env (see the
  # MONGO_STORY_CHAT_* env on the deployment).
  01-create-story-chat-user.sh: |
    #!/usr/bin/env bash
    set -euo pipefail
    mongosh --quiet <<EOF
    db.getSiblingDB("${MONGO_STORY_CHAT_DB}").createUser({
      user: "${MONGO_STORY_CHAT_USER}",
      pwd: "${MONGO_STORY_CHAT_PASSWORD}",
      roles: [{ role: "readWrite", db: "${MONGO_STORY_CHAT_DB}" }]
    })
    EOF
```

- [ ] **Step 4: Rewrite the mongodb Deployment**

Replace the entire contents of `java/k8s/deployments/mongodb.yml` with:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mongodb
  namespace: java-tasks
spec:
  replicas: 1
  strategy:
    type: Recreate
  selector:
    matchLabels:
      app: mongodb
  template:
    metadata:
      labels:
        app: mongodb
    spec:
      securityContext:
        fsGroup: 999
      containers:
        - name: mongodb
          image: mongo:7
          ports:
            - containerPort: 27017
          env:
            - name: MONGO_STORY_CHAT_DB
              value: story_chat
            - name: MONGO_STORY_CHAT_USER
              value: story_chat
            - name: MONGO_STORY_CHAT_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: java-secrets
                  key: mongo-story-chat-password
          readinessProbe:
            exec:
              command:
                - mongosh
                - --quiet
                - --eval
                - "db.runCommand({ ping: 1 }).ok"
            initialDelaySeconds: 10
            periodSeconds: 10
            timeoutSeconds: 5
            failureThreshold: 3
          livenessProbe:
            exec:
              command:
                - mongosh
                - --quiet
                - --eval
                - "db.runCommand({ ping: 1 }).ok"
            initialDelaySeconds: 30
            periodSeconds: 20
            timeoutSeconds: 5
            failureThreshold: 3
          volumeMounts:
            - name: mongodb-data
              mountPath: /data/db
              subPath: data
            - name: mongodb-initdb
              mountPath: /docker-entrypoint-initdb.d
          resources:
            requests:
              memory: "128Mi"
              cpu: "100m"
            limits:
              memory: "512Mi"
              cpu: "500m"
      volumes:
        - name: mongodb-data
          persistentVolumeClaim:
            claimName: mongodb-data
        - name: mongodb-initdb
          configMap:
            name: mongodb-initdb
            defaultMode: 0755
```

- [ ] **Step 5: Register the new resources in kustomization**

In `java/k8s/kustomization.yaml`, under `resources:`, add `configmaps/mongodb-initdb.yml` next to the other `configmaps/` entries, and add `volumes/mongodb-pvc.yml` next to the other `volumes/` entries:

```yaml
  - configmaps/mongodb-initdb.yml
```
```yaml
  - volumes/mongodb-pvc.yml
```

- [ ] **Step 6: Delete the new resources in the AWS overlay**

In `java/k8s/overlays/aws/patches/remove-infra.yaml`, append (Atlas replaces in-cluster Mongo):

```yaml
---
# Remove mongodb PVC — Atlas handles storage
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: mongodb-data
  namespace: java-tasks
$patch: delete
---
# Remove mongodb-initdb ConfigMap — Atlas handles initialization
apiVersion: v1
kind: ConfigMap
metadata:
  name: mongodb-initdb
  namespace: java-tasks
$patch: delete
```

- [ ] **Step 7: Validate base render (PVC + initdb wired, deployment mounts present)**

Run:
```bash
kubectl kustomize java/k8s > /tmp/base.yaml && \
grep -c 'name: mongodb-data' /tmp/base.yaml && \
grep -c 'name: mongodb-initdb' /tmp/base.yaml && \
grep -c 'MONGO_STORY_CHAT_PASSWORD' /tmp/base.yaml && \
grep -c 'type: Recreate' /tmp/base.yaml
```
Expected: command exits 0; each count is ≥ 1 (mongodb-data and mongodb-initdb each appear in both the resource and the deployment volume → ≥ 2; `type: Recreate` includes postgres + mongodb → ≥ 2).

- [ ] **Step 8: Validate AWS overlay render (new infra removed)**

Run:
```bash
kubectl kustomize java/k8s/overlays/aws > /tmp/aws.yaml && \
grep -c 'name: mongodb-data' /tmp/aws.yaml; \
grep -c 'name: mongodb-initdb' /tmp/aws.yaml
```
Expected: overlay renders without error; both counts are `0` (resources deleted by the overlay).

- [ ] **Step 9: Commit**

```bash
git add java/k8s/volumes/mongodb-pvc.yml java/k8s/configmaps/mongodb-initdb.yml \
        java/k8s/deployments/mongodb.yml java/k8s/kustomization.yaml \
        java/k8s/overlays/aws/patches/remove-infra.yaml
git commit -m "feat(k8s): durable storage + story_chat bootstrap for mongodb (#393)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: RabbitMQ mnesia persistence + self-healing story user

**Files:**
- Create: `java/k8s/volumes/rabbitmq-pvc.yml`
- Modify: `java/k8s/configmaps/rabbitmq-definitions.yml` (rename `definitions.json` key to `definitions.json.tmpl`, add `story` user/vhost/permissions)
- Modify: `java/k8s/deployments/rabbitmq.yml` (full rewrite below — adds Recreate, mnesia PVC, render initContainer, rendered-definitions emptyDir)
- Modify: `java/k8s/kustomization.yaml` (register the PVC)
- Modify: `java/k8s/overlays/aws/patches/remove-infra.yaml` (delete the PVC)

**Interfaces:**
- Consumes: `java-secrets` Secret key `rabbit-story-password` (documented in Task 3; not required for kustomize rendering).
- Produces: PVC `rabbitmq-data`; ConfigMap `rabbitmq-definitions` now exposes key `definitions.json.tmpl` (template) instead of `definitions.json`. No later task depends on these.

- [ ] **Step 1: Confirm the current state fails the goal**

Run: `kubectl kustomize java/k8s | grep -A40 'kind: Deployment' | grep -A40 'name: rabbitmq' | grep -c '/var/lib/rabbitmq'`
Expected: `0` (rabbitmq has no mnesia volume today).

- [ ] **Step 2: Create the mnesia PVC**

Create `java/k8s/volumes/rabbitmq-pvc.yml`:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: rabbitmq-data
  namespace: java-tasks
spec:
  accessModes:
    - ReadWriteOnce
  storageClassName: standard
  resources:
    requests:
      storage: 2Gi
```

- [ ] **Step 3: Convert definitions to a template with the story user**

Replace the entire contents of `java/k8s/configmaps/rabbitmq-definitions.yml` with (note the key is now `definitions.json.tmpl` and contains the `${STORY_RABBIT_PASSWORD}` token; `rabbitmq.conf` and `enabled_plugins` are unchanged):

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: rabbitmq-definitions
  namespace: java-tasks
data:
  # Template rendered at pod start by the render-definitions initContainer,
  # which substitutes ${STORY_RABBIT_PASSWORD} from java-secrets. The story
  # user password is therefore never committed. RabbitMQ reloads definitions
  # on every boot, so the story user/vhost self-heal even on a fresh volume.
  definitions.json.tmpl: |
    {
      "users": [
        {"name": "guest", "password": "guest", "tags": ["administrator"]},
        {"name": "story", "password": "${STORY_RABBIT_PASSWORD}", "tags": []}
      ],
      "vhosts": [
        {"name": "/"},
        {"name": "qa"},
        {"name": "story"}
      ],
      "permissions": [
        {"user": "guest", "vhost": "/", "configure": ".*", "write": ".*", "read": ".*"},
        {"user": "guest", "vhost": "qa", "configure": ".*", "write": ".*", "read": ".*"},
        {"user": "story", "vhost": "story", "configure": ".*", "write": ".*", "read": ".*"}
      ]
    }
  rabbitmq.conf: |
    management.load_definitions = /etc/rabbitmq/definitions.json
  enabled_plugins: |
    [rabbitmq_management,rabbitmq_prometheus].
```

- [ ] **Step 4: Rewrite the rabbitmq Deployment**

Replace the entire contents of `java/k8s/deployments/rabbitmq.yml` with:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: rabbitmq
  namespace: java-tasks
spec:
  replicas: 1
  strategy:
    type: Recreate
  selector:
    matchLabels:
      app: rabbitmq
  template:
    metadata:
      labels:
        app: rabbitmq
    spec:
      initContainers:
        - name: render-definitions
          image: busybox:1.36
          # Inject the story password (from java-secrets) into the committed
          # template. '|' is the sed delimiter, so rabbit-story-password must
          # not contain a literal '|'. \$ keeps the token literal for matching;
          # $STORY_RABBIT_PASSWORD expands to the real value.
          command:
            - sh
            - -c
            - 'sed "s|\${STORY_RABBIT_PASSWORD}|$STORY_RABBIT_PASSWORD|" /templates/definitions.json.tmpl > /rendered/definitions.json'
          env:
            - name: STORY_RABBIT_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: java-secrets
                  key: rabbit-story-password
          volumeMounts:
            - name: definitions
              mountPath: /templates/definitions.json.tmpl
              subPath: definitions.json.tmpl
              readOnly: true
            - name: rendered-definitions
              mountPath: /rendered
      containers:
        - name: rabbitmq
          image: rabbitmq:3-management-alpine
          env:
            - name: RABBITMQ_DEFAULT_USER
              value: "guest"
            - name: RABBITMQ_DEFAULT_PASS
              value: "guest"
          ports:
            - name: amqp
              containerPort: 5672
            - name: management
              containerPort: 15672
            - name: prometheus
              containerPort: 15692
          volumeMounts:
            - name: rendered-definitions
              mountPath: /etc/rabbitmq/definitions.json
              subPath: definitions.json
              readOnly: true
            - name: definitions
              mountPath: /etc/rabbitmq/conf.d/20-definitions.conf
              subPath: rabbitmq.conf
            - name: definitions
              mountPath: /etc/rabbitmq/enabled_plugins
              subPath: enabled_plugins
            - name: rabbitmq-data
              mountPath: /var/lib/rabbitmq
          resources:
            requests:
              memory: "128Mi"
              cpu: "100m"
            limits:
              memory: "512Mi"
              cpu: "500m"
      volumes:
        - name: definitions
          configMap:
            name: rabbitmq-definitions
        - name: rendered-definitions
          emptyDir: {}
        - name: rabbitmq-data
          persistentVolumeClaim:
            claimName: rabbitmq-data
```

- [ ] **Step 5: Register the PVC in kustomization**

In `java/k8s/kustomization.yaml`, under `resources:`, add next to the other `volumes/` entries:

```yaml
  - volumes/rabbitmq-pvc.yml
```

- [ ] **Step 6: Delete the PVC in the AWS overlay**

In `java/k8s/overlays/aws/patches/remove-infra.yaml`, append (Amazon MQ replaces in-cluster RabbitMQ):

```yaml
---
# Remove rabbitmq PVC — Amazon MQ handles storage
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: rabbitmq-data
  namespace: java-tasks
$patch: delete
```

- [ ] **Step 7: Validate base render (mnesia volume, initContainer, story user)**

Run:
```bash
kubectl kustomize java/k8s > /tmp/base.yaml && \
grep -c 'name: rabbitmq-data' /tmp/base.yaml && \
grep -c 'render-definitions' /tmp/base.yaml && \
grep -c 'definitions.json.tmpl' /tmp/base.yaml && \
grep -c '"name": "story"' /tmp/base.yaml && \
grep -c 'STORY_RABBIT_PASSWORD' /tmp/base.yaml
```
Expected: command exits 0; every count is ≥ 1. Confirm no literal password is present: `grep -c 'rabbit-story-password' /tmp/base.yaml` shows the secret **key name** only (in `secretKeyRef`), never a value.

- [ ] **Step 8: Validate AWS overlay render (PVC removed)**

Run:
```bash
kubectl kustomize java/k8s/overlays/aws > /tmp/aws.yaml && \
grep -c 'name: rabbitmq-data' /tmp/aws.yaml
```
Expected: overlay renders without error; count is `0`.

- [ ] **Step 9: Commit**

```bash
git add java/k8s/volumes/rabbitmq-pvc.yml java/k8s/configmaps/rabbitmq-definitions.yml \
        java/k8s/deployments/rabbitmq.yml java/k8s/kustomization.yaml \
        java/k8s/overlays/aws/patches/remove-infra.yaml
git commit -m "feat(k8s): mnesia PVC + self-healing story user for rabbitmq (#393)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Document the new java-secrets keys (deploy prerequisite)

**Files:**
- Modify: `java/k8s/deploy.sh` (document required secret keys near the secrets-apply step)

**Interfaces:**
- Consumes: nothing.
- Produces: operator documentation. No code depends on this.

- [ ] **Step 1: Add a documentation comment to deploy.sh**

In `java/k8s/deploy.sh`, immediately above the `echo "==> Applying secrets..."` line, insert this comment block:

```bash
# java-secrets (secrets/java-secrets.yml, NOT committed) must include these keys,
# in addition to postgres-password:
#   mongo-story-chat-password  — password for the Mongo story_chat app user
#   rabbit-story-password      — password for the RabbitMQ story user
#                                (must NOT contain a literal '|')
# Values must match the corresponding Story credentials in the GalaxyVoyagers
# story-secrets Secret. Coordinate cross-repo; do not commit real values here.
```

- [ ] **Step 2: Verify no secret values were introduced**

Run: `pre-commit run gitleaks --files java/k8s/deploy.sh`
Expected: `Passed` (only key names and prose, no secret values).

- [ ] **Step 3: Commit**

```bash
git add java/k8s/deploy.sh
git commit -m "docs(k8s): document mongo/rabbit story secret keys for deploy (#393)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Final verification (after all tasks)

- [ ] `kubectl kustomize java/k8s` renders without error.
- [ ] `kubectl kustomize java/k8s/overlays/aws` renders without error and contains none of: `mongodb-data`, `mongodb-initdb`, `rabbitmq-data`.
- [ ] `pre-commit run gitleaks --all-files` passes (no committed secret values).
- [ ] (Optional, requires a cluster) On a fresh minikube apply: both pods reach Ready; `kubectl exec deploy/mongodb -n java-tasks -- mongosh --quiet --eval 'db.getSiblingDB("story_chat").getUsers()'` lists `story_chat`; the RabbitMQ `story` user/vhost exist. `kubectl rollout restart deploy/mongodb deploy/rabbitmq -n java-tasks` preserves data and users.
