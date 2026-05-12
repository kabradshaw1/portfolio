# Postboot Recovery Design

## Goal

After a host power outage or hard reboot, the Debian runtime host should bring
the portfolio stack back to serving traffic without an operator manually
debugging Minikube DNS, image-pull backoff, or service readiness.

## Scope

This design adds a repo-owned recovery path for the existing Debian + Docker
Minikube host. It does not retag container images, change CI image publishing,
or convert production Deployments away from `imagePullPolicy: Always`; those
are separate follow-up hardening items.

## Architecture

The recovery path is split into three scripts:

- `scripts/ops/recover-after-host-boot.sh` performs mutating, idempotent
  recovery against Debian. It waits for required systemd services, waits for
  the Kubernetes node, repairs Minikube node DNS when registry resolution is
  broken, recycles only pods stuck in image-pull waiting states, and waits for
  application Deployments to become available.
- `scripts/ops/check-postboot-health.sh` is read-only verification. It checks
  Kubernetes readiness, confirms no active image-pull failures remain, verifies
  representative service endpoints, and probes representative external routes.
- `scripts/ops/install-postboot-recovery-systemd.sh` copies the committed
  recovery and health scripts over SSH into `/home/kyle/.local/lib/portfolio-postboot`
  on Debian, then installs systemd unit files that run those copied scripts.

The Debian host does not need to clone or pull this repository. It receives
only the runtime scripts that systemd needs. The source of truth remains the
committed scripts in this repo, and installation is performed from the Mac via
SSH.

## Recovery Behavior

The recovery script must be safe to run repeatedly. It may mutate only the
Minikube node resolver file and pods currently waiting with `ErrImagePull` or
`ImagePullBackOff`. It must not delete healthy pods, scale workloads, edit live
Secrets, apply manifests, purge queues, or mutate databases.

DNS repair writes this resolver set inside the Minikube node:

- `100.100.100.100` for the host's Tailscale DNS path
- `1.1.1.1`
- `8.8.8.8`
- `options ndots:0`

The script verifies `ghcr.io` and `registry-1.docker.io` resolution from inside
Minikube before recycling image-pull-blocked pods.

## Health Behavior

The health script is read-only. It verifies:

- the Minikube node is `Ready`
- all Deployments in production, QA, and monitoring namespaces are available
- no pods are currently waiting on `ErrImagePull` or `ImagePullBackOff`
- representative services have endpoints
- representative external URLs return expected non-5xx responses

The old failed backup verification Jobs are explicitly tolerated because they
pre-date this outage and are not serving-path workloads.

## Systemd Behavior

The installer copies scripts into `/home/kyle/.local/lib/portfolio-postboot`
and writes these units on Debian:

- `portfolio-postboot-recovery.service`: one-shot recovery after Docker,
  Tailscale, Minikube, minikube tunnel, and Cloudflare tunnel are expected to be
  online.
- `portfolio-postboot-health.service`: one-shot read-only health check.
- `portfolio-postboot-health.timer`: periodic health check after boot.

The installer enables the recovery service and the health timer. It may start
the timer immediately, but it must not run the recovery service during install;
operators can run it explicitly when needed.

## Verification

Local verification is shell syntax checking for the scripts. Runtime
verification is running the health script against Debian after install or after
a recovery event.
