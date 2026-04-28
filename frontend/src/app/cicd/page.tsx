import { MermaidDiagram } from "@/components/MermaidDiagram";
import { QADiffSection } from "@/components/QADiffSection";

const pipelineFlowDiagram = `flowchart LR
  subgraph PR["Pull Request to qa"]
    direction LR
    A[PR Created] --> B[Quality Checks]
    B --> B2[E2E Staging Checks]
    B2 --> C{All Pass?}
    C -->|Yes| D[Ready for Review]
    C -->|No| E[Fix & Push]
    E --> B
  end

  subgraph QA["Push to qa"]
    direction LR
    F[PR Merged] --> G[Quality Checks]
    G --> G2[E2E Staging Checks]
    G2 --> H[Build Images]
    H --> I[Deploy to QA]
    I --> J[Smoke Tests]
  end

  subgraph Prod["Push to main"]
    direction LR
    K[Ship It] --> L[Quality Checks]
    L --> M[Build Images]
    M --> N[Deploy to Prod]
    N --> O[Smoke Tests]
  end
`;

const qaArchitectureDiagram = `flowchart TB
  subgraph Minikube["Minikube Cluster"]
    subgraph ProdNS["Production Namespaces"]
      direction LR
      P1[ai-services]
      P2[java-tasks]
      P3[go-ecommerce]
      P4[monitoring]
    end
    subgraph QANS["QA Namespaces"]
      direction LR
      Q1[ai-services-qa]
      Q2[java-tasks-qa]
      Q3[go-ecommerce-qa]
    end
  end

  CF1[api.kylebradshaw.dev] --> P1
  CF2[qa-api.kylebradshaw.dev] --> Q1

  QANS -.->|shared infra| ProdNS
`;

const optimizationDiagram = `flowchart LR
  A[PR / Push] --> B[Quality Checks]
  B --> C[Compose Smoke]
  C --> D[Build Images]
  D --> E[Deploy]
  E --> F[Smoke Tests]

  style B fill:#1e3a5f,stroke:#3b82f6,color:#fff
  style C fill:#1e3a5f,stroke:#3b82f6,color:#fff
  style D fill:#1e3a5f,stroke:#3b82f6,color:#fff
  style E fill:#1e3a5f,stroke:#3b82f6,color:#fff

  B -.- G["① Venv caching"]
  C -.- H["③ Pull, don't build"]
  D -.- I["② Conditional builds"]
  E -.- J["④ Job immutability fix"]
`;

