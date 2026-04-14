import { MermaidDiagram } from "@/components/MermaidDiagram";

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

        {/* Smoke Tests */}
        <section className="mt-12 mb-16">
          <h2 className="text-xl font-semibold">Smoke Tests</h2>
          <p className="mt-2 text-muted-foreground leading-relaxed">
            After every deployment, automated smoke tests verify the services are
            healthy. QA runs health endpoint checks against{" "}
            <code>qa-api.kylebradshaw.dev</code>. Production runs Playwright
            tests against the live site — including an end-to-end RAG flow that
            uploads a PDF, asks a question, and verifies a streamed response.
          </p>
        </section>
      </div>
    </div>
  );
}
