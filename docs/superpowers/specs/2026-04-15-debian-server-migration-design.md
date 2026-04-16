# Debian 13 Server Migration Design

## Context

The Windows PC (100.79.113.84) that runs all backend infrastructure — Minikube, Ollama (RTX 3090), Cloudflare tunnel, and CI/CD target — is being fully replaced by a Debian 13 machine (100.82.52.82). The Debian machine is the same physical hardware (RTX 3090 GPU) with a fresh minimal install. Only SSH and Tailscale are configured so far. Nothing was saved from the Windows installation.

**Goal:** Replicate the full deployment pipeline on Debian with production-grade security hardening, then cut over CI/CD and Cloudflare to point at the new machine.

## Phase 1: OS Hardening & Security

**SSH lockdown:**
- Disable password authentication (`PasswordAuthentication no`)
- Disable root login (`PermitRootLogin no`)
- Ensure only key-based auth works (already configured for `kyle`)

**Firewall (UFW):**
- Default deny incoming, allow outgoing
- Allow SSH (port 22) from Tailscale subnet only (`100.64.0.0/10`)
- Allow port 80 from localhost only (Cloudflare tunnel connects locally)
- Allow port 11434 from localhost/Minikube network (Ollama)

**fail2ban:**
- Install and configure for SSH brute-force protection
- Default jail: 5 attempts, 10 min ban

**Unattended security updates:**
- Install `unattended-upgrades`
- Enable automatic security patches for Debian stable

**Set root password:**
- Fix the blank root password from the install

**Verification:** SSH in with key (works), try password auth (rejected), check UFW status, check fail2ban status.

## Phase 2: NVIDIA Drivers + Ollama

**NVIDIA drivers:**
- Add `non-free` and `non-free-firmware` to `/etc/apt/sources.list`
- Install `nvidia-driver` package (Debian 13 ships driver 535+)
- Reboot, verify with `nvidia-smi` (should show RTX 3090)

**CUDA toolkit:**
- Install `nvidia-cuda-toolkit` or CUDA from NVIDIA repos
- Verify with `nvcc --version`

**Ollama:**
- Install via official script: `curl -fsSL https://ollama.com/install.sh | sh`
- This creates a systemd service automatically
- Configure environment: `OLLAMA_HOST=0.0.0.0`, `OLLAMA_ORIGINS=*`
  - Edit `/etc/systemd/system/ollama.service.d/override.conf`
- Pull required models: `ollama pull qwen2.5:14b`, `ollama pull nomic-embed-text`
- Enable and start service

**Verification:** `nvidia-smi` shows GPU, `ollama run qwen2.5:14b "hello"` responds, `curl localhost:11434/api/tags` lists models.

## Phase 3: Docker + Minikube

