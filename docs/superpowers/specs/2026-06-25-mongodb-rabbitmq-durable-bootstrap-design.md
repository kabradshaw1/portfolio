# MongoDB durable bootstrap + RabbitMQ parity — Design

- **Date:** 2026-06-25
- **Issue:** #393 — `java-tasks/mongodb` has no durable storage or user bootstrap — wiped on every restart (caused Story prod outage)
- **Status:** Approved, ready for implementation planning

## Problem

`java/k8s/deployments/mongodb.yml` runs with **no persistent storage and no user
bootstrap**:

- No `volumes` / `volumeMounts` → `/data/db` lives on the container's ephemeral
  filesystem. Every pod restart or reschedule wipes all databases, documents, and
  users.
- No `MONGO_INITDB_ROOT_*` and no init scripts → no users are recreated at startup.

Dependent apps that authenticate as a per-service Mongo user then fail with
`AuthenticationFailed` / `UserNotFound` until someone manually re-provisions the
user. This caused a GalaxyVoyagers / Story production outage on 2026-06-25:
`story-chat` → MongoDB failed with `UserNotFound: Could not find user "story_chat"
for db "story_chat"` ~49 min after a `mongodb` pod restart, breaking every page
that loads comments. Recovery required manually running
`GalaxyVoyagers/scripts/ops/provision-story-broker-users.sh`.

RabbitMQ has the same class of gap: it loads users/vhosts declaratively from a
ConfigMap (good — those survive restart), but has **no PVC for `/var/lib/rabbitmq`**
(mnesia / message state is lost on restart) and the Story `story` user/vhost is
**not** in `definitions.json` (also provisioned out-of-band by the recovery script).

## Goals

- MongoDB data and the `story_chat` user survive pod restarts and reschedules.
- RabbitMQ mnesia state survives restarts, and the Story `story` user/vhost is
  self-healing and declarative.
- No real secret values committed to git.
- Mirror the existing postgres durability pattern; minimize blast radius on
  in-cluster services.

## Non-goals

- **No enabling of MongoDB auth.** Mongo currently runs with auth OFF —
  `activity-service` connects unauthenticated (`MONGODB_HOST: mongodb`, no
  credentials). Setting `MONGO_INITDB_ROOT_USERNAME/PASSWORD` would enable `--auth`
  and break `activity-service`. We keep auth off and bootstrap only the external
  `story_chat` app user (clients that supply credentials still authenticate against
  a created user even when the server does not require auth).
- **No StatefulSet conversion.** Mongo stays a `Deployment` with `strategy: Recreate`,
  matching `postgres` and keeping the AWS overlay's `kind: Deployment` delete patch
  working.
- **No `activity-service` changes** (code or config).
- **No removal** of the interim `provision-story-broker-users.sh` /
  `post-outage-recover.sh` scripts — they remain a manual fallback (see Edge cases).

## Existing pattern being mirrored

`postgres` already solves durable bootstrap in this repo:

- `Deployment` + `strategy: Recreate` + `PersistentVolumeClaim` (`postgres-data`)
  mounted at the data dir via `subPath`.
- An `initdb` ConfigMap mounted at `/docker-entrypoint-initdb.d` that runs once on a
  fresh volume.
- Credentials via `secretKeyRef` → the `java-secrets` Secret, which is applied
  out-of-band by `deploy.sh` (`kubectl apply -f secrets/java-secrets.yml`) and is
  **not committed** to the repo.
- The AWS overlay's `overlays/aws/patches/remove-infra.yaml` deletes the in-cluster
  DB Deployment/Service/PVC/ConfigMap because AWS managed services (RDS, Atlas,
  Amazon MQ) replace them.

## Design

### Part A — MongoDB durability

**A1. Persist data.** New file `java/k8s/volumes/mongodb-pvc.yml`:

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

**A2. Mount it + survive rolling restarts.** Edit `deployments/mongodb.yml`:

- Add `spec.strategy: { type: Recreate }` — a single RWO volume cannot be mounted by
  two pods at once; `Recreate` tears down the old pod before starting the new one.
- Add `spec.template.spec.securityContext: { fsGroup: 999 }` — the `mongo:7` image
  runs as uid/gid `999` (`mongodb`); `fsGroup` makes the mounted volume group-writable
  by that user.
