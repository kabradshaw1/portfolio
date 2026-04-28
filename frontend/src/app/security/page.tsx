import { MermaidDiagram } from "@/components/MermaidDiagram";
import { Badge } from "@/components/ui/badge";

const REPO = "https://github.com/kabradshaw1/gen_ai_engineer";

const defenseInDepthDiagram = `flowchart TD
  INET["Internet"]
  CF["Cloudflare Tunnel<br/><i>TLS edge · outbound-only</i>"]
  HOST["Debian 13 Host<br/><i>UFW default-deny · auditd · sysctl · Tailscale-only SSH</i>"]
  K8S["Minikube Cluster<br/><i>per-Deployment securityContext · readiness probes</i>"]
  APP["Application Layer<br/><i>JWT validation · httpOnly cookies · rate limiting · prompt-injection defenses</i>"]
  DATA["Data Layer<br/><i>bcrypt passwords · Redis token denylist · secretKeyRef injection</i>"]

  INET --> CF --> HOST --> K8S --> APP --> DATA`;

const STATUS_ROWS: { area: string; status: string; variant: "default" | "secondary" | "outline"; notes: string }[] = [
  { area: "Shift-left security in CI", status: "Strong", variant: "default", notes: "Six gating jobs: Bandit, pip-audit, npm audit, gitleaks, Hadolint, CORS guardrail" },
  { area: "Infrastructure-as-Code validation", status: "Strong", variant: "default", notes: "kubeconform, kind dry-run, custom policy-as-code script" },
  { area: "Supply chain", status: "Adequate", variant: "secondary", notes: "Multi-stage non-root builds, private GHCR; no image signing or digest pinning" },
  { area: "Secrets management", status: "Strong", variant: "default", notes: "Sealed Secrets controller in-cluster (Phase 1–2 of 6 shipped); committed SealedSecret resources, full-history gitleaks, K8s secretKeyRef injection" },
  { area: "Application AuthN/AuthZ", status: "Strong", variant: "default", notes: "JWT + bcrypt + OAuth 2.0, httpOnly cookies, Redis token revocation, Python JWT enforcement" },
  { area: "Transport security", status: "Adequate", variant: "secondary", notes: "TLS at Cloudflare edge; no direct internet exposure; intra-cluster traffic unencrypted" },
  { area: "Developer guardrails", status: "Strong", variant: "default", notes: "Pre-commit hooks, preflight Makefile targets, structured branch workflow" },
  { area: "Post-deploy verification", status: "Strong", variant: "default", notes: "Production Playwright smoke tests + compose-smoke CI job" },
  { area: "Kubernetes runtime posture", status: "Adequate", variant: "secondary", notes: "Pod security contexts on every Deployment; NetworkPolicy and namespace PSS still gaps" },
  { area: "Observability", status: "Foundation", variant: "outline", notes: "Prometheus + Grafana dashboards; no security alerting" },
  { area: "Host / OS hardening", status: "Strong", variant: "default", notes: "Debian 13: UFW, SSH Tailscale-only, narrow sudo, auditd, sysctl, lynis 77" },
];

