# Kubernetes And Runtime Infrastructure

Debian 13 (`ssh debian`) hosts Ollama, Minikube, observability, and runtime
services. It is runtime infrastructure, not a build or test worker.

## Namespaces

- `ai-services` - Python AI services and Qdrant
- `java-tasks` - Java services and databases
- `go-ecommerce` - Go auth and ecommerce services
- `monitoring` - Prometheus, Grafana, Loki, Promtail, Jaeger, exporters
- `ai-services-qa`, `java-tasks-qa`, `go-ecommerce-qa` - QA service copies

## Local And Production Routing

Local frontend development uses an SSH tunnel:

```bash
ssh -f -N -L 8000:localhost:8000 debian
```

Production traffic reaches Debian Minikube through Cloudflare Tunnel:
`https://api.kylebradshaw.dev` routes to Minikube Ingress.

Ingress routes by path:

- `/ingestion/*`, `/chat/*`, `/debug/*` - Python services
- `/graphql`, `/api/auth/*` - Java services
- `/go-api/*`, `/go-auth/*`, `/go-products/*` - Go services
- `/grafana/*` - monitoring

`minikube tunnel` and `cloudflared` run as systemd services and auto-start on
boot. Minikube memory is fixed at 16Gi unless the cluster is deleted and
recreated, which wipes cluster state.

## Shared Environment Safety

Before any mutating shared-environment action, use the `ops-as-code` skill.
This includes `kubectl apply/exec/rollout/scale/delete`, database mutations,
secret edits, queue purges, or one-off production fixes.

Application secrets are committed as `SealedSecret` resources in
`k8s/secrets/<namespace>/<name>.sealed.yml`. Do not edit live Kubernetes
Secrets, do not create app Secrets imperatively in CI, and do not put
credentials in ConfigMap data.

Use `scripts/seal-from-cluster.sh` when a sealed secret must be regenerated
from cluster state.

## Config And Migrations

Credentials never go in ConfigMap data. Split DSNs into ConfigMap host, port,
database, and options plus Secret user and password; applications assemble the
connection string at startup.

Go migration Jobs bypass PgBouncer and use `DATABASE_URL_DIRECT`. App
Deployments use `DATABASE_URL` through PgBouncer. `sslmode=disable` is required
for Go PostgreSQL URLs.

## Cert-Manager And gRPC mTLS

cert-manager resources live under `k8s/cert-manager/`. Go gRPC services use
per-service TLS secrets mounted at `/etc/tls/` and switch to mTLS when
`TLS_CERT_DIR` is set.

If a gRPC call fails with an authentication handshake error, check:

```bash
kubectl get pods -n cert-manager
kubectl get certificates -n <namespace>
```

Then inspect service logs for `mTLS enabled` to catch stale images.

## Observability

Critical datasource UIDs:

- Prometheus: `PBFA97CFB590B2093`
- Loki: `loki`
- Jaeger: `jaeger`

For alerts, running pods with errors, circuit breakers, saga issues, gRPC
failures, or post-incident verification, use `debug-observability` before ad
hoc log inspection.

Pods down or CrashLoopBackOff are the exception: start with `kubectl get pods`
and `kubectl logs`, then use observability after the target is running.

When creating or editing Grafana alerting or dashboard provisioning under
`k8s/monitoring/configmaps/` and `k8s/monitoring/deployments/grafana.yml`, load
the `debug-observability` skill first. If the change needs to touch the shared
cluster, load `ops-as-code` and use a committed script or manifest before
running any mutation.

Grafana provisioning files are mounted with `subPath`, so ConfigMap updates do
not refresh inside an already-running Grafana pod. Alerting ConfigMap changes
must include a rollout trigger in the Grafana Deployment annotation and must be
verified against Grafana's live provisioning API after rollout. For the current
alerting reload procedure, use
`scripts/ops/2026-05-09-reload-grafana-alerting.sh`.
