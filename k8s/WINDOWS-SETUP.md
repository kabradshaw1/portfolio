# Windows PC Setup Guide — Kubernetes Deployment

This guide walks through setting up the Minikube-based Kubernetes deployment on the Windows PC (PC@100.79.113.84).

## Prerequisites

- **Docker Desktop** installed and running (Minikube uses it as its container runtime)
- **Ollama** installed and running (serves LLM inference via GPU)
- **Cloudflared** installed as a Windows service (Cloudflare Tunnel)
- **Git** — repo cloned and on the `feat/debug-assistant` branch (or later, `main`)

## Quick Start

Run the setup script as Administrator:

```powershell
# From the repo directory
powershell -ExecutionPolicy Bypass -File k8s/setup-windows.ps1
```

The script handles steps 1-6 below automatically. Step 7 (Cloudflare Tunnel) and step 8 (minikube tunnel persistence) require manual configuration.

## Step-by-Step

### 1. Install Minikube

```powershell
winget install Kubernetes.minikube
```

Minikube creates a single-node Kubernetes cluster inside a Docker container. It's the simplest way to run K8s locally.

### 2. Install kubectl

```powershell
winget install Kubernetes.kubectl
```

kubectl is the CLI for interacting with the K8s cluster. Minikube bundles its own, but having a standalone install avoids `minikube kubectl --` prefix everywhere.

### 3. Start Minikube

```powershell
minikube start --driver=docker --cpus=4 --memory=8192
```

**Resource allocation:**
- **4 CPUs** — enough for all services without starving the host. Adjust up if the PC has cores to spare.
- **8 GB RAM** — covers all pods (Python services ~512Mi each, Java services ~512Mi each, databases, monitoring). The Windows host needs RAM for Ollama too (14B model uses ~10GB VRAM + some system RAM).
- **Docker driver** — uses Docker Desktop as the container runtime. This is the most reliable driver on Windows and avoids Hyper-V networking issues.

**Verify it's running:**
```powershell
minikube status
kubectl get nodes
```

### 4. Enable NGINX Ingress Controller

```powershell
minikube addons enable ingress
```

This installs the NGINX Ingress Controller as a pod in the `ingress-nginx` namespace. It's the unified entry point for all HTTP traffic — replacing the Docker Compose nginx container.

**Verify it's ready:**
```powershell
kubectl get pods -n ingress-nginx
```

You should see a `controller` pod in `Running` state.

### 5. Deploy All Services

```bash
cd <repo-path>
git pull  # make sure you have the latest K8s manifests
bash k8s/deploy.sh
```

The deploy script:
1. Creates 3 namespaces (`ai-services`, `java-tasks`, `monitoring`)
2. Applies secrets and ConfigMaps
3. Deploys infrastructure (Qdrant, Postgres, MongoDB, Redis, RabbitMQ) and waits for readiness
4. Deploys application services (Python AI services, Java microservices)
5. Deploys monitoring (Prometheus, Grafana)
6. Applies all Ingress resources

**Verify everything is running:**
```powershell
kubectl get pods --all-namespaces
kubectl get ingress --all-namespaces
```

All pods should be `Running` (or `Ready`). All ingresses should have an `ADDRESS` assigned (after minikube tunnel is running).

### 6. nvidia-gpu-exporter

This stays as a standalone Docker container (not in K8s) because it needs direct GPU access:

```powershell
docker run -d --restart=unless-stopped --gpus all -p 9835:9835 utkuozdemir/nvidia_gpu_exporter:1.4.1
```

Prometheus (in K8s) scrapes it via `host.minikube.internal:9835`.

### 7. Update Cloudflare Tunnel

The Cloudflare Tunnel currently points to `localhost:8000` (the old Docker Compose nginx). It needs to point to `localhost:80` (the Minikube Ingress via tunnel).

