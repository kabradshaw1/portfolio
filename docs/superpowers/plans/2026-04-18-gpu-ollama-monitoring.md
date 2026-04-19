# GPU & Ollama Monitoring Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add alerting for GPU, Ollama, and ai-service failures so Kyle gets a Telegram notification when infrastructure degrades.

**Architecture:** Prometheus scrapes a new Ollama target and the existing GPU exporter. Grafana evaluates alert rules against Prometheus queries and sends notifications via a provisioned Telegram contact point. Host-level systemd services (Ollama override, nvidia_gpu_exporter) are configured manually via SSH.

**Tech Stack:** Prometheus, Grafana (provisioned alerting), Telegram Bot API, nvidia_gpu_exporter, systemd

---

### Task 1: Install kernel headers meta-package (manual — Debian server)

This prevents a repeat of the root cause: kernel updates without matching headers break DKMS NVIDIA module rebuilds.

**Files:** None (host-level change)

- [ ] **Step 1: SSH into Debian and install the meta-package**

```bash
ssh debian
sudo apt install linux-headers-amd64
```

Expected: Package installs (may already be installed from today's fix). This meta-package ensures future kernel upgrades automatically pull matching headers.

- [ ] **Step 2: Verify DKMS is healthy**

```bash
sudo dkms status
```

Expected: Output shows `nvidia-current` built for the current kernel `6.12.74+deb13+1-amd64`.

---

### Task 2: Create Ollama keep-alive systemd override (manual — Debian server)

Models must stay loaded in GPU memory permanently. Without this, Ollama unloads models after 5 minutes of inactivity.

**Files:** None (host-level change)

- [ ] **Step 1: Create the override file**

```bash
sudo systemctl edit ollama
```

In the editor that opens, paste exactly:

```ini
[Service]
Environment=OLLAMA_KEEP_ALIVE=-1
```

Save and exit.

- [ ] **Step 2: Reload and restart Ollama**

```bash
sudo systemctl daemon-reload
sudo systemctl restart ollama
```

- [ ] **Step 3: Verify models are loaded with Forever keep-alive**

```bash
# Warm up the models first
curl -s -X POST http://localhost:11434/api/generate -d '{"model":"qwen2.5:14b","prompt":"hi","stream":false}' > /dev/null
curl -s -X POST http://localhost:11434/api/embeddings -d '{"model":"nomic-embed-text","prompt":"hi"}' > /dev/null

# Check they're loaded on GPU with Forever keep-alive
ollama ps
```

Expected:
```
NAME                       ID              SIZE      PROCESSOR    CONTEXT    UNTIL
qwen2.5:14b                ...             17 GB     100% GPU     32768      Forever
nomic-embed-text:latest    ...             565 MB    100% GPU     2048       Forever
```

- [ ] **Step 4: Verify override persists**

```bash
cat /etc/systemd/system/ollama.service.d/override.conf
```

Expected:
```ini
[Service]
Environment=OLLAMA_KEEP_ALIVE=-1
```

---

### Task 3: Install nvidia_gpu_exporter (manual — Debian server)

Prometheus already scrapes `host.minikube.internal:9835` but nothing is listening. This installs the exporter.

**Files:** None (host-level change)

- [ ] **Step 1: Download the binary**

```bash
# Check latest release at https://github.com/utkuozdemir/nvidia_gpu_exporter/releases
# As of writing, v1.2.1 is latest
cd /tmp
curl -LO https://github.com/utkuozdemir/nvidia_gpu_exporter/releases/download/v1.4.1/nvidia_gpu_exporter_1.4.1_linux_amd64.tar.gz
tar xzf nvidia_gpu_exporter_1.4.1_linux_amd64.tar.gz
sudo mv nvidia_gpu_exporter /usr/local/bin/
sudo chmod +x /usr/local/bin/nvidia_gpu_exporter
```

- [ ] **Step 2: Create a dedicated user**

```bash
sudo useradd --system --no-create-home --shell /usr/sbin/nologin nvidia-exporter
```

- [ ] **Step 3: Create the systemd service**

```bash
sudo tee /etc/systemd/system/nvidia_gpu_exporter.service > /dev/null << 'EOF'
[Unit]
Description=NVIDIA GPU Exporter for Prometheus
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/nvidia_gpu_exporter
User=nvidia-exporter
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
```

- [ ] **Step 4: Enable and start the service**

```bash
sudo systemctl daemon-reload
sudo systemctl enable nvidia_gpu_exporter
sudo systemctl start nvidia_gpu_exporter
```

- [ ] **Step 5: Verify the exporter is serving metrics**

```bash
curl -s http://localhost:9835/metrics | head -20
```

Expected: Prometheus-format metrics including `nvidia_smi_temperature_gpu`, `nvidia_smi_utilization_gpu_ratio`, `nvidia_smi_memory_used_bytes`, etc.

- [ ] **Step 6: Verify Prometheus can scrape it from inside Minikube**

```bash
# From the host, check the minikube-internal route
curl -s http://$(minikube ip):9835/metrics 2>/dev/null | head -5 || echo "Direct IP won't work — checking via Prometheus targets"

# Check Prometheus targets page
kubectl exec -n monitoring deploy/prometheus -- wget -qO- 'http://localhost:9090/api/v1/targets' 2>/dev/null | python3 -m json.tool | grep -A5 '"nvidia-gpu"'
```

Expected: Target `nvidia-gpu` shows `health: "up"`.

---

### Task 4: Add Ollama scrape target to Prometheus ConfigMap

**Files:**
- Modify: `k8s/monitoring/configmaps/prometheus-config.yml:22-23`

- [ ] **Step 1: Add the ollama scrape job**

In `k8s/monitoring/configmaps/prometheus-config.yml`, add a new job after the `nvidia-gpu` job (after line 23):

```yaml
      - job_name: "ollama"
        metrics_path: /api/tags
        scrape_interval: 30s
        static_configs:
          - targets: ["host.minikube.internal:11434"]
```

The `metrics_path: /api/tags` is used because Ollama doesn't expose `/metrics`. Prometheus will still record `up{job="ollama"}` based on whether the HTTP request succeeds. The `scrape_interval: 30s` is slightly longer than default since this is just a health check, not real metrics.

- [ ] **Step 2: Validate YAML**

```bash
python3 -c "import yaml; yaml.safe_load(open('k8s/monitoring/configmaps/prometheus-config.yml'))" && echo "YAML valid"
```

Expected: `YAML valid`

- [ ] **Step 3: Commit**

```bash
git add k8s/monitoring/configmaps/prometheus-config.yml
git commit -m "feat(monitoring): add Ollama scrape target to Prometheus"
```

---

### Task 5: Create Telegram bot (manual — Kyle's phone)

**Files:** None

- [ ] **Step 1: Create the bot**

1. Open Telegram and search for `@BotFather`
2. Send `/newbot`
3. Name: `Portfolio Alerts` (or whatever you prefer)
4. Username: `kylebradshaw_alerts_bot` (must be unique, end in `bot`)
5. Save the **bot token** (looks like `123456789:ABCdefGHIjklMNOpqrSTUvwxYZ`)

- [ ] **Step 2: Get your chat ID**

1. Send any message to your new bot in Telegram
2. Open this URL in a browser (replace `<TOKEN>` with your bot token):
   ```
   https://api.telegram.org/bot<TOKEN>/getUpdates
   ```
3. Find the `"chat":{"id": 123456789}` value in the response — that's your chat ID

- [ ] **Step 3: Test the bot sends messages**

```bash
curl -s -X POST "https://api.telegram.org/bot<TOKEN>/sendMessage" \
  -d "chat_id=<CHAT_ID>" \
  -d "text=🔔 Test alert from Portfolio Monitoring"
```

Expected: You receive the message on your phone.

- [ ] **Step 4: Store credentials in a K8s secret**

```bash
ssh debian "kubectl create secret generic telegram-bot \
  --namespace=monitoring \
  --from-literal=bot-token='<TOKEN>' \
  --from-literal=chat-id='<CHAT_ID>' \
  --dry-run=client -o yaml | kubectl apply -f -"
```

---

### Task 6: Create Grafana alerting provisioning ConfigMap

Grafana supports provisioning alert rules, contact points, and notification policies via YAML files mounted into `/etc/grafana/provisioning/alerting/`. This is the declarative equivalent of configuring alerts through the UI.

**Files:**
- Create: `k8s/monitoring/configmaps/grafana-alerting.yml`

- [ ] **Step 1: Create the alerting provisioning ConfigMap**

Create `k8s/monitoring/configmaps/grafana-alerting.yml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: grafana-alerting
  namespace: monitoring
data:
  alerting.yml: |
    apiVersion: 1

    contactPoints:
      - orgId: 1
        name: telegram
        receivers:
          - uid: telegram-default
            type: telegram
            settings:
              bottoken: $__file{/etc/grafana/secrets/bot-token}
              chatid: $__file{/etc/grafana/secrets/chat-id}
            disableResolveMessage: false

    policies:
      - orgId: 1
        receiver: telegram
        group_by:
          - grafana_folder
          - alertname
        group_wait: 30s
        group_interval: 5m
        repeat_interval: 4h

    groups:
      - orgId: 1
        name: Infrastructure
        folder: Infrastructure Alerts
        interval: 1m
        rules:
          - uid: gpu-exporter-down
            title: GPU Exporter Down
            condition: C
            data:
              - refId: A
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: prometheus
                model:
                  expr: up{job="nvidia-gpu"}
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
                        type: lt
                        params:
                          - 1
                  refId: C
            for: 2m
            labels:
              severity: critical
            annotations:
              summary: "GPU exporter is unreachable — NVIDIA GPU may be down or driver unloaded"

          - uid: ollama-down
            title: Ollama Down
            condition: C
            data:
              - refId: A
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: prometheus
                model:
                  expr: up{job="ollama"}
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
                        type: lt
                        params:
                          - 1
                  refId: C
            for: 2m
            labels:
              severity: critical
            annotations:
              summary: "Ollama is not responding — AI services will be degraded"

          - uid: ai-service-not-ready
            title: AI Service Not Ready
            condition: C
            data:
              - refId: A
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: prometheus
                model:
                  expr: kube_pod_status_ready{pod=~"go-ai-service.*", condition="true"}
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
                        type: lt
                        params:
                          - 1
                  refId: C
            for: 3m
            labels:
              severity: critical
            annotations:
              summary: "go-ai-service pod is not ready — shopping assistant is down"

          - uid: gpu-temperature-high
            title: GPU Temperature High
            condition: C
            data:
              - refId: A
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: prometheus
                model:
                  expr: nvidia_smi_temperature_gpu
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
                          - 85
                  refId: C
            for: 5m
            labels:
              severity: warning
            annotations:
              summary: "GPU temperature is above 85°C"

          - uid: gpu-vram-high
            title: GPU VRAM Usage High
            condition: C
            data:
              - refId: A
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: prometheus
                model:
                  expr: nvidia_smi_memory_used_bytes / nvidia_smi_memory_total_bytes
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
                          - 0.9
                  refId: C
            for: 10m
            labels:
              severity: warning
            annotations:
              summary: "GPU VRAM usage is above 90%"
```

**Note on the datasourceUid:** Grafana provisioned datasources use the `name` field as UID by default. The existing datasource is named `Prometheus`. We need to verify this matches. If Grafana assigns a different UID, we'll update the alert rules to use the correct one. Check with:

```bash
ssh debian "kubectl exec -n monitoring deploy/grafana -- curl -s http://localhost:3000/api/datasources 2>/dev/null" | python3 -m json.tool
```

If the UID is not `prometheus`, update every `datasourceUid: prometheus` in the ConfigMap to match.

- [ ] **Step 2: Validate YAML**

```bash
python3 -c "import yaml; yaml.safe_load(open('k8s/monitoring/configmaps/grafana-alerting.yml'))" && echo "YAML valid"
```

Expected: `YAML valid`

- [ ] **Step 3: Commit**

```bash
git add k8s/monitoring/configmaps/grafana-alerting.yml
git commit -m "feat(monitoring): add Grafana alerting provisioning with Telegram contact point"
```

---

### Task 7: Mount alerting ConfigMap and secrets in Grafana deployment

**Files:**
- Modify: `k8s/monitoring/deployments/grafana.yml`
- Modify: `k8s/monitoring/kustomization.yaml`

- [ ] **Step 1: Add the alerting volume mount and secret mount to the Grafana deployment**

In `k8s/monitoring/deployments/grafana.yml`, add to the `volumeMounts` section (after the dashboards mount, line 42):

```yaml
            - name: alerting
              mountPath: /etc/grafana/provisioning/alerting/alerting.yml
              subPath: alerting.yml
            - name: telegram-secret
              mountPath: /etc/grafana/secrets
              readOnly: true
```

Add to the `volumes` section (after the dashboards volume, line 65):

```yaml
        - name: alerting
          configMap:
            name: grafana-alerting
        - name: telegram-secret
          secret:
            secretName: telegram-bot
```

Also add an env var to enable Grafana's unified alerting (it's on by default in recent versions, but let's be explicit):

```yaml
            - name: GF_UNIFIED_ALERTING_ENABLED
              value: "true"
```

- [ ] **Step 2: Add the alerting ConfigMap to kustomization.yaml**

In `k8s/monitoring/kustomization.yaml`, add after line 14 (`configmaps/grafana-datasource.yml`):

```yaml
  - configmaps/grafana-alerting.yml
```

- [ ] **Step 3: Validate YAML for both files**

```bash
python3 -c "import yaml; yaml.safe_load(open('k8s/monitoring/deployments/grafana.yml'))" && echo "grafana.yml valid"
python3 -c "import yaml; yaml.safe_load(open('k8s/monitoring/kustomization.yaml'))" && echo "kustomization.yaml valid"
```

Expected: Both valid.

- [ ] **Step 4: Commit**

```bash
git add k8s/monitoring/deployments/grafana.yml k8s/monitoring/kustomization.yaml
git commit -m "feat(monitoring): mount alerting provisioning and Telegram secret in Grafana"
```

---

### Task 8: Deploy and verify

**Files:** None (deployment commands)

- [ ] **Step 1: Deploy the monitoring stack**

```bash
ssh debian "cd ~/repos/gen_ai_engineer && git pull && kubectl apply -k k8s/monitoring"
```

Expected: ConfigMaps updated, Grafana deployment updated (triggers pod restart).

- [ ] **Step 2: Wait for Grafana to restart**

```bash
ssh debian "kubectl rollout status deployment/grafana -n monitoring --timeout=120s"
```

Expected: `deployment "grafana" successfully rolled out`

- [ ] **Step 3: Verify Prometheus targets**

```bash
ssh debian "kubectl exec -n monitoring deploy/prometheus -- wget -qO- 'http://localhost:9090/api/v1/targets' 2>/dev/null" | python3 -c "
import json, sys
data = json.load(sys.stdin)
for t in data['data']['activeTargets']:
    print(f\"{t['labels'].get('job', 'unknown'):20s} {t['health']:6s} {t['lastError'] or 'OK'}\")
"
```

Expected: Both `nvidia-gpu` and `ollama` targets show `up` health.

- [ ] **Step 4: Verify Grafana alert rules are loaded**

```bash
ssh debian "kubectl exec -n monitoring deploy/grafana -- curl -s http://localhost:3000/api/v1/provisioning/alert-rules 2>/dev/null" | python3 -c "
import json, sys
rules = json.load(sys.stdin)
for r in rules:
    print(f\"{r['title']:30s} uid={r['uid']}\")
"
```

Expected: All 5 alert rules listed (GPU Exporter Down, Ollama Down, AI Service Not Ready, GPU Temperature High, GPU VRAM Usage High).

- [ ] **Step 5: Verify Telegram contact point**

```bash
ssh debian "kubectl exec -n monitoring deploy/grafana -- curl -s http://localhost:3000/api/v1/provisioning/contact-points 2>/dev/null" | python3 -m json.tool
```

Expected: `telegram` contact point listed with type `telegram`.

- [ ] **Step 6: Send a test notification**

```bash
ssh debian "kubectl exec -n monitoring deploy/grafana -- curl -s -X POST http://localhost:3000/api/v1/provisioning/contact-points/telegram-default/test 2>/dev/null"
```

If this endpoint isn't available, test by temporarily stopping the GPU exporter to trigger the GPUExporterDown alert:

```bash
ssh debian "sudo systemctl stop nvidia_gpu_exporter"
# Wait ~3 minutes for the alert to fire
# Check your Telegram — you should receive a notification
# Then restart it:
ssh debian "sudo systemctl start nvidia_gpu_exporter"
```

- [ ] **Step 7: Commit any adjustments and push**

If any datasource UIDs, metric names, or config needed adjustment during verification:

```bash
git add -A
git commit -m "fix(monitoring): adjust alerting config after deployment verification"
git push
```