- Add `volumeMounts` on the `mongodb` container:
  - `mongodb-data` → `/data/db` (`subPath: data`)
  - `mongodb-initdb` → `/docker-entrypoint-initdb.d`
- Add `volumes`:
  - `mongodb-data` → `persistentVolumeClaim: { claimName: mongodb-data }`
  - `mongodb-initdb` → `configMap: { name: mongodb-initdb, defaultMode: 0755 }`
- Add `env` on the `mongodb` container (used only by the init script):
  - `MONGO_STORY_CHAT_DB: story_chat` (plain value)
  - `MONGO_STORY_CHAT_USER: story_chat` (plain value)
  - `MONGO_STORY_CHAT_PASSWORD` → `secretKeyRef: { name: java-secrets, key: mongo-story-chat-password }`
- Keep existing `readinessProbe`, `livenessProbe`, `ports`, and `resources`
  unchanged.

**A3. Bootstrap `story_chat` at init time.** New file
`java/k8s/configmaps/mongodb-initdb.yml` — a ConfigMap named `mongodb-initdb`
containing one executable shell script. The `mongo` docker entrypoint runs
`/docker-entrypoint-initdb.d/*.sh` exactly once, against a temporary `mongod`, when
the data dir is empty (i.e. on a fresh PVC). Combined with the PVC, this becomes a
one-time provision that thereafter persists.

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: mongodb-initdb
  namespace: java-tasks
data:
  01-create-story-chat-user.sh: |
    #!/usr/bin/env bash
    # Runs once, on first boot against a fresh PVC. Creates the external
    # Story app user (story_chat) so it survives restarts. Mongo runs with
    # auth disabled; clients that supply credentials still authenticate
    # against a created user. Password is injected from java-secrets via env.
    set -euo pipefail
    mongosh --quiet <<EOF
    db.getSiblingDB("${MONGO_STORY_CHAT_DB}").createUser({
      user: "${MONGO_STORY_CHAT_USER}",
      pwd: "${MONGO_STORY_CHAT_PASSWORD}",
      roles: [{ role: "readWrite", db: "${MONGO_STORY_CHAT_DB}" }]
    })
    EOF
```

Rationale for a `.sh` (not `.js`) init script: the entrypoint passes `.js` files to
mongosh without shell-level env expansion, so injecting a secret cleanly requires a
shell heredoc that expands `${MONGO_STORY_CHAT_*}` from the container env.

### Part B — RabbitMQ parity

**B1. Persist mnesia.** New file `java/k8s/volumes/rabbitmq-pvc.yml`:

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

Edit `deployments/rabbitmq.yml`:

- Add `spec.strategy: { type: Recreate }` (same RWO reasoning as Mongo; the current
  default `RollingUpdate` would deadlock on the new volume).
- Add a `volumeMount` `rabbitmq-data` → `/var/lib/rabbitmq` and the corresponding
  `volumes` entry referencing the `rabbitmq-data` PVC.

**B2. Self-healing, secret-safe `story` user.** RabbitMQ's `load_definitions`
reloads `definitions.json` on **every** boot. That means:

- We cannot commit a placeholder password for `story` — it would reset the real
  password on every restart, breaking the live Story app.
- We cannot commit the real password — that violates the no-committed-secrets rule.

So the password must be injected from `java-secrets` at load time. Approach:

- Convert the `definitions.json` key in the `rabbitmq-definitions` ConfigMap into a
  **template** (`definitions.json.tmpl`) that references `${STORY_RABBIT_PASSWORD}`
  for the `story` user, and adds the `story` vhost + permissions declaratively
  (non-secret). The existing `guest` user and `/` + `qa` vhosts stay as-is.
- Add an **initContainer** to `deployments/rabbitmq.yml` that runs `envsubst` over
  the template, writing a rendered `definitions.json` into a shared `emptyDir`
  volume. The main container loads the rendered file (its `rabbitmq.conf` already
  points `management.load_definitions` at `/etc/rabbitmq/definitions.json`).
- The initContainer gets `STORY_RABBIT_PASSWORD` via `secretKeyRef` →
  `java-secrets` (key `rabbit-story-password`).

Template (`definitions.json.tmpl`) adds to the existing structure:

```json
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
```

Volume wiring:

- `definitions` ConfigMap → mounted read-only into the initContainer at a template
  path (e.g. `/templates/definitions.json.tmpl` via `subPath`), plus the existing
  `rabbitmq.conf` / `enabled_plugins` mounts on the main container (unchanged).
- New `emptyDir` `rendered-definitions` → mounted writable in the initContainer (it
  writes the rendered file there) and read-only in the main container at
  `/etc/rabbitmq/definitions.json` (`subPath: definitions.json`), replacing any
  direct ConfigMap mount of the rendered file.

initContainer (decided): image `busybox:1.36` (tiny, no extra heavy pull, `sed`
always present — alpine-based images do not ship `gettext`/`envsubst` by default).
Command:

```sh
sed "s|\${STORY_RABBIT_PASSWORD}|$STORY_RABBIT_PASSWORD|" \
  /templates/definitions.json.tmpl > /rendered/definitions.json