**Steps:**
1. Go to [Cloudflare Zero Trust Dashboard](https://one.dash.cloudflare.com/)
2. Navigate to **Networks → Tunnels**
3. Find your tunnel and click **Configure**
4. Under **Public Hostname**, find the `api.kylebradshaw.dev` entry
5. Change the **Service** URL from `http://localhost:8000` to `http://localhost:80`
6. Save

**No restart needed** — Cloudflared picks up the config change automatically.

**Important:** The old Docker Compose nginx on port 8000 can still run for local development, but production traffic now goes through the Minikube Ingress on port 80.

### 8. Persistent minikube tunnel

`minikube tunnel` creates a network route so that the Ingress LoadBalancer is accessible on `localhost:80`. It must run continuously.

**Option A: Manual (simplest)**

Open an Administrator PowerShell and run:
```powershell
minikube tunnel
```
Leave this terminal open. If the PC restarts, you need to re-run this.

**Option B: Windows Scheduled Task (auto-start on boot)**

1. Open **Task Scheduler** (search for it in Start)
2. Click **Create Task** (not "Create Basic Task")
3. **General tab:**
   - Name: `Minikube Tunnel`
   - Check "Run with highest privileges" (required for route creation)
   - Configure for: Windows 10/11
4. **Triggers tab:**
   - New → Begin the task: At startup
   - Delay task for: 30 seconds (give Docker Desktop time to start)
5. **Actions tab:**
   - New → Start a program
   - Program: `C:\Program Files\Kubernetes\Minikube\minikube.exe` (or wherever minikube is installed — run `where minikube` to find it)
   - Arguments: `tunnel`
6. **Conditions tab:**
   - Uncheck "Start only if the computer is on AC power"
7. **Settings tab:**
   - Check "If the task fails, restart every 1 minute"
   - Attempt to restart up to: 3 times
   - Check "Run task as soon as possible after a scheduled start is missed"
8. Click OK

**Verify the task works:**
```powershell
# After creating the task, test it:
schtasks /run /tn "Minikube Tunnel"
# Check if localhost:80 is responding:
curl http://localhost/chat/health
```

**Option C: NSSM (Non-Sucking Service Manager)**

If you want minikube tunnel as a proper Windows service:
```powershell
# Install NSSM
winget install NSSM.NSSM
# Create the service
nssm install MinikubeTunnel "C:\Program Files\Kubernetes\Minikube\minikube.exe" tunnel
nssm set MinikubeTunnel Start SERVICE_AUTO_START
nssm set MinikubeTunnel AppStdout C:\logs\minikube-tunnel.log
nssm set MinikubeTunnel AppStderr C:\logs\minikube-tunnel-error.log
# Start it
nssm start MinikubeTunnel
```

## Verification

After all steps are complete, verify the full stack:

```powershell
# All pods running
kubectl get pods --all-namespaces

# All ingresses have addresses
kubectl get ingress --all-namespaces

# Python services responding (via Ingress)
curl http://localhost/chat/health
curl http://localhost/ingestion/health

# Java GraphQL responding
curl -X POST http://localhost/graphql -H "Content-Type: application/json" -d '{"query":"{ __typename }"}'

# GraphiQL UI loads
# Open http://localhost/graphiql in browser

# Grafana loads
# Open http://localhost/grafana/ in browser

# Via Cloudflare Tunnel (after step 7)
curl https://api.kylebradshaw.dev/chat/health
```

## Troubleshooting

**Pods stuck in `ImagePullBackOff`:**
The GHCR images may be private. Either make them public in GitHub (Settings → Packages → Package settings → Change visibility), or create an image pull secret:
```powershell
kubectl create secret docker-registry ghcr-secret `
  --docker-server=ghcr.io `
  --docker-username=kabradshaw1 `
  --docker-password=<github-pat> `
  -n ai-services
# Repeat for java-tasks namespace
```

**Pods stuck in `Pending`:**
Check if Minikube has enough resources:
```powershell
kubectl describe nodes | Select-String -Pattern "Allocated|Capacity|cpu|memory"
```
If resources are exhausted, restart Minikube with more:
```powershell
minikube stop
minikube start --cpus=6 --memory=12288
```

**Ingress returns 404 or 502:**
Check the ingress controller logs:
```powershell
kubectl logs -n ingress-nginx -l app.kubernetes.io/component=controller --tail=50
```

**Python services `degraded` (Ollama/Qdrant unreachable):**
- Verify Ollama is running: `curl http://localhost:11434/api/tags`
- Verify the ExternalName service resolves: `kubectl exec -n ai-services deploy/chat -- nslookup ollama`
- Verify Qdrant pod is ready: `kubectl get pods -n ai-services -l app=qdrant`

**minikube tunnel not working:**
- Must run as Administrator
- Docker Desktop must be running
- Check: `minikube tunnel --cleanup` then `minikube tunnel`

## Updating Services

When new code is pushed to `main`, CI runs `kubectl apply` and `kubectl rollout restart` via SSH. To manually update:

```powershell
cd <repo-path>
git pull origin main
kubectl apply -f k8s/ai-services/ --recursive
kubectl apply -f java/k8s/ --recursive
kubectl apply -f k8s/monitoring/ --recursive
kubectl rollout restart deployment -n ai-services
kubectl rollout restart deployment -n java-tasks
```

## Stopping Everything

```powershell
# Stop all K8s services (keeps Minikube running)
kubectl delete namespace ai-services java-tasks monitoring

# Stop Minikube entirely
minikube stop

# Start back up later
minikube start
bash k8s/deploy.sh
```
