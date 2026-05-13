# Eval Result Persistence Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist eval datasets/results across Kubernetes pod restarts and make `/ai/eval` reload failures visible to users.

**Architecture:** Keep the eval service's existing SQLite database at `/app/data/eval.db`, but back `/app/data` with a Kubernetes PVC instead of `emptyDir`. Keep the frontend backend-driven: no browser cache, just explicit error states when saved runs or baseline history cannot be loaded.

**Tech Stack:** Kubernetes manifests/Kustomize, Next.js React client components, Playwright mocked E2E tests.

---

## File Map

- Create `k8s/ai-services/pvc/eval-data.yml`: PVC for eval SQLite data.
- Modify `k8s/ai-services/kustomization.yaml`: include the new PVC.
- Modify `k8s/ai-services/deployments/eval.yml`: replace `emptyDir` with the PVC.
- Modify `frontend/src/components/eval/ResultsTab.tsx`: add visible list/detail load errors.
- Modify `frontend/src/components/eval/EvaluateTab.tsx`: add visible baseline-history warning.
- Modify `frontend/e2e/mocked/eval-dashboard.spec.ts`: add mocked E2E coverage for the new error states.

## Task 0: Prepare Feature Worktree

**Files:**
- No repo files changed in this task.

- [ ] **Step 1: Confirm branch and status**

Run:

```bash
git branch --show-current
git status --short
```

Expected: current branch is `main` or `qa`, with no unrelated changes that block creating a worktree.

- [ ] **Step 2: Create and enter feature worktree**

Run:

```bash
git worktree add .codex/worktrees/eval-result-persistence -b feat/eval-result-persistence
cd .codex/worktrees/eval-result-persistence
pwd
git branch --show-current
git rev-parse --show-toplevel
```

Expected:

```text
/Users/kylebradshaw/repos/gen_ai_engineer/.codex/worktrees/eval-result-persistence
feat/eval-result-persistence
/Users/kylebradshaw/repos/gen_ai_engineer/.codex/worktrees/eval-result-persistence
```

All later commands in this plan must run from `/Users/kylebradshaw/repos/gen_ai_engineer/.codex/worktrees/eval-result-persistence`.

## Task 1: Persist Eval SQLite Data In Kubernetes

**Files:**
- Create: `k8s/ai-services/pvc/eval-data.yml`
- Modify: `k8s/ai-services/kustomization.yaml`
- Modify: `k8s/ai-services/deployments/eval.yml`

- [ ] **Step 1: Add the eval PVC manifest**

Create `k8s/ai-services/pvc/eval-data.yml`:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: eval-data
  namespace: ai-services
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
```

- [ ] **Step 2: Add the PVC to the AI services kustomization**

In `k8s/ai-services/kustomization.yaml`, add `pvc/eval-data.yml` next to the existing ingestion PVC:

```yaml
resources:
  - namespace.yml
  - configmaps/chat-config.yml
  - configmaps/debug-config.yml
  - configmaps/eval-config.yml
  - configmaps/ingestion-config.yml
  - pvc/ingestion-data.yml
  - pvc/eval-data.yml
  - deployments/chat.yml
  - deployments/debug.yml
  - deployments/eval.yml
  - deployments/ingestion.yml
  - deployments/qdrant.yml
  - services/chat.yml
  - services/debug.yml
  - services/eval.yml
  - services/ingestion.yml
  - services/qdrant.yml
  - ingress.yml
```

- [ ] **Step 3: Mount the PVC in the eval deployment**

In `k8s/ai-services/deployments/eval.yml`, replace the current `emptyDir` volume:

```yaml
      volumes:
        - name: eval-data
          emptyDir: {}
```

with:

```yaml
      volumes:
        - name: eval-data
          persistentVolumeClaim:
            claimName: eval-data
