# QA Environment Deployment Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the Kustomize directory cycle bug in Java/Go manifests, deploy QA workloads to the Debian server, and verify the full QA pipeline works.

**Architecture:** Restructure `java/k8s/` and `go/k8s/` into standard `base/` + `overlays/` layout so `kubectl apply -k` works with Kustomize v5.7.1+. Then deploy QA overlays and verify health endpoints via `qa-api.kylebradshaw.dev`.

**Tech Stack:** Kustomize, kubectl, Cloudflare Tunnel, GitHub Actions CI/CD

---

### Task 1: Create feature branch

**Files:** None

- [ ] **Step 1: Create and switch to feature branch**

```bash
git checkout main
git pull origin main
git checkout -b agent/feat-qa-deploy
```

- [ ] **Step 2: Commit spec file**

The spec at `docs/superpowers/specs/2026-04-15-qa-environment-deploy-design.md` was already created. Stage and commit it.

```bash
git add docs/superpowers/specs/2026-04-15-qa-environment-deploy-design.md
git commit -m "docs: add QA deployment and Kustomize fix spec"
```

---

### Task 2: Restructure Java K8s directories

**Files:**
- Move: `java/k8s/{kustomization.yaml,namespace.yml,ingress.yml,ingress-rabbitmq.yml}` → `java/k8s/base/`
- Move: `java/k8s/{configmaps,deployments,services,volumes,secrets}/` → `java/k8s/base/`
- Keep: `java/k8s/overlays/` stays in place
- Keep: `java/k8s/deploy.sh` stays in place
- Modify: `java/k8s/overlays/minikube/kustomization.yaml` — change `../../` to `../base`
- Modify: `java/k8s/overlays/aws/kustomization.yaml` — change `../../` to `../base`
- Modify: `java/k8s/overlays/qa/kustomization.yaml` — change `../../` to `../base`

- [ ] **Step 1: Create base directory and move files**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer
mkdir -p java/k8s/base
git mv java/k8s/kustomization.yaml java/k8s/base/
git mv java/k8s/namespace.yml java/k8s/base/
git mv java/k8s/ingress.yml java/k8s/base/
git mv java/k8s/ingress-rabbitmq.yml java/k8s/base/
git mv java/k8s/configmaps java/k8s/base/
git mv java/k8s/deployments java/k8s/base/
git mv java/k8s/services java/k8s/base/
git mv java/k8s/volumes java/k8s/base/
git mv java/k8s/secrets java/k8s/base/
```

- [ ] **Step 2: Update minikube overlay reference**

Edit `java/k8s/overlays/minikube/kustomization.yaml`:
```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - ../base
```

- [ ] **Step 3: Update AWS overlay reference**

Edit `java/k8s/overlays/aws/kustomization.yaml` — change `../../` to `../base` in the `resources` section.

- [ ] **Step 4: Update QA overlay reference**

Edit `java/k8s/overlays/qa/kustomization.yaml` — change `../../` to `../base` in the `resources` section (line 5).

- [ ] **Step 5: Verify Kustomize builds without cycle error**

```bash
kubectl kustomize java/k8s/overlays/minikube/
kubectl kustomize java/k8s/overlays/qa/
```

Expected: YAML output with no cycle error.

- [ ] **Step 6: Commit**

```bash
git add java/k8s/
git commit -m "refactor: restructure java/k8s to base/overlays to fix Kustomize cycle"
```

---

### Task 3: Restructure Go K8s directories

**Files:**
- Move: `go/k8s/{kustomization.yaml,namespace.yml,ingress.yml}` → `go/k8s/base/`
- Move: `go/k8s/{configmaps,deployments,services,hpa,pdb,jobs,secrets}/` → `go/k8s/base/`
- Keep: `go/k8s/overlays/` stays in place
- Modify: `go/k8s/overlays/minikube/kustomization.yaml` — change `../../` to `../base`
- Modify: `go/k8s/overlays/aws/kustomization.yaml` — change `../../` to `../base`
- Modify: `go/k8s/overlays/qa/kustomization.yaml` — change `../../` to `../base`

- [ ] **Step 1: Create base directory and move files**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer
mkdir -p go/k8s/base
git mv go/k8s/kustomization.yaml go/k8s/base/
git mv go/k8s/namespace.yml go/k8s/base/
git mv go/k8s/ingress.yml go/k8s/base/
git mv go/k8s/configmaps go/k8s/base/
git mv go/k8s/deployments go/k8s/base/
git mv go/k8s/services go/k8s/base/
git mv go/k8s/hpa go/k8s/base/
git mv go/k8s/pdb go/k8s/base/
git mv go/k8s/jobs go/k8s/base/
git mv go/k8s/secrets go/k8s/base/
```