**Docker Engine:**
- Install Docker CE from official Docker repos (not Debian's older package)
- Add `kyle` to `docker` group
- Enable and start `docker.service`

**Minikube:**
- Install latest minikube binary
- Start cluster: `minikube start --driver=docker --cpus=8 --memory=16g`
  - Machine has i7-11700K (8 cores/16 threads) and 32GB RAM — allocate half to Minikube
- Enable addons: `ingress`, `metrics-server`

**Minikube tunnel systemd service:**
- Create `/etc/systemd/system/minikube-tunnel.service`
- Runs `minikube tunnel` as the `kyle` user
- Starts after Docker, restarts on failure
- This exposes Ingress LoadBalancer on localhost:80

**kubectl:**
- Install kubectl (minikube bundles it, but standalone is better)
- Verify cluster access: `kubectl get nodes`

**host.minikube.internal:**
- On Linux with Docker driver, `host.minikube.internal` resolves to the host
- Verify Ollama is reachable from inside a pod: `kubectl run test --rm -it --image=curlimages/curl -- curl http://host.minikube.internal:11434/api/tags`

**Verification:** `docker ps` works without sudo, `kubectl get nodes` shows Ready, `minikube tunnel` exposes port 80, Ollama reachable from pods.

## Phase 4: Cloudflare Tunnel

**Install cloudflared:**
- Install from Cloudflare's apt repo
- Authenticate: `cloudflared tunnel login` (opens browser URL to authorize)

**Create new tunnel:**
- `cloudflared tunnel create debian-server`
- This generates credentials in `~/.cloudflared/`

**Configure tunnel routes:**
```yaml
tunnel: <tunnel-id>
credentials-file: /home/kyle/.cloudflared/<tunnel-id>.json

ingress:
  - hostname: api.kylebradshaw.dev
    service: http://localhost:80
  - hostname: qa-api.kylebradshaw.dev
    service: http://localhost:80
  - hostname: grafana.kylebradshaw.dev
    service: http://localhost:80
  - service: http_status:404
```

**DNS records:**
- `cloudflared tunnel route dns debian-server api.kylebradshaw.dev`
- `cloudflared tunnel route dns debian-server qa-api.kylebradshaw.dev`
- `cloudflared tunnel route dns debian-server grafana.kylebradshaw.dev`
- This creates CNAME records pointing to `<tunnel-id>.cfargotunnel.com`

**Systemd service:**
- `cloudflared service install`
- Move config to `/etc/cloudflared/config.yml`
- Enable and start

**Delete old Windows tunnel:**
- In Cloudflare Zero Trust dashboard, delete the old tunnel (ID `85f54326-...`)

**Verification:** `curl -H "Host: api.kylebradshaw.dev" https://api.kylebradshaw.dev` returns a response (will 502 until K8s services are deployed, but tunnel is working).

## Phase 5: Kubernetes Deployment

**Namespaces:**
```bash
kubectl create namespace ai-services
kubectl create namespace java-tasks
kubectl create namespace go-ecommerce
kubectl create namespace monitoring
kubectl create namespace ai-services-qa
kubectl create namespace java-tasks-qa
kubectl create namespace go-ecommerce-qa
```

**GHCR pull secret:**
- Create image pull secret in each namespace for `ghcr.io/kabradshaw1/portfolio`
- Requires a GitHub PAT with `read:packages` scope

**K8s secrets:**
- Recreate secrets from templates in `k8s/ai-services/secrets/`, `java/k8s/secrets/`, `go/k8s/secrets/`
- These include: DB passwords, JWT secret, API keys, RabbitMQ password

**Deploy manifests:**
- Run `k8s/deploy.sh` (or apply Kustomize overlays manually)
- Order: infrastructure (postgres, redis, rabbitmq, qdrant, mongodb) first, then services
- Run Go migration jobs before Go services start

**Pod Security Standards:**
- Label namespaces with `pod-security.kubernetes.io/enforce=baseline` (start with baseline, tighten to restricted later)

**Network Policies (production-grade):**
- Default-deny ingress in each namespace
- Allow ingress-controller to reach service pods
- Allow gateway services to reach downstream services
- Allow services to reach their databases

**Verification:** `kubectl get pods --all-namespaces` shows all pods Running, health endpoints respond via `kubectl port-forward`.

## Phase 6: CI/CD Pipeline Update

**GitHub Secrets to update:**
- `SSH_PRIVATE_KEY` — generate new deploy key pair on Debian, add public key to `kyle@debian:~/.ssh/authorized_keys`, update GitHub secret with private key
- `TAILSCALE_AUTHKEY` — may need new authkey if current one is tied to Windows node
- No changes needed for: `GITHUB_TOKEN`, `LLM_API_KEY`, `EMBEDDING_API_KEY`, `SMOKE_GO_PASSWORD`, `JWT_SECRET`

**Workflow changes (`.github/workflows/ci.yml`):**
- Update SSH target from `PC@100.79.113.84` to `kyle@100.82.52.82`
- Update any `host.minikube.internal` references if needed
- Verify `kubectl` path works over SSH on Debian (may differ from Windows)

**Kyle's sudo for CI:**
- CI deploys via SSH need `kubectl` access — `kyle` user needs kubectl configured
- May need passwordless sudo for specific commands (kubectl, minikube) or just ensure kubectl works without sudo

**Verification:** Push a test commit to `qa`, watch GitHub Actions deploy successfully to Debian.

## Phase 7: Smoke Test & Cutover

**End-to-end verification:**
- All health endpoints respond via Cloudflare tunnel URLs
- Frontend at `kylebradshaw.dev` can reach backend at `api.kylebradshaw.dev`
- QA endpoints work at `qa-api.kylebradshaw.dev`
- Grafana accessible at `grafana.kylebradshaw.dev`
- CI/CD deploys from both `qa` and `main` branches
- Ollama inference works (test chat and debug services)
- GPU utilization visible in `nvidia-smi` during inference

**Cleanup:**
- Remove Windows PC SSH config entry
- Update CLAUDE.md to reflect new machine (IP, user, OS)
- Update `docs/adr/deployment-architecture.md`

## Files to Modify

- `.github/workflows/ci.yml` — SSH target, deploy commands
- `CLAUDE.md` — Infrastructure section (IP, user, OS references)
- `docs/adr/deployment-architecture.md` — Architecture description
- `k8s/` — Potentially new NetworkPolicy manifests

## Files to Create (on Debian machine)

- `/etc/systemd/system/minikube-tunnel.service`
- `/etc/cloudflared/config.yml`
- `/etc/systemd/system/ollama.service.d/override.conf` (Ollama env overrides)
- UFW rules, fail2ban config, SSH config changes
- K8s secrets from templates
- NetworkPolicy manifests (new)

## Execution Notes

- Each phase should be completed and verified before starting the next
- Most work is done over SSH from the Mac (`ssh debian`)
- Some steps require interactive browser access (Cloudflare tunnel login) — Kyle will need to handle those on the Debian machine directly or via the Cloudflare dashboard
- The Tailscale IP `100.82.52.82` is already working and tested
