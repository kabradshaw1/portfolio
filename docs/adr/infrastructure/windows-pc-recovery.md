# Windows PC Recovery After Power Outage

When the Windows PC (100.79.113.84) restarts after a power outage, most services auto-start but Minikube and its tunnel need manual intervention.

## Quick recovery

Open an **admin PowerShell** (right-click Terminal → Run as administrator) and run:

```powershell
restartservices
```

This runs `C:\Users\PC\Scripts\Restart-Services.ps1`, which:
1. Verifies Ollama is responding
2. Verifies Cloudflared tunnel service is running
3. Starts Minikube if stopped
4. Waits for all pods to become ready (up to 3 minutes)
5. Starts `minikube tunnel` in a minimized window
6. Runs a health check to confirm ingress is working

**Leave the minikube tunnel window open** — closing it kills ingress routing.

## What auto-starts on boot

- **Ollama** — startup app, GPU-accelerated
- **Cloudflared** — Windows service, reconnects the Cloudflare Tunnel automatically

## What does NOT auto-start

- **Minikube** — must be started manually (`minikube start`)
- **Minikube tunnel** — must be started manually in an admin terminal (`minikube tunnel`)

## Verification

After recovery, these should all return HTTP 200:

```
curl http://localhost/go-api/health
curl http://localhost/grafana/api/health
curl http://localhost/chat/health
```

Once working, the production site (kylebradshaw.dev) is also functional since Cloudflared routes through the same localhost:80 ingress.

## From the Mac

Re-establish the SSH tunnel for local dev if needed:

```
ssh -f -N -L 8000:localhost:8000 PC@100.79.113.84
```

## Files

- **Script:** `C:\Users\PC\Scripts\Restart-Services.ps1`
- **Profile alias:** `C:\Users\PC\Documents\WindowsPowerShell\Microsoft.PowerShell_profile.ps1`
- **Execution policy:** `RemoteSigned` (CurrentUser scope)