- [ ] **Step 2: Update minikube overlay reference**

Edit `go/k8s/overlays/minikube/kustomization.yaml`:
```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - ../base
```

- [ ] **Step 3: Update AWS overlay reference**

Edit `go/k8s/overlays/aws/kustomization.yaml` — change `../../` to `../base` in the `resources` section.

- [ ] **Step 4: Update QA overlay reference**

Edit `go/k8s/overlays/qa/kustomization.yaml` — change `../../` to `../base` in the `resources` section (line 5).

- [ ] **Step 5: Verify Kustomize builds without cycle error**

```bash
kubectl kustomize go/k8s/overlays/minikube/
kubectl kustomize go/k8s/overlays/qa/
```

Expected: YAML output with no cycle error.

- [ ] **Step 6: Commit**

```bash
git add go/k8s/
git commit -m "refactor: restructure go/k8s to base/overlays to fix Kustomize cycle"
```

---

### Task 4: Update CI/CD workflow references

**Files:**
- Modify: `.github/workflows/ci.yml`
- Modify: `k8s/deploy.sh`

- [ ] **Step 1: Update kubeconform path (ci.yml line 342)**

Change:
```
find k8s/ java/k8s/ go/k8s/ \
```
To:
```
find k8s/ java/k8s/base/ go/k8s/base/ \
```

- [ ] **Step 2: Update server-side dry-run path (ci.yml line 367)**

Change:
```
for dir in k8s java/k8s go/k8s; do
```
To:
```
for dir in k8s java/k8s/base go/k8s/base; do
```

- [ ] **Step 3: Update prod deploy find paths (ci.yml lines 799-803)**

Change line 799:
```
for f in $(find k8s go/k8s java/k8s -name 'namespace.yml' 2>/dev/null); do
```
To:
```
for f in $(find k8s go/k8s/base java/k8s/base -name 'namespace.yml' 2>/dev/null); do
```

Change line 801:
```
for f in $(find java/k8s -name '*.yml' -not -name 'namespace.yml' -not -path '*/secrets/*'); do
```
To:
```
for f in $(find java/k8s/base -name '*.yml' -not -name 'namespace.yml' -not -path '*/secrets/*'); do
```

Change line 803:
```
for f in $(find go/k8s -name '*.yml' -not -name 'namespace.yml' -not -path '*/secrets/*' -not -path '*/jobs/*'); do
```
To:
```
for f in $(find go/k8s/base -name '*.yml' -not -name 'namespace.yml' -not -path '*/secrets/*' -not -path '*/jobs/*'); do
```

- [ ] **Step 4: Update migration job paths (ci.yml lines 826, 833)**

Change line 826:
```
$SSH "kubectl apply -f -" < go/k8s/jobs/auth-service-migrate.yml
```
To:
```
$SSH "kubectl apply -f -" < go/k8s/base/jobs/auth-service-migrate.yml
```

Change line 833:
```
$SSH "kubectl apply -f -" < go/k8s/jobs/ecommerce-service-migrate.yml
```
To:
```
$SSH "kubectl apply -f -" < go/k8s/base/jobs/ecommerce-service-migrate.yml
```

- [ ] **Step 5: Update k8s/deploy.sh secret paths**

Change lines 65-66:
```
if [ -f "$REPO_DIR/java/k8s/secrets/java-secrets.yml" ]; then
  kubectl apply -f "$REPO_DIR/java/k8s/secrets/java-secrets.yml"
```
To:
```
if [ -f "$REPO_DIR/java/k8s/base/secrets/java-secrets.yml" ]; then
  kubectl apply -f "$REPO_DIR/java/k8s/base/secrets/java-secrets.yml"
```