```

Delimiter `|` is chosen for the `sed` substitution; the `rabbit-story-password`
value must therefore not contain a literal `|` (documented alongside the secret
keys). `STORY_RABBIT_PASSWORD` is provided to the initContainer via `secretKeyRef`.

### Part C — Wiring & operational docs

**C1. `kustomization.yaml`.** Add the three new resources:

- `volumes/mongodb-pvc.yml`
- `volumes/rabbitmq-pvc.yml`
- `configmaps/mongodb-initdb.yml`

**C2. AWS overlay `overlays/aws/patches/remove-infra.yaml`.** Add `$patch: delete`
entries for the new in-cluster-only resources (Atlas / Amazon MQ replace them):

- `PersistentVolumeClaim` `mongodb-data`
- `ConfigMap` `mongodb-initdb`
- `PersistentVolumeClaim` `rabbitmq-data`

**C3. Secrets coordination (deploy prerequisite — values NOT committed).** Two new
keys must be added to the out-of-band `java-secrets` Secret
(`java/k8s/secrets/java-secrets.yml`, gitignored / applied by `deploy.sh`):

- `mongo-story-chat-password`
- `rabbit-story-password`

Their values must match the corresponding Story credentials in the GalaxyVoyagers
`story-secrets` Secret. This is a cross-repo coordination requirement; the
implementation will document it (in `deploy.sh` comments and/or a short README note)
but will **not** write the real values. Per the no-committed-secrets rule: the
presence of committed placeholders elsewhere is not authorization to write the same
value to prod — the gap is flagged here explicitly.

## Edge cases & limitations

- **Fresh volume vs. existing volume.** The Mongo init script and the RabbitMQ
  rendered definitions fully self-heal on a *fresh* volume. On an *existing* volume,
  durability comes from the PVC itself (Mongo init scripts do not re-run on a
  non-empty data dir; RabbitMQ definitions still reload every boot, so the `story`
  user self-heals regardless).
- **User deleted on a live volume.** If someone deletes the `story_chat` Mongo user
  on a populated volume, the init script will not re-run. The interim
  `provision-story-broker-users.sh` remains the manual fallback for this case. (Not a
  regression — it is strictly better than today, where every restart wiped
  everything.)
- **RWO + Recreate downtime.** `Recreate` causes a brief outage during pod
  replacement (old pod stops before new pod starts). Acceptable for single-replica
  infra and consistent with `postgres`.

## Files touched

New:
- `java/k8s/volumes/mongodb-pvc.yml`
- `java/k8s/volumes/rabbitmq-pvc.yml`
- `java/k8s/configmaps/mongodb-initdb.yml`

Edited:
- `java/k8s/deployments/mongodb.yml`
- `java/k8s/deployments/rabbitmq.yml`
- `java/k8s/configmaps/rabbitmq-definitions.yml` (definitions → template + story user/vhost/permissions)
- `java/k8s/kustomization.yaml`
- `java/k8s/overlays/aws/patches/remove-infra.yaml`
- `java/k8s/deploy.sh` (document new java-secrets keys) and/or a short README note

## Verification

- `kustomize build java/k8s` (base) and `kustomize build java/k8s/overlays/aws`
  both render without error, and the AWS overlay shows the new PVCs/ConfigMap
  removed.
- On a fresh minikube apply: `mongodb` and `rabbitmq` pods become Ready; the
  `story_chat` Mongo user exists (`db.getSiblingDB("story_chat").getUsers()`); the
  `story` RabbitMQ user/vhost exist with the injected password.
- Restart both pods (`kubectl rollout restart`): data and users persist; no manual
  provisioning needed.