```

- [ ] **Step 4: Verify rendered Kubernetes resources**

Run:

```bash
kubectl kustomize k8s/ai-services > /tmp/eval-ai-services-rendered.yml
rg -n "name: eval-data|persistentVolumeClaim|claimName: eval-data|mountPath: /app/data" /tmp/eval-ai-services-rendered.yml
```

Expected: output includes a PVC named `eval-data`, the eval deployment volume has `persistentVolumeClaim`, the claim name is `eval-data`, and the container still mounts `/app/data`.

- [ ] **Step 5: Commit Kubernetes persistence change**

Run:

```bash
git add k8s/ai-services/pvc/eval-data.yml k8s/ai-services/kustomization.yaml k8s/ai-services/deployments/eval.yml
git commit -m "fix: persist eval sqlite data"
```

## Task 2: Show Results Tab Load Errors

**Files:**
- Modify: `frontend/src/components/eval/ResultsTab.tsx`
- Test: `frontend/e2e/mocked/eval-dashboard.spec.ts`

- [ ] **Step 1: Add failing E2E coverage for Results list and detail errors**

In `frontend/e2e/mocked/eval-dashboard.spec.ts`, first add a route for `eval-base-001` inside `mockEvalApi`, directly before the existing `eval-new-003` route:

```ts
  await page.route("**/eval/evaluations/eval-base-001", async (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(historyRuns[0]),
    }),
  );
```

Then add these tests inside the existing `/ai/eval dashboard` describe block:

```ts
  test("results tab shows a saved-evaluations load error", async ({ page }) => {
    await page.unroute("**/eval/evaluations");
    await page.route("**/eval/evaluations", async (route) => {
      if (route.request().method() === "GET") {
        return route.fulfill({
          status: 500,
          contentType: "application/json",
          body: JSON.stringify({ detail: "boom" }),
        });
      }
      return route.fallback();
    });

    await page.goto("/ai/eval");
    await page.getByRole("button", { name: "Results" }).click();

    await expect(
      page.getByText("Could not load saved evaluations. Refresh or sign in again."),
    ).toBeVisible();
    await expect(
      page.getByText("No evaluation results yet. Go to the Evaluate tab to run one."),
    ).not.toBeVisible();
  });

  test("results tab shows a selected-evaluation detail load error", async ({
    page,
  }) => {
    await page.unroute("**/eval/evaluations");
    await page.route("**/eval/evaluations", async (route) => {
      if (route.request().method() === "GET") {
        return route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({ evaluations: [historyRuns[1]] }),
        });
      }
      return route.fallback();
    });
    await page.unroute("**/eval/evaluations/eval-tuned-002");
    await page.route("**/eval/evaluations/eval-tuned-002", async (route) =>
      route.fulfill({
        status: 500,
        contentType: "application/json",
        body: JSON.stringify({ detail: "boom" }),
      }),
    );

    await page.goto("/ai/eval");
    await page.getByRole("button", { name: "Results" }).click();

    await expect(page.getByLabel("Evaluation")).toBeVisible();
    await expect(
      page.getByText(
        "Could not load the selected evaluation. Try selecting it again or refresh.",
      ),
    ).toBeVisible();
    await expect(page.getByText("Aggregate Scores")).not.toBeVisible();
  });
```

- [ ] **Step 2: Run the new Results tests and verify they fail**

Run:

```bash
cd frontend
npx playwright test e2e/mocked/eval-dashboard.spec.ts -g "results tab shows" --project=chromium
```

Expected: both new tests fail because `ResultsTab` still swallows load errors.

- [ ] **Step 3: Add ResultsTab error state**

In `frontend/src/components/eval/ResultsTab.tsx`, add state after `expandedQuery`:

```ts
  const [listError, setListError] = useState<string>("");
  const [detailError, setDetailError] = useState<string>("");