export default function SecurityPage() {
  return (
    <div className="mx-auto max-w-3xl px-6 py-12">
      <h1 className="mt-8 text-3xl font-bold">Security</h1>

      {/* Intro */}
      <section className="mt-8">
        <p className="text-muted-foreground leading-relaxed">
          I treat security as a layered concern — every request crosses the
          perimeter, the host, the cluster, and the application before it touches
          data. Every claim below cites the file (and often the line) that
          implements it; the full assessments live alongside the code in the
          repo:{" "}
          <a
            href={`${REPO}/blob/main/docs/security/security-assessment.md`}
            target="_blank"
            rel="noopener noreferrer"
            className="underline hover:text-foreground transition-colors"
          >
            security-assessment.md
          </a>{" "}
          and{" "}
          <a
            href={`${REPO}/blob/main/docs/security/linux-server-hardening.md`}
            target="_blank"
            rel="noopener noreferrer"
            className="underline hover:text-foreground transition-colors"
          >
            linux-server-hardening.md
          </a>
          .
        </p>
      </section>

      {/* Defense-in-depth diagram */}
      <section className="mt-12">
        <h2 className="text-2xl font-semibold">Defense in Depth</h2>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          Five layers of controls compose from the internet edge to the data
          store. A compromise at any single layer is contained by the layers
          below it.
        </p>
        <div className="mt-6 rounded-xl border border-foreground/10 bg-card p-6">
          <MermaidDiagram chart={defenseInDepthDiagram} />
        </div>
      </section>

      {/* Zero-attack-surface callout */}
      <section className="mt-8">
        <div className="rounded-lg border border-foreground/10 bg-card p-4">
          <p className="text-sm text-muted-foreground">
            <span className="font-semibold text-foreground">
              Public attack surface: zero.
            </span>{" "}
            <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
              nmap
            </code>{" "}
            against the public IP returns no open ports. Cloudflare Tunnel is
            outbound-only; SSH binds only to the Tailscale interface; Ollama is
            firewall-fenced to docker bridges and the tailnet.
          </p>
        </div>
      </section>

      {/* Status summary table */}
      <section className="mt-12">
        <h2 className="text-2xl font-semibold">Summary of Findings</h2>
        <div className="mt-6 overflow-x-auto">
          <table className="w-full text-sm text-muted-foreground">
            <thead>
              <tr className="border-b text-left">
                <th className="pb-2 pr-4 font-medium text-foreground">Area</th>
                <th className="pb-2 pr-4 font-medium text-foreground">
                  Status
                </th>
                <th className="pb-2 font-medium text-foreground">Notes</th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {STATUS_ROWS.map((row) => (
                <tr key={row.area}>
                  <td className="py-2 pr-4 text-foreground">{row.area}</td>
                  <td className="py-2 pr-4">
                    <Badge variant={row.variant}>{row.status}</Badge>
                  </td>
                  <td className="py-2">{row.notes}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>

      {/* Application & Infrastructure Highlights */}
      <section className="mt-12">
        <h2 className="text-2xl font-semibold">
          Application & Infrastructure
        </h2>

        <div className="mt-8 space-y-8">
          <div>
            <h3 className="text-lg font-medium">
              Authentication & Authorization
            </h3>
            <p className="mt-2 text-muted-foreground leading-relaxed">
              JWT access and refresh tokens (HMAC-SHA256, 15-min / 7-day TTLs)
              with bcrypt password hashes. Both Go services explicitly validate
              the signing method to prevent alg-confusion attacks. Tokens are
              delivered as httpOnly cookies (Secure, SameSite=Lax) — the
              frontend never touches the JWT in JavaScript. On logout, the token
              hash is written to a Redis denylist with matching TTL. Python AI
              services share a JWT auth dependency that enforces authentication
              on every mutating endpoint.
            </p>
            <p className="mt-2 text-sm text-muted-foreground">
              Full breakdown:{" "}
              <a
                href={`${REPO}/blob/main/docs/security/security-assessment.md#5-application-authnauthorz`}
                target="_blank"
                rel="noopener noreferrer"
                className="underline hover:text-foreground transition-colors"
              >
                §5 of security-assessment.md
              </a>
            </p>
          </div>

          <div>
            <h3 className="text-lg font-medium">
              Secrets Management — Sealed Secrets{" "}
              <span className="ml-2 align-middle text-xs font-normal text-muted-foreground">
                (in progress · 2 of 6 phases shipped)
              </span>
            </h3>
            <p className="mt-2 text-muted-foreground leading-relaxed">
              Cluster Secrets are migrating from live{" "}
              <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
                kubectl
              </code>{" "}
              edits to committed{" "}
              <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
                SealedSecret
              </code>{" "}
              resources at{" "}
              <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
                k8s/secrets/&lt;namespace&gt;/&lt;name&gt;.sealed.yml
              </code>
              . The Bitnami sealed-secrets controller in{" "}
              <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
                kube-system
              </code>{" "}
              decrypts each resource on apply using a controller-held private
              key; the encrypted file in the repo is the single source of
              truth, and only the in-cluster controller can decrypt it. This
              enables GitOps for secrets without committing plaintext.
            </p>
            <p className="mt-2 text-muted-foreground leading-relaxed">
              <span className="text-foreground font-medium">Phase 1</span> —
              controller install, public-key export, sealing helper script (
              <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
                scripts/seal-from-cluster.sh
              </code>
              ).{" "}
              <span className="text-foreground font-medium">Phase 2</span> —
              four live application Secrets converted to{" "}
              <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
                SealedSecret
              </code>{" "}
              files, plaintext templates removed from the repo.
            </p>
            <p className="mt-2 text-muted-foreground leading-relaxed">
              <span className="text-foreground font-medium">
                Phases 3–6 queued:
              </span>{" "}
              key-rotation runbook, audit-trail integration with the existing
              gitleaks gate, prod cutover for the remaining Secrets, and
              retiring the last hand-managed credentials.
            </p>
            <p className="mt-2 text-sm text-muted-foreground">
              Migration decision:{" "}
              <a
                href={`${REPO}/blob/main/docs/adr/security/secrets-management.md`}
                target="_blank"
                rel="noopener noreferrer"
                className="underline hover:text-foreground transition-colors"
              >
                secrets-management.md
              </a>{" "}
              · Day-to-day rules:{" "}
              <a
                href={`${REPO}/blob/main/docs/adr/security/secrets-and-config-practices.md`}
                target="_blank"
                rel="noopener noreferrer"
                className="underline hover:text-foreground transition-colors"
              >
                secrets-and-config-practices.md
              </a>
            </p>
          </div>

          <div>
            <h3 className="text-lg font-medium">Shift-Left CI</h3>
            <p className="mt-2 text-muted-foreground leading-relaxed">
              Six dedicated security jobs in the GitHub Actions workflow gate
              every push: Bandit (Python SAST), pip-audit (dependency CVEs), npm
              audit, gitleaks (full-history secret scanning), Hadolint
              (Dockerfile lint), and a custom CORS guardrail that blocks
              wildcard origins. The deploy job declares{" "}
              <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
                needs:
              </code>{" "}
              on all six — a single failure blocks production deployment.
            </p>
            <p className="mt-2 text-sm text-muted-foreground">
              Full breakdown:{" "}
              <a
                href={`${REPO}/blob/main/docs/security/security-assessment.md#1-shift-left-security-in-ci`}
                target="_blank"
                rel="noopener noreferrer"
                className="underline hover:text-foreground transition-colors"
              >
                §1 of security-assessment.md
              </a>
            </p>
          </div>

          <div>
            <h3 className="text-lg font-medium">Kubernetes Runtime</h3>
            <p className="mt-2 text-muted-foreground leading-relaxed">
              Every Deployment manifest sets{" "}
              <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
                runAsNonRoot: true
              </code>
              ,{" "}
              <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
                readOnlyRootFilesystem: true
              </code>
              ,{" "}
              <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
                allowPrivilegeEscalation: false
              </code>
              , and{" "}
              <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
                capabilities.drop: [&quot;ALL&quot;]
              </code>
              . Readiness probes on every stateful service are enforced by a
              custom policy-as-code script in CI. Remaining gaps: namespace-level
              Pod Security Standards and NetworkPolicy are documented as accepted
              risks.
            </p>
            <p className="mt-2 text-sm text-muted-foreground">
              Full breakdown:{" "}
              <a
                href={`${REPO}/blob/main/docs/security/security-assessment.md#9-kubernetes-runtime-posture`}
                target="_blank"
                rel="noopener noreferrer"
                className="underline hover:text-foreground transition-colors"
              >
                §9 of security-assessment.md
              </a>
            </p>
          </div>

          <div>
            <h3 className="text-lg font-medium">Supply Chain</h3>
            <p className="mt-2 text-muted-foreground leading-relaxed">
              Multi-stage Docker builds on every service — the builder stage
              compiles code; the final image is slim/Alpine with an explicit
              non-root{" "}
              <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
                USER
              </code>
              . Images are pushed to a private GHCR registry with{" "}
              <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
                imagePullSecrets
              </code>{" "}
              on every Deployment. Tool versions in CI are pinned for
              reproducibility. Accepted gap: images are tagged{" "}
              <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
                :latest
              </code>{" "}
              rather than pinned by digest.
            </p>
            <p className="mt-2 text-sm text-muted-foreground">
              Full breakdown:{" "}
              <a
                href={`${REPO}/blob/main/docs/security/security-assessment.md#3-supply-chain`}
                target="_blank"
                rel="noopener noreferrer"
                className="underline hover:text-foreground transition-colors"
              >
                §3 of security-assessment.md
              </a>
            </p>
          </div>
        </div>
      </section>

      {/* Linux Host Hardening */}
      <section className="mt-12">
        <h2 className="text-2xl font-semibold">Linux Host Hardening</h2>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          The production server is a hand-installed Debian 13 box with an RTX
          3090. SSH binds only to the Tailscale IP — public SSH is gone entirely.
          UFW runs a default-deny firewall with narrow allow rules (during
          hardening, a pre-existing rule that silently exposed Ollama to the home
          LAN was discovered and removed). Privileged operations use a narrow
          passwordless sudo allowlist for routine ops; privilege-changing actions
          still require a password.
        </p>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
            auditd
          </code>{" "}
          runs with immutable baseline rules covering identity files, sudo/sshd
          configs, and privilege-escalation invocations. A{" "}
          <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
            sysctl
          </code>{" "}
          drop-in hardens the kernel (kptr_restrict, ptrace_scope,
          unprivileged_bpf, network-stack hygiene).{" "}
          <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
            fail2ban
          </code>{" "}
          watches SSH with a 1-hour ban after 3 attempts. Lynis hardening index:{" "}
          <span className="font-semibold text-foreground">77</span>.
        </p>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          The most consequential finding was a silent gap in patch management:{" "}
          <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
            unattended-upgrades
          </code>{" "}
          was active and configured to allow security-origin packages, but{" "}
          <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
            /etc/apt/sources.list
          </code>{" "}
          was missing the{" "}
          <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
            security.debian.org
          </code>{" "}
          repository since the migration. Adding it pulled 15 stale patches
          including a kernel update (6.12.73 → 6.12.74).
        </p>
        <p className="mt-4 text-sm text-muted-foreground">
          Full breakdown:{" "}
          <a
            href={`${REPO}/blob/main/docs/security/linux-server-hardening.md`}
            target="_blank"
            rel="noopener noreferrer"
            className="underline hover:text-foreground transition-colors"
          >
            linux-server-hardening.md
          </a>
        </p>
      </section>

      {/* Recommended next steps */}
      <section className="mt-12">
        <h2 className="text-2xl font-semibold">Recommended Next Steps</h2>
        <ul className="mt-4 list-disc pl-6 text-muted-foreground space-y-2">
          <li>
            <span className="text-foreground font-medium">
              Pod Security Standards
            </span>{" "}
            — add PSS{" "}
            <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
              restricted
            </code>{" "}
            labels to all namespaces (the per-Deployment security contexts
            already satisfy it)
          </li>
          <li>
            <span className="text-foreground font-medium">NetworkPolicy</span>{" "}
            — default-deny ingress in each namespace with explicit allow rules
            for gateway→downstream and ingress→gateway
          </li>
          <li>
            <span className="text-foreground font-medium">
              Remote audit log forwarding
            </span>{" "}
            — send host auditd/journald to a remote sink so the audit trail
            survives a host compromise
          </li>
          <li>
            <span className="text-foreground font-medium">
              Image digest pinning
            </span>{" "}
            — replace{" "}
            <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
              :latest
            </code>{" "}
            tags with{" "}
            <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
              @sha256:…
            </code>{" "}
            digests to eliminate mutable-tag supply chain risk
          </li>
        </ul>
      </section>

      {/* Evidence footer */}
      <section className="mt-12 mb-8">
        <p className="text-sm text-muted-foreground leading-relaxed">
          Every assertion above is verifiable in the repo. Start at{" "}
          <a
            href={`${REPO}/blob/main/docs/security/security-assessment.md`}
            target="_blank"
            rel="noopener noreferrer"
            className="underline hover:text-foreground transition-colors"
          >
            security-assessment.md
          </a>{" "}
          or{" "}
          <a
            href={`${REPO}/blob/main/docs/security/linux-server-hardening.md`}
            target="_blank"
            rel="noopener noreferrer"
            className="underline hover:text-foreground transition-colors"
          >
            linux-server-hardening.md
          </a>
          , or browse the cited source files directly on GitHub.
        </p>
      </section>
    </div>
  );
}