export default function CICDPage() {
  return (
    <div className="min-h-screen bg-background text-foreground">
      <div className="mx-auto max-w-3xl px-6 py-12">
        <h1 className="mt-8 text-3xl font-bold">CI/CD Pipeline</h1>

        {/* Overview */}
        <section className="mt-8">
          <p className="text-muted-foreground leading-relaxed">
            A unified GitHub Actions pipeline built for a solo developer. One
            workflow file handles all quality checks, image builds, and
            deployments for three service stacks (Python, Java, Go) and a Next.js
            frontend. Designed to automate everything from code push to production
            deploy, with a QA environment for visual inspection before shipping.
          </p>
        </section>

        {/* Pipeline Flow */}
        <section className="mt-12">
          <h2 className="text-xl font-semibold">Pipeline Flow</h2>
          <p className="mt-2 text-sm text-muted-foreground">
            Three triggers, one workflow. Every code change follows the same path
            through quality gates before reaching production.
          </p>
          <div className="mt-4">
            <MermaidDiagram chart={pipelineFlowDiagram} />
          </div>
        </section>

        {/* Why Unified */}
        <section className="mt-12">
          <h2 className="text-xl font-semibold">Why a Unified Workflow</h2>
          <p className="mt-2 text-muted-foreground leading-relaxed">
            I started with separate workflow files for each language stack —
            Python, Java, and Go each had their own CI pipeline. That was
            helpful early on for refining the specific checks I wanted per
            stack. But as a solo developer, I found that I very rarely stopped
            between stages. The separate workflows added maintenance overhead
            without adding real decision points.
          </p>
          <p className="mt-4 text-muted-foreground leading-relaxed">
            Consolidating into a single workflow made the pipeline easier to
            reason about — both for me and for the Claude Code agents that drive
            most of the development. One file to maintain, one set of status
            checks to watch, and a single place to debug when something fails.
            All quality gates run unconditionally on every trigger, which is
            slower but catches cross-stack issues that path filtering would
            miss.
          </p>
          <p className="mt-4 text-muted-foreground leading-relaxed">
            Since I&apos;m the only one working on this project, there&apos;s no
            need to rigorously defend the QA branch. I push minor tweaks
            directly to <code>qa</code> without feature branches — there&apos;s
            no one else to disrupt. The E2E staging checks still run on those
            direct pushes, so regressions get caught before deploy.
          </p>
        </section>

        {/* Trigger Matrix */}
        <section className="mt-12">
          <h2 className="text-xl font-semibold">Trigger Matrix</h2>
          <div className="mt-4 overflow-x-auto">
            <table className="w-full text-sm border-collapse">
              <thead>
                <tr className="border-b border-border">
                  <th className="py-2 pr-4 text-left font-medium">Job</th>
                  <th className="py-2 px-4 text-center font-medium">
                    PR to qa
                  </th>
                  <th className="py-2 px-4 text-center font-medium">
                    Push to qa
                  </th>
                  <th className="py-2 px-4 text-center font-medium">
                    Push to main
                  </th>
                </tr>
              </thead>
              <tbody className="text-muted-foreground">
                <tr className="border-b border-border/50">
                  <td className="py-2 pr-4">Quality checks</td>
                  <td className="py-2 px-4 text-center">✓</td>
                  <td className="py-2 px-4 text-center">✓</td>
                  <td className="py-2 px-4 text-center">✓</td>
                </tr>
                <tr className="border-b border-border/50">
                  <td className="py-2 pr-4">E2E staging checks</td>
                  <td className="py-2 px-4 text-center">✓</td>
                  <td className="py-2 px-4 text-center">✓</td>
                  <td className="py-2 px-4 text-center">—</td>
                </tr>
                <tr className="border-b border-border/50">
                  <td className="py-2 pr-4">Build images</td>
                  <td className="py-2 px-4 text-center">—</td>
                  <td className="py-2 px-4 text-center">✓</td>
                  <td className="py-2 px-4 text-center">✓</td>
                </tr>
                <tr className="border-b border-border/50">
                  <td className="py-2 pr-4">Deploy</td>
                  <td className="py-2 px-4 text-center">—</td>
                  <td className="py-2 px-4 text-center">QA</td>
                  <td className="py-2 px-4 text-center">Prod</td>
                </tr>
                <tr>
                  <td className="py-2 pr-4">Smoke tests</td>
                  <td className="py-2 px-4 text-center">—</td>
                  <td className="py-2 px-4 text-center">✓</td>
                  <td className="py-2 px-4 text-center">✓</td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>

        {/* Quality Gates */}
        <section className="mt-12">
          <h2 className="text-xl font-semibold">Quality Gates</h2>
          <p className="mt-2 text-sm text-muted-foreground">
            22 parallel jobs run on every trigger. All must pass before images are
            built.
          </p>
          <div className="mt-4 grid gap-3">
            <div className="rounded-lg border border-border p-4">
              <h3 className="text-sm font-medium">Python</h3>
              <p className="mt-1 text-sm text-muted-foreground">
                Ruff lint + format, pytest with coverage (ingestion, chat, debug),
                Bandit SAST, pip-audit
              </p>
            </div>
            <div className="rounded-lg border border-border p-4">
              <h3 className="text-sm font-medium">Java</h3>
              <p className="mt-1 text-sm text-muted-foreground">
                Checkstyle, unit tests (4 services), integration tests with
                Testcontainers
              </p>
            </div>
            <div className="rounded-lg border border-border p-4">
              <h3 className="text-sm font-medium">Go</h3>
              <p className="mt-1 text-sm text-muted-foreground">
                golangci-lint, go test -race (3 services), migration pipeline test
              </p>
            </div>
            <div className="rounded-lg border border-border p-4">
              <h3 className="text-sm font-medium">Frontend</h3>
              <p className="mt-1 text-sm text-muted-foreground">
                ESLint, TypeScript type check, Next.js build, npm audit
              </p>
            </div>
            <div className="rounded-lg border border-border p-4">
              <h3 className="text-sm font-medium">Security</h3>
              <p className="mt-1 text-sm text-muted-foreground">
                Gitleaks (secrets), Hadolint (Dockerfiles), CORS guardrail (no
                wildcard origins)
              </p>
            </div>
            <div className="rounded-lg border border-border p-4">
              <h3 className="text-sm font-medium">Infrastructure</h3>
              <p className="mt-1 text-sm text-muted-foreground">
                K8s manifest validation (kubeconform + kind dry-run), Grafana
                dashboard sync, Compose smoke test
              </p>
            </div>
          </div>
        </section>

        {/* QA Environment */}
        <section className="mt-12">
          <h2 className="text-xl font-semibold">QA Environment</h2>
          <p className="mt-2 text-muted-foreground leading-relaxed">
            QA runs in the same Minikube cluster as production using separate
            Kubernetes namespaces. Kustomize overlays patch the base manifests to
            set QA-specific CORS origins, database names, and ingress hosts —
            without duplicating the manifests themselves.
          </p>
          <div className="mt-4 flex items-center gap-4">
            <a
              href="https://qa.kylebradshaw.dev"
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center rounded-md bg-indigo-600 px-4 py-2 text-sm font-medium text-white hover:bg-indigo-500 transition-colors"
            >
              Visit the QA environment →
            </a>
            <span className="text-sm text-muted-foreground">
              See the latest pre-prod build before it ships.
            </span>
          </div>
          <div className="mt-4">
            <MermaidDiagram chart={qaArchitectureDiagram} />
          </div>
          <div className="mt-4 overflow-x-auto">
            <table className="w-full text-sm border-collapse">
              <thead>
                <tr className="border-b border-border">
                  <th className="py-2 pr-4 text-left font-medium">
                    Production
                  </th>
                  <th className="py-2 px-4 text-left font-medium">QA</th>
                </tr>
              </thead>
              <tbody className="text-muted-foreground">
                <tr className="border-b border-border/50">
                  <td className="py-2 pr-4">ai-services</td>
                  <td className="py-2 px-4">ai-services-qa</td>
                </tr>
                <tr className="border-b border-border/50">
                  <td className="py-2 pr-4">java-tasks</td>
                  <td className="py-2 px-4">java-tasks-qa</td>
                </tr>
                <tr>
                  <td className="py-2 pr-4">go-ecommerce</td>
                  <td className="py-2 px-4">go-ecommerce-qa</td>
                </tr>
              </tbody>
            </table>
          </div>
          <QADiffSection />
        </section>

        {/* Image Tagging */}
        <section className="mt-12">
          <h2 className="text-xl font-semibold">Image Tagging</h2>
          <p className="mt-2 text-muted-foreground leading-relaxed">
            All 10 service images are built in a single matrix job and pushed to
            GitHub Container Registry. QA images use a commit-pinned tag for
            traceability; production uses <code>:latest</code>.
          </p>
          <pre className="mt-4 overflow-x-auto rounded-lg border border-border bg-muted/50 p-4 text-sm">
{`# QA (push to qa branch)
ghcr.io/kabradshaw1/portfolio/ingestion:qa-abc1234

# Production (push to main branch)
ghcr.io/kabradshaw1/portfolio/ingestion:latest`}
          </pre>
        </section>

        {/* Deploy Mechanism */}
        <section className="mt-12">
          <h2 className="text-xl font-semibold">Deploy Mechanism</h2>
          <p className="mt-2 text-muted-foreground leading-relaxed">
            GitHub Actions joins a Tailscale VPN to reach the home server, then
            deploys via SSH. Kustomize overlays are built on the runner and piped
            to the server via <code>kubectl apply</code>.
          </p>
          <pre className="mt-4 overflow-x-auto rounded-lg border border-border bg-muted/50 p-4 text-sm">
{`# CI runner joins Tailscale VPN
- uses: tailscale/github-action@v3

# Build overlay locally, apply remotely
kubectl kustomize k8s/overlays/qa/ | \\
  ssh PC@100.79.113.84 "kubectl apply -f -"

# Restart deployments to pull new images
ssh PC@100.79.113.84 \\
  "kubectl rollout restart deployment -n ai-services-qa"`}
          </pre>
        </section>

        {/* No Branch Protection */}
        <section className="mt-12">
          <h2 className="text-xl font-semibold">Why No Branch Protection</h2>
          <p className="mt-2 text-muted-foreground leading-relaxed">
            This is a solo developer project. By the time code reaches{" "}
            <code>main</code>, it has passed all quality checks on the PR, been
            deployed to QA, and been visually inspected. Branch protection
            requiring PR approval would mean one person approving their own PR —
            ceremony with no value.
          </p>
          <p className="mt-4 text-muted-foreground leading-relaxed">
            The real protection is the CI pipeline itself: 22 quality gates that
            run on every push. If any fail, the deploy doesn&apos;t happen.
          </p>
        </section>

        {/* Agent Automation */}
        <section className="mt-12">
          <h2 className="text-xl font-semibold">Agent Automation</h2>
          <p className="mt-2 text-muted-foreground leading-relaxed">
            Since I&apos;m the only developer on this project, there&apos;s no
            risk to letting Claude Code agents drive the workflow from spec to
            production. No one else&apos;s deployment gets disrupted, and I
            review every spec thoroughly before any code gets written.
          </p>
          <p className="mt-4 text-muted-foreground leading-relaxed">
            The agents use a{" "}
            <a
              href="https://github.com/nichochar/claude-plugins-official/tree/main/superpowers"
              className="underline underline-offset-2"
            >
              superpowers
            </a>{" "}
            plugin that adds built-in quality gates throughout the workflow —
            automated spec self-review, code review agents, and
            verification-before-completion checks that require evidence before
            claiming work is done.
          </p>
          <div className="mt-4 rounded-lg border border-border p-4">
            <ol className="space-y-2 text-sm text-muted-foreground">
              <li>
                <strong className="text-foreground">1. Spec:</strong> Kyle and
                Claude brainstorm the design together. Claude writes a spec,
                then self-reviews it for placeholders, contradictions, and
                ambiguity before presenting it. Kyle reviews the spec
                thoroughly — this is the main human checkpoint.
              </li>
              <li>
                <strong className="text-foreground">2. Plan:</strong> Once the
                spec is approved, Claude writes a detailed implementation plan
                to keep track of what it needs to do during execution.
              </li>
              <li>
                <strong className="text-foreground">
                  3. Implement &rarr; PR:
                </strong>{" "}
                Agent creates a feature branch, implements the plan, and pushes
                a PR to <code>qa</code>. A code-review agent examines the work
                against the plan before the PR is created.
              </li>
              <li>
                <strong className="text-foreground">4. CI Watch:</strong> Agent
                monitors CI, fixes lint/format/config failures autonomously.
                Verification checks confirm the fix before claiming
                it&apos;s resolved.
              </li>
              <li>
                <strong className="text-foreground">5. QA Deploy:</strong> Kyle
                reviews PR, merges. QA deploys automatically.
              </li>
              <li>
                <strong className="text-foreground">6. Ship It:</strong> Kyle
                inspects QA, tells agent to ship. Agent merges to main, watches
                prod deploy, cleans up.
              </li>
            </ol>
          </div>
        </section>

        {/* Pipeline Optimization */}
        <section className="mt-12">
          <h2 className="text-xl font-semibold">Pipeline Optimization</h2>
          <p className="mt-2 text-muted-foreground leading-relaxed">
            Adding a RAG evaluation service exposed several performance
            bottlenecks. The eval service depends on RAGAS, which pulls in 200+
            transitive packages including LangChain. That single addition pushed
            the pipeline from a manageable ~10 minutes to 30+ minutes per run,
            with most of the time spent on redundant work. Here&apos;s how each
            bottleneck was diagnosed and fixed.
          </p>
          <div className="mt-4">
            <MermaidDiagram chart={optimizationDiagram} />
          </div>
        </section>

        {/* Optimization 1: Venv Caching */}
        <section className="mt-12">
          <h3 className="text-lg font-semibold">
            1. Virtualenv Caching
          </h3>
          <div className="mt-3 space-y-4 text-muted-foreground leading-relaxed">
            <p>
              <strong className="text-foreground">Problem:</strong>{" "}
              <code>pip install</code> ran from scratch on every CI run. For the
              eval service with its 200+ transitive dependencies, this took ~20
              minutes — longer than all other checks combined.
            </p>
            <p>
              <strong className="text-foreground">Investigation:</strong> The
              GitHub Actions runner starts fresh each time, so there was no pip
              cache to reuse. The eval service&apos;s dependency tree (RAGAS →
              LangChain → dozens of ML packages) made cold installs
              exceptionally slow.
            </p>
            <p>
              <strong className="text-foreground">Fix:</strong> Cache the
              entire <code>.venv</code> directory using{" "}
              <code>actions/cache@v4</code>, keyed on the hash of{" "}
              <code>requirements.txt</code> and{" "}
              <code>shared/pyproject.toml</code>. On cache hit, the install
              step is skipped entirely.
            </p>
            <pre className="overflow-x-auto rounded-lg border border-border bg-muted/50 p-4 text-sm">
{`# Cache key
venv-{service}-{hash(requirements.txt, shared/pyproject.toml)}

# On cache hit → skip pip install entirely`}
            </pre>
            <p>
              <strong className="text-foreground">Result:</strong> Eval tests
              went from <strong>20 minutes → 20 seconds</strong>. pip-audit
              dropped from <strong>20 minutes → 9 seconds</strong>.
            </p>
          </div>
        </section>

        {/* Optimization 2: Conditional Image Builds */}
        <section className="mt-12">
          <h3 className="text-lg font-semibold">
            2. Conditional Image Builds
          </h3>
          <div className="mt-3 space-y-4 text-muted-foreground leading-relaxed">
            <p>
              <strong className="text-foreground">Problem:</strong> All 11
              service images were rebuilt on every push, even when only one
              service changed. A one-line fix to the chat service triggered
              builds for all Go, Java, and Python images.
            </p>
            <p>
              <strong className="text-foreground">Investigation:</strong> The
              build matrix had no path awareness — every matrix entry ran
              unconditionally. Most builds were wasted compute producing
              identical images.
            </p>
            <p>
              <strong className="text-foreground">Fix:</strong> Each matrix
              entry declares a <code>paths</code> field listing the directories
              that affect its image. A <code>git diff HEAD~1</code> check at
              the start of each build job skips the build when none of those
              paths changed.
            </p>
            <pre className="overflow-x-auto rounded-lg border border-border bg-muted/50 p-4 text-sm">
{`- service: chat
  paths: services/chat services/shared
- service: go-auth-service
  paths: go/auth-service go/pkg go/go.work`}
            </pre>
            <p>
              <strong className="text-foreground">Result:</strong> A typical
              single-service change rebuilds{" "}
              <strong>1 image instead of 11</strong>. Unchanged services are
              skipped in ~20 seconds.
            </p>
          </div>
        </section>

        {/* Optimization 3: Compose Smoke */}
        <section className="mt-12">
          <h3 className="text-lg font-semibold">
            3. Compose Smoke: Pull Instead of Build
          </h3>
          <div className="mt-3 space-y-4 text-muted-foreground leading-relaxed">
            <p>
              <strong className="text-foreground">Problem:</strong>{" "}
              <code>docker compose up --build</code> rebuilt all Python images
              from source in CI, spending ~10 minutes per service on pip install
              with no layer cache (fresh runner each time).
            </p>
            <p>
              <strong className="text-foreground">Investigation:</strong> The
              compose-smoke job existed to verify service configuration — env
              vars, nginx routing, health checks, inter-service connectivity.
              It didn&apos;t need freshly built images to test those things.
              Code correctness was already covered by unit tests.
            </p>
            <p>
              <strong className="text-foreground">Fix:</strong> Pull pre-built{" "}
              <code>:latest</code> images from GHCR instead of building from
              source. The smoke tests verify configuration, not code.
            </p>
            <pre className="overflow-x-auto rounded-lg border border-border bg-muted/50 p-4 text-sm">
{`# Before: build from source (~15 min)
docker compose up --build

# After: pull pre-built images (~95 sec)
for svc in ingestion chat debug; do
  docker pull "ghcr.io/.../\${svc}:latest"
done
docker compose up -d`}
            </pre>
            <p>
              <strong className="text-foreground">Result:</strong> Compose
              smoke went from <strong>~15 minutes → 95 seconds</strong>.
            </p>
          </div>
        </section>

        {/* Optimization 4: Job Immutability */}
        <section className="mt-12">
          <h3 className="text-lg font-semibold">
            4. QA Deploy: Job Immutability Fix
          </h3>
          <div className="mt-3 space-y-4 text-muted-foreground leading-relaxed">
            <p>
              <strong className="text-foreground">Problem:</strong> QA deploys
              were failing entirely. The Go kustomize overlay includes
              migration Jobs, and Kubernetes Jobs are immutable — once created,
              their <code>spec.template</code> cannot be patched.
            </p>
            <p>
              <strong className="text-foreground">Investigation:</strong> When
              kustomize apply tried to update existing Jobs with a new image
              tag, Kubernetes rejected it with{" "}
              <code>field is immutable</code>. The Jobs had completed
              successfully on the previous deploy but were still present in the
              namespace.
            </p>
            <p>
              <strong className="text-foreground">Fix:</strong> Filter Jobs out
              of the kustomize output using awk, then handle them separately:
              delete the old Job, create the new one, wait for completion.
            </p>
            <pre className="overflow-x-auto rounded-lg border border-border bg-muted/50 p-4 text-sm">
{`# Apply overlay without Jobs
kubectl kustomize k8s/overlays/qa-go/ \\
  | awk '...filter out kind: Job...' \\
  | kubectl apply -f -

# Run migrations sequentially
kubectl delete job go-auth-migrate --ignore-not-found
kubectl apply -f auth-service-migrate.yml
kubectl wait --for=condition=complete job/go-auth-migrate`}
            </pre>
            <p>
              <strong className="text-foreground">Result:</strong> QA deploy
              went from <strong>failing → succeeding in 85 seconds</strong>.
            </p>
          </div>
        </section>

        {/* Optimization 5: Precise Change Detection */}
        <section className="mt-12">
          <h3 className="text-lg font-semibold">
            5. Precise Change Detection
          </h3>
          <div className="mt-3 space-y-4 text-muted-foreground leading-relaxed">
            <p>
              <strong className="text-foreground">Problem:</strong> The path
              filter from <em>#2</em> evolved with the codebase. The first
              version compared against <code>HEAD~1</code>, which silently
              missed rebuilds when a fix was pushed in a multi-commit batch
              (the diff only saw the final commit). A post-incident hardening
              widened it to <code>HEAD~5</code>. That fixed the missed-rebuild
              bug but introduced the opposite failure mode: once Go work
              landed in the last 5 commits, <em>every</em> subsequent push —
              including docs-only or frontend-only ones — re-ran the full Go
              test, lint, and image-build matrices.
            </p>
            <p>
              <strong className="text-foreground">Investigation:</strong>{" "}
              GitHub gives the exact range for every event: pushes carry{" "}
              <code>github.event.before</code> (the previous tip of the
              branch); PRs carry{" "}
              <code>github.event.pull_request.base.sha</code> (the merge
              base). Both are precise — <code>HEAD~N</code> was always a
              heuristic dressed up as a window. The same overshoot also hit
              the test and lint matrices, which never had path filtering at
              all and re-ran the full Python, Java, and Go test suites on
              every push regardless of what changed.
            </p>
            <p>
              <strong className="text-foreground">Fix:</strong> A composite
              action at <code>.github/actions/check-changes</code> picks the
              compare base based on the event type — push.before for pushes,
              PR base SHA for PRs, with <code>HEAD~5</code> kept only as a
              fallback for first pushes and <code>workflow_dispatch</code>.
              Wired into 14 gated jobs: the original three (go-tests,
              go-lint, build-and-push-images) plus python-tests,
              java-unit-tests, java-integration-tests, frontend-checks,
              k8s-manifest-validation, go-migration-test, all three
              compose-smoke jobs, security-pip-audit, and security-hadolint.
              Every gated entry&apos;s <code>paths:</code> value includes{" "}
              <code>ci.yml</code> and the action&apos;s own{" "}
              <code>action.yml</code>, so a workflow refactor triggers every
              matrix entry — a safeguard against silent pipeline regressions.
            </p>
            <pre className="overflow-x-auto rounded-lg border border-border bg-muted/50 p-4 text-sm">
{`- name: Check for changes
  id: changes
  uses: ./.github/actions/check-changes
  with:
    paths: services/chat services/shared
           .github/workflows/ci.yml
           .github/actions/check-changes/action.yml

# Subsequent steps gated on:
#   if: steps.changes.outputs.changed == 'true'`}
            </pre>
            <p>
              <strong className="text-foreground">Result:</strong> A docs-only
              push now skips ~14 matrix entries in seconds. The pipeline
              failure mode shifted with each iteration:{" "}
              <code>HEAD~1</code> silently missed rebuilds,{" "}
              <code>HEAD~5</code> silently over-rebuilt, and the precise
              push-range / PR-base approach rebuilds exactly what changed.
            </p>
            <p>
              <strong className="text-foreground">Lesson:</strong> The first
              merge after extending the gate broke CI entirely. Every run
              showed{" "}
              <em>
                &ldquo;This run likely failed because of a workflow file
                issue&rdquo;
              </em>{" "}
              with an empty workflow graph — no jobs visible because nothing
              ever started. Locally, the pre-flight check used{" "}
              <code>python -c &quot;yaml.safe_load(...)&quot;</code>, which
              passed because YAML accepts duplicate keys (last value wins).{" "}
              <code>actionlint</code> caught it instantly: a step had both
              the new gate{" "}
              <code>
                if: steps.changes.outputs.changed == &apos;true&apos;
              </code>{" "}
              and an existing{" "}
              <code>
                if: steps.venv-cache.outputs.cache-hit != &apos;true&apos;
              </code>{" "}
              — two <code>if:</code> keys on the same map, which
              GitHub&apos;s parser rejects.{" "}
              <strong className="text-foreground">
                Validate workflow files with <code>actionlint</code>, not
                just a YAML parser.
              </strong>{" "}
              YAML&apos;s permissiveness is the wrong shape for CI configs.
            </p>
          </div>
        </section>

        {/* Combined Impact */}
        <section className="mt-12">
          <h3 className="text-lg font-semibold">Combined Impact</h3>
          <p className="mt-2 text-sm text-muted-foreground">
            The five optimizations together reduced the pipeline from 30+
            minutes to ~5 minutes on a typical push, and a docs-only or
            single-stack push now skips most of it entirely.
          </p>
          <div className="mt-4 overflow-x-auto">
            <table className="w-full text-sm border-collapse">
              <thead>
                <tr className="border-b border-border">
                  <th className="py-2 pr-4 text-left font-medium">Stage</th>
                  <th className="py-2 px-4 text-left font-medium">Before</th>
                  <th className="py-2 px-4 text-left font-medium">After</th>
                </tr>
              </thead>
              <tbody className="text-muted-foreground">
                <tr className="border-b border-border/50">
                  <td className="py-2 pr-4">Python Tests (eval)</td>
                  <td className="py-2 px-4">20 min</td>
                  <td className="py-2 px-4 text-green-400">20 sec</td>
                </tr>
                <tr className="border-b border-border/50">
                  <td className="py-2 pr-4">pip-audit (eval)</td>
                  <td className="py-2 px-4">20 min</td>
                  <td className="py-2 px-4 text-green-400">9 sec</td>
                </tr>
                <tr className="border-b border-border/50">
                  <td className="py-2 pr-4">Compose Smoke</td>
                  <td className="py-2 px-4">~15 min</td>
                  <td className="py-2 px-4 text-green-400">95 sec</td>
                </tr>
                <tr className="border-b border-border/50">
                  <td className="py-2 pr-4">Image Builds (no change)</td>
                  <td className="py-2 px-4">~3 min each</td>
                  <td className="py-2 px-4 text-green-400">~20 sec (skipped)</td>
                </tr>
                <tr className="border-b border-border/50">
                  <td className="py-2 pr-4">Deploy QA</td>
                  <td className="py-2 px-4">failing</td>
                  <td className="py-2 px-4 text-green-400">85 sec</td>
                </tr>
                <tr className="border-b border-border/50">
                  <td className="py-2 pr-4">
                    Test/lint matrices (no change)
                  </td>
                  <td className="py-2 px-4">always 1-3 min each</td>
                  <td className="py-2 px-4 text-green-400">~30 sec (skipped)</td>
                </tr>
                <tr className="border-t border-border">
                  <td className="py-2 pr-4 font-medium text-foreground">
                    Total pipeline
                  </td>
                  <td className="py-2 px-4 font-medium text-foreground">
                    30+ min
                  </td>
                  <td className="py-2 px-4 font-medium text-green-400">
                    ~5 min
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>

        {/* Smoke Tests */}
        <section className="mt-12 mb-16">
          <h2 className="text-xl font-semibold">Smoke Tests</h2>
          <p className="mt-2 text-muted-foreground leading-relaxed">
            After every deployment, automated smoke tests verify the services are
            healthy. QA runs health endpoint checks against{" "}
            <code>qa-api.kylebradshaw.dev</code> covering the Python AI, Java,
            and Go stacks — auth, products, cart, orders, payments, and the
            saga happy-path. Production runs Playwright tests against the live
            site — including an end-to-end RAG flow that uploads a PDF, asks a
            question, and verifies a streamed response.
          </p>
        </section>
      </div>
    </div>
  );
}