```

Update the list-loading effect to clear and set errors:

```ts
  useEffect(() => {
    setListError("");
    listEvaluations()
      .then((list) => {
        setEvaluations(list);
        if (selectedEvaluation) {
          const inList = list.some((e) => e.id === selectedEvaluation.id);
          if (!inList) {
            setEvaluations([selectedEvaluation, ...list]);
          }
          setSelectedId(selectedEvaluation.id);
          setDetail(selectedEvaluation);
          setDetailError("");
        } else if (list.length > 0) {
          setSelectedId(list[0].id);
        }
      })
      .catch(() => {
        setEvaluations([]);
        setSelectedId("");
        setDetail(null);
        setDetailError("");
        setListError("Could not load saved evaluations. Refresh or sign in again.");
      });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
```

Update the selected-evaluation prop effect:

```ts
  useEffect(() => {
    if (selectedEvaluation) {
      setEvaluations((prev) => {
        const inList = prev.some((e) => e.id === selectedEvaluation.id);
        return inList ? prev : [selectedEvaluation, ...prev];
      });
      setSelectedId(selectedEvaluation.id);
      setDetail(selectedEvaluation);
      setListError("");
      setDetailError("");
      setExpandedQuery(null);
    }
  }, [selectedEvaluation]);
```

Update the detail-loading effect:

```ts
  useEffect(() => {
    if (!selectedId) return;
    setDetailError("");
    if (selectedEvaluation && selectedEvaluation.id === selectedId) {
      setDetail(selectedEvaluation);
      return;
    }
    getEvaluation(selectedId)
      .then((d) => {
        setDetail(d);
        setExpandedQuery(null);
      })
      .catch(() => {
        setDetail(null);
        setExpandedQuery(null);
        setDetailError(
          "Could not load the selected evaluation. Try selecting it again or refresh.",
        );
      });
  }, [selectedId]); // eslint-disable-line react-hooks/exhaustive-deps
```

Render both errors between the selector and empty state:

```tsx
      {listError && <p className="text-sm text-red-600">{listError}</p>}
      {detailError && <p className="text-sm text-red-600">{detailError}</p>}
```

Change the empty-state condition to:

```tsx
      {!listError && !detailError && !detail && evaluations.length === 0 && (
```

- [ ] **Step 4: Run the Results tests and verify they pass**

Run:

```bash
cd frontend
npx playwright test e2e/mocked/eval-dashboard.spec.ts -g "results tab shows" --project=chromium
```

Expected: both tests pass.

- [ ] **Step 5: Commit Results tab error handling**

Run:

```bash
git add frontend/src/components/eval/ResultsTab.tsx frontend/e2e/mocked/eval-dashboard.spec.ts
git commit -m "fix: show eval results load errors"
```

## Task 3: Show Evaluate Tab Baseline History Warning

**Files:**
- Modify: `frontend/src/components/eval/EvaluateTab.tsx`
- Test: `frontend/e2e/mocked/eval-dashboard.spec.ts`

- [ ] **Step 1: Add failing E2E coverage for baseline history errors**

Add this test inside the existing `/ai/eval dashboard` describe block:

```ts
  test("evaluate tab shows a non-blocking baseline history warning", async ({
    page,
  }) => {
    await page.unroute("**/eval/evaluations/history?**");
    await page.route("**/eval/evaluations/history?**", async (route) =>
      route.fulfill({
        status: 500,
        contentType: "application/json",
        body: JSON.stringify({ detail: "boom" }),
      }),
    );

    await page.goto("/ai/eval");
    await page.getByRole("button", { name: "Evaluate" }).click();

    await expect(
      page.getByText("Could not load baseline runs for this dataset and collection."),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Run Evaluation" }),
    ).toBeEnabled();
  });
```

- [ ] **Step 2: Run the new Evaluate test and verify it fails**

Run:

```bash
cd frontend
npx playwright test e2e/mocked/eval-dashboard.spec.ts -g "baseline history warning" --project=chromium
```

Expected: the test fails because the baseline history error is still silent.

- [ ] **Step 3: Add baseline warning state and render it**

In `frontend/src/components/eval/EvaluateTab.tsx`, add state after `baselineLoading`:

```ts
  const [baselineWarning, setBaselineWarning] = useState<string>("");
```

In the baseline-history effect, clear the warning on successful load and set it on failure:

```ts
      setBaselineLoading(true);
      setBaselineWarning("");
      try {
        const history = await getHistory(selectedDatasetId, trimmedCollection);
        if (cancelled) return;
        setBaselineRuns(history.runs);
        setBaselineWarning("");
        setBaselineEvalId((current) =>
          history.runs.some((run) => run.id === current) ? current : "",
        );
      } catch {
        if (cancelled) return;
        setBaselineRuns([]);
        setBaselineEvalId("");
        setBaselineWarning(
          "Could not load baseline runs for this dataset and collection.",
        );
      } finally {
        if (!cancelled) setBaselineLoading(false);
      }
```

Render the warning directly below the baseline `<select>`:

```tsx
          {baselineWarning && (
            <p className="mt-1 text-sm text-yellow-700">{baselineWarning}</p>
          )}
```

- [ ] **Step 4: Run the Evaluate test and verify it passes**

Run:

```bash
cd frontend
npx playwright test e2e/mocked/eval-dashboard.spec.ts -g "baseline history warning" --project=chromium
```

Expected: the test passes.

- [ ] **Step 5: Commit Evaluate tab baseline warning**

Run:

```bash
git add frontend/src/components/eval/EvaluateTab.tsx frontend/e2e/mocked/eval-dashboard.spec.ts
git commit -m "fix: show eval baseline history warning"
```

## Task 4: Full Verification And PR

**Files:**
- No new code files unless verification uncovers required fixes.

- [ ] **Step 1: Run focused eval dashboard E2E tests**

Run:

```bash
cd frontend
npx playwright test e2e/mocked/eval-dashboard.spec.ts --project=chromium
```

Expected: all tests in `eval-dashboard.spec.ts` pass.

- [ ] **Step 2: Run frontend preflight**

Run from repo root:

```bash
make preflight-frontend
```

Expected: frontend typecheck and lint pass.

- [ ] **Step 3: Run frontend E2E preflight**

Run from repo root:

```bash
make preflight-e2e
```

Expected: mocked E2E suite passes.

- [ ] **Step 4: Run Kubernetes render checks**

Run from repo root:

```bash
kubectl kustomize k8s/ai-services > /tmp/eval-ai-services-rendered.yml
rg -n "kind: PersistentVolumeClaim|name: eval-data|persistentVolumeClaim|claimName: eval-data|mountPath: /app/data" /tmp/eval-ai-services-rendered.yml
```

Expected: rendered output includes the eval PVC and eval deployment PVC mount.

- [ ] **Step 5: Run policy check if local dependencies exist**

Run from repo root:

```bash
scripts/k8s-policy-check.sh
```

Expected: `k8s-policy-check: all rules passed`. If `yq` is missing, report that CI will run the same policy check in the `k8s-manifest-validation` job.

- [ ] **Step 6: Inspect final diff**

Run:

```bash
git status --short
git diff --stat main..HEAD
git diff main..HEAD -- k8s/ai-services frontend/src/components/eval frontend/e2e/mocked/eval-dashboard.spec.ts
```

Expected: changes are limited to the approved Kubernetes PVC and frontend error-handling scope.

- [ ] **Step 7: Push branch and create PR to `qa`**

Run:

```bash
git push -u origin feat/eval-result-persistence
gh pr create --base qa --head feat/eval-result-persistence --title "Persist eval results and surface reload errors" --body "## Summary
- back eval SQLite data with a Kubernetes PVC
- show saved-run and selected-run load errors in /ai/eval Results
- show a non-blocking baseline history warning in Evaluate

## Verification
- npx playwright test e2e/mocked/eval-dashboard.spec.ts --project=chromium
- make preflight-frontend
- make preflight-e2e
- kubectl kustomize k8s/ai-services
- scripts/k8s-policy-check.sh"
```

Expected: PR is created against `qa`. Do not watch CI unless Kyle explicitly asks.