Change lines 71-72:
```
if [ -f "$REPO_DIR/go/k8s/secrets/go-secrets.yml" ]; then
  kubectl apply -f "$REPO_DIR/go/k8s/secrets/go-secrets.yml"
```
To:
```
if [ -f "$REPO_DIR/go/k8s/base/secrets/go-secrets.yml" ]; then
  kubectl apply -f "$REPO_DIR/go/k8s/base/secrets/go-secrets.yml"
```

- [ ] **Step 6: Commit**

```bash
git add .github/workflows/ci.yml k8s/deploy.sh
git commit -m "fix: update CI/CD paths for base/overlays directory structure"
```

---

### Task 5: Run preflight checks

- [ ] **Step 1: Run K8s manifest validation locally**

```bash
kubectl kustomize java/k8s/overlays/minikube/ > /dev/null && echo "Java minikube OK"
kubectl kustomize java/k8s/overlays/qa/ > /dev/null && echo "Java QA OK"
kubectl kustomize java/k8s/overlays/aws/ > /dev/null && echo "Java AWS OK"
kubectl kustomize go/k8s/overlays/minikube/ > /dev/null && echo "Go minikube OK"
kubectl kustomize go/k8s/overlays/qa/ > /dev/null && echo "Go QA OK"
kubectl kustomize go/k8s/overlays/aws/ > /dev/null && echo "Go AWS OK"
kubectl kustomize k8s/overlays/qa/ > /dev/null && echo "AI QA OK"
kubectl kustomize k8s/overlays/minikube/ > /dev/null && echo "AI minikube OK"
```

Expected: All 8 print "OK" with no errors.

- [ ] **Step 2: Grep for stale paths**

```bash
grep -rn 'java/k8s/jobs\|java/k8s/secrets\|java/k8s/configmaps\|java/k8s/deployments\|java/k8s/services' .github/ k8s/ --include='*.yml' --include='*.yaml' --include='*.sh'
grep -rn 'go/k8s/jobs\|go/k8s/secrets\|go/k8s/configmaps\|go/k8s/deployments\|go/k8s/services' .github/ k8s/ --include='*.yml' --include='*.yaml' --include='*.sh'
```

Expected: No matches (all paths should now use `base/` prefix).

---

### Task 6: Push feature branch and create PR

- [ ] **Step 1: Push**

```bash
git push -u origin agent/feat-qa-deploy
```

- [ ] **Step 2: Watch CI**

Wait for all GitHub Actions checks to pass. The K8s manifest validation job is the critical one — it will confirm the restructured paths work.

- [ ] **Step 3: If CI fails, fix and push**

Debug any failures. Likely causes: missed path reference or Kustomize build error.

- [ ] **Step 4: Create PR to qa**

```bash
gh pr create --base qa --title "fix: restructure K8s dirs and deploy QA environment" --body "..."
```

---

### Task 7: Merge to QA and verify deployment

- [ ] **Step 1: Merge PR**

Once CI passes and Kyle approves, merge the PR to `qa`.

- [ ] **Step 2: Watch Deploy QA job**

The push to `qa` triggers the CI pipeline which includes `Deploy QA`. Watch:
```bash
gh run list -R kabradshaw1/gen_ai_engineer --branch qa -L 1
gh run view <run-id> -R kabradshaw1/gen_ai_engineer
```

- [ ] **Step 3: Verify QA pods are running**

```bash
ssh debian 'kubectl get pods -n ai-services-qa && kubectl get pods -n java-tasks-qa && kubectl get pods -n go-ecommerce-qa'
```

Expected: All pods Running (1/1).

- [ ] **Step 4: Verify QA endpoints**

```bash
curl -s https://qa-api.kylebradshaw.dev/chat/health
curl -s https://qa-api.kylebradshaw.dev/ingestion/health
curl -s https://qa-api.kylebradshaw.dev/debug/health
curl -s https://qa-api.kylebradshaw.dev/go-auth/health
curl -s https://qa-api.kylebradshaw.dev/go-api/health
```

Expected: All return healthy JSON.

- [ ] **Step 5: Verify QA Smoke Tests pass in CI**

The CI pipeline runs QA Smoke Tests after Deploy QA. Confirm they pass in the GitHub Actions run.
