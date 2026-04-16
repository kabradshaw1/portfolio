# Kustomize overlay cycle: repo-root overlays over per-service base/

- **Date:** 2026-04-16
- **Status:** Accepted

## Context

Kustomize v5.7.1 rejects the pattern where an overlay directory lives *inside* the base directory it references. The original layout —

```
java/k8s/
  kustomization.yaml      # base
  configmaps/ deployments/ services/ ...
  overlays/
    minikube/ aws/ qa/    # each referencing ../../ = java/k8s (which contains this overlay)
```

— produces a "cycle detected" error on `kubectl kustomize`. The same structure existed under `go/k8s/`. The Python AI stack at `k8s/ai-services/` was unaffected because its overlays already lived at `k8s/overlays/` as a *sibling* of the base, not a child.

## Options considered

**A. Restructure each service's k8s/ into `k8s/base/ + k8s/overlays/`** (proposed in closed PR [#59](https://github.com/kabradshaw1/portfolio/pull/59) and its plan doc at `docs/superpowers/plans/2026-04-15-qa-environment-deploy.md` — see `refs/pull/59/head`).

Move `java/k8s/{kustomization.yaml, configmaps, deployments, services, volumes, secrets, ingress.yml, …}` into `java/k8s/base/`, leaving `java/k8s/overlays/` as its sibling. Same for `go/k8s/`. Overlay references change from `../../` to `../base`.

**B. Move the QA overlays to repo-root `k8s/overlays/qa-java/` and `k8s/overlays/qa-go/`** (what shipped).

Keep `java/k8s/` and `go/k8s/` flat. The *minikube* and *aws* overlays stay where they are (they happen to work without triggering the cycle check). Only the QA overlays — the ones actively being exercised by the new QA deploy pipeline — move out entirely, to `k8s/overlays/qa-java/` and `k8s/overlays/qa-go/`, referencing their bases via `../../../java/k8s` and `../../../go/k8s`.

## Decision

Option B.

## Rationale

- **Smaller blast radius.** B changed ~5 files; A renamed 60+ files and required updating every path reference in CI/CD scripts, Makefiles, and the deploy workflow.
- **Aligns with an existing repo convention.** The Python stack's QA overlay already lives at `k8s/overlays/qa/`. Putting `qa-java/` and `qa-go/` next to it unifies all three stacks' QA tooling at the same layer of the tree, which also simplifies the deploy script (three `kubectl kustomize k8s/overlays/<qa-*>/` invocations from one location).
- **Scoped to the actual problem.** The cycle was only triggered by the QA overlays being pulled into CI at every push; minikube/aws overlays are rarely built. Moving only what breaks avoids touching working paths.
- **Preserves the spirit of A.** Both options solve the problem by making the overlay a sibling of (not a descendant of) its base. A does it per-service with a new `base/` subdir; B does it at the repo root with the existing flat `k8s/` layout. The underlying mental model is the same.

## Consequences

**Positive:**
- Existing tooling and paths for `java/k8s/` and `go/k8s/` remain valid — no Dockerfile, Makefile, script, or doc needed updating beyond the deploy workflow.
- Three-way parity across Python/Java/Go QA overlays (all under `k8s/overlays/`).
- The fix shipped alongside the QA deploy debugging in a single short chain of commits (`ba9613d` → `15e2ada` → `6ad8482`..`5cd2ec8`) rather than as a separate 66-file restructure.

**Trade-offs:**
- `java/k8s/overlays/{minikube,aws}/` still use the original `../../` pattern. If those overlays ever get pulled into a Kustomize-v5.7+ CI path, they'll hit the same cycle and need to be moved (either into a future `base/` subdir per Option A, or out to `k8s/overlays/` like the QA ones). For now they're unaffected because they're not part of the QA/prod deploy flow.
- The QA overlay paths are slightly less discoverable — someone touching `java/k8s/` might not realize a QA counterpart lives at the repo root. Mitigated by `CLAUDE.md` documenting the layout.

## Superseded artifacts

PR [#59](https://github.com/kabradshaw1/portfolio/pull/59) (`agent/feat-qa-deploy-v2`) was closed as superseded by this decision. Its plan/design docs (`docs/superpowers/plans/2026-04-15-qa-environment-deploy.md`, `docs/superpowers/specs/2026-04-15-qa-environment-deploy-design.md`) describe Option A in detail and remain retrievable via `refs/pull/59/head` if the tradeoffs ever need revisiting.
