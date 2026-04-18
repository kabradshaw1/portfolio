import { MermaidDiagram } from "@/components/MermaidDiagram";

const currentProductionDiagram = `flowchart TD
  subgraph Internet
    VERCEL[Vercel CDN<br/>kylebradshaw.dev]
    CF[Cloudflare Edge<br/>api.kylebradshaw.dev]
  end

  CF -->|Cloudflare Tunnel| CFD[cloudflared<br/>Windows PC]
  CFD --> INGRESS[NGINX Ingress Controller<br/>localhost:80]

  subgraph Minikube["Minikube Cluster"]
    INGRESS --> AI
    INGRESS --> JAVA
    INGRESS --> GO
    subgraph AI["ai-services"]
      direction LR
      ING[ingestion]
      CHAT[chat]
      DEBUG[debug]
      QD[(Qdrant)]
    end
    subgraph JAVA["java-tasks"]
      direction LR
      GW[gateway]
      TASK[task]
      ACT[activity]
      NOTIFY[notification]
      PG[(PostgreSQL)]
      MONGO[(MongoDB)]
      REDIS[(Redis)]
      RMQ{{RabbitMQ}}
    end
    subgraph GO["go-ecommerce"]
      direction LR
      AUTH[auth]
      ECOM[ecommerce]
      AIAGENT[ai-agent]
    end
  end

  OLLAMA[Ollama<br/>RTX 3090] -.->|host.minikube.internal| AI`;

const awsDeploymentDiagram = `flowchart TD
  DNS[Cloudflare DNS<br/>api.kylebradshaw.dev] --> ALB[AWS ALB<br/>Load Balancer Controller]

  subgraph EKS["EKS Cluster"]
    ALB --> AI2
    ALB --> JAVA2
    ALB --> GO2
    subgraph AI2["ai-services"]
      direction LR
      ING2[ingestion]
      CHAT2[chat]
      DEBUG2[debug]
      QD2[(Qdrant)]
    end
    subgraph JAVA2["java-tasks"]
      direction LR
      GW2[gateway]
      TASK2[task]
      ACT2[activity]
      NOTIFY2[notification]
    end
    subgraph GO2["go-ecommerce"]
      direction LR
      AUTH2[auth]
      ECOM2[ecommerce]
      AIAGENT2[ai-agent]
    end
  end

  subgraph Managed["AWS Managed Services"]
    RDS[(RDS PostgreSQL)]
    EC[(ElastiCache Redis)]
    MQ{{Amazon MQ RabbitMQ}}
    ATLAS[(MongoDB Atlas)]
  end

  JAVA2 --> RDS
  JAVA2 --> EC
  JAVA2 --> MQ
  GO2 --> RDS
  GO2 --> EC
  GO2 --> MQ
  GROQ[Groq API] -.->|LLM inference| AI2`;

export default function AWSPage() {
  return (
    <div className="mx-auto max-w-3xl px-6 py-12">
      <h1 className="mt-8 text-3xl font-bold">Infrastructure & Deployment</h1>

      {/* Intro */}
      <section className="mt-8">
        <p className="text-muted-foreground leading-relaxed">
          Every service in this portfolio runs in Kubernetes — today on a home
          server behind Cloudflare Tunnel, and optionally on AWS with EKS and
          managed services. The home server costs nothing to run. The AWS
          deployment spins up in 15 minutes with a single script and tears down
          after to keep costs near zero. Same application code, different
          infrastructure — swapped via Kustomize overlays.
        </p>
      </section>

      {/* Current Production */}
      <section className="mt-12">
        <h2 className="text-2xl font-semibold">Current Production</h2>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          The frontend is a Next.js app on Vercel. API traffic hits
          Cloudflare&apos;s edge network, which routes it through an
          outbound-only Cloudflare Tunnel to a Debian 13 running a Minikube
          Kubernetes cluster. Three namespaces isolate services by concern.
          Ollama runs natively on the host to access the RTX 3090 GPU directly.
          No ports are opened, no public IP is exposed, and no port forwarding
          is configured.
        </p>
        <div className="mt-6 rounded-xl border border-foreground/10 bg-card p-6">
          <MermaidDiagram chart={currentProductionDiagram} />
        </div>
      </section>

      {/* AWS Deployment */}
      <section className="mt-12">
        <h2 className="text-2xl font-semibold">AWS Deployment</h2>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          The same services deployed on EKS, with self-managed infrastructure
          replaced by AWS managed services. Terraform provisions the VPC, EKS
          cluster, RDS, ElastiCache, Amazon MQ, ECR repositories, and the ALB
          Ingress controller. Kustomize overlays swap connection strings and
          ingress annotations without changing application code. LLM inference
          moves from local Ollama to the Groq API.
        </p>
        <div className="mt-6 rounded-xl border border-foreground/10 bg-card p-6">
          <MermaidDiagram chart={awsDeploymentDiagram} />
        </div>
      </section>

      {/* Comparison Table */}
      <section className="mt-12">
        <h2 className="text-2xl font-semibold">
          What Changes Between Environments
        </h2>
        <div className="mt-6 overflow-x-auto">
          <table className="w-full text-sm text-muted-foreground">
            <thead>
              <tr className="border-b text-left">
                <th className="pb-2 pr-4 font-medium text-foreground">
                  Concern
                </th>
                <th className="pb-2 pr-4 font-medium text-foreground">
                  Home Server
                </th>
                <th className="pb-2 font-medium text-foreground">AWS</th>
              </tr>
            </thead>
            <tbody className="divide-y">
              <tr>
                <td className="py-2 pr-4">Kubernetes</td>
                <td className="py-2 pr-4">Minikube (Docker driver)</td>
                <td className="py-2">EKS (managed)</td>
              </tr>
              <tr>
                <td className="py-2 pr-4">Ingress</td>
                <td className="py-2 pr-4">NGINX Ingress Controller</td>
                <td className="py-2">AWS ALB (LB Controller)</td>
              </tr>
              <tr>
                <td className="py-2 pr-4">PostgreSQL</td>
                <td className="py-2 pr-4">Self-hosted in K8s</td>
                <td className="py-2">RDS</td>
              </tr>
              <tr>
                <td className="py-2 pr-4">Redis</td>
                <td className="py-2 pr-4">Self-hosted in K8s</td>
                <td className="py-2">ElastiCache</td>
              </tr>
              <tr>
                <td className="py-2 pr-4">RabbitMQ</td>
                <td className="py-2 pr-4">Self-hosted in K8s</td>
                <td className="py-2">Amazon MQ</td>
              </tr>
              <tr>
                <td className="py-2 pr-4">MongoDB</td>
                <td className="py-2 pr-4">Self-hosted in K8s</td>
                <td className="py-2">MongoDB Atlas (free tier)</td>
              </tr>
              <tr>
                <td className="py-2 pr-4">LLM inference</td>
                <td className="py-2 pr-4">Ollama (local RTX 3090)</td>
                <td className="py-2">Groq API</td>
              </tr>
              <tr>
                <td className="py-2 pr-4">Embeddings</td>
                <td className="py-2 pr-4">Ollama nomic-embed-text</td>
                <td className="py-2">OpenAI API</td>
              </tr>
              <tr>
                <td className="py-2 pr-4">DNS / TLS</td>
                <td className="py-2 pr-4">Cloudflare Tunnel</td>
                <td className="py-2">Cloudflare DNS + ACM</td>
              </tr>
              <tr>
                <td className="py-2 pr-4">CI/CD deploy</td>
                <td className="py-2 pr-4">SSH → kubectl (Tailscale)</td>
                <td className="py-2">GitHub OIDC → EKS</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      {/* Spin-Up Workflow */}
      <section className="mt-12">
        <h2 className="text-2xl font-semibold">One-Command Deployment</h2>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          <code className="rounded bg-muted px-1.5 py-0.5 text-sm">
            ./scripts/aws-up.sh
          </code>{" "}
          brings the full AWS stack up in ~15-20 minutes:
        </p>
        <ol className="mt-4 list-decimal pl-6 text-muted-foreground space-y-2">
          <li>
            <span className="text-foreground font-medium">Bootstrap</span> —
            creates S3 state bucket and DynamoDB lock table (first run only)
          </li>
          <li>
            <span className="text-foreground font-medium">Terraform apply</span>{" "}
            — provisions VPC, EKS, RDS, ElastiCache, Amazon MQ, ECR, and ALB
            controller
          </li>
          <li>
            <span className="text-foreground font-medium">
              Configure kubectl
            </span>{" "}
            — connects to the new EKS cluster
          </li>
          <li>
            <span className="text-foreground font-medium">Deploy services</span>{" "}
            — applies all Kubernetes manifests using Kustomize AWS overlays
          </li>
          <li>
            <span className="text-foreground font-medium">DNS handoff</span> —
            prints the ALB hostname for Cloudflare DNS configuration
          </li>
        </ol>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          Tear-down is equally simple:{" "}
          <code className="rounded bg-muted px-1.5 py-0.5 text-sm">
            ./scripts/aws-down.sh
          </code>{" "}
          destroys all infrastructure except the S3 state bucket and ECR images
          (~5 minutes).
        </p>
      </section>

      {/* Cost Breakdown */}
      <section className="mt-12">
        <h2 className="text-2xl font-semibold">Cost</h2>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          The AWS deployment is designed for spin-up/tear-down — run it for a
          demo, destroy it after. This keeps monthly costs near zero.
        </p>

        <h3 className="mt-6 text-lg font-medium">Running (~$5-9/day)</h3>
        <div className="mt-2 overflow-x-auto">
          <table className="w-full text-sm text-muted-foreground">
            <thead>
              <tr className="border-b text-left">
                <th className="pb-2 pr-4 font-medium text-foreground">
                  Resource
                </th>
                <th className="pb-2 font-medium text-foreground">Cost/day</th>
              </tr>
            </thead>
            <tbody className="divide-y">
              <tr>
                <td className="py-2 pr-4">EKS control plane</td>
                <td className="py-2">$3.30</td>
              </tr>
              <tr>
                <td className="py-2 pr-4">2x t3.medium nodes</td>
                <td className="py-2">$2.00</td>
              </tr>
              <tr>
                <td className="py-2 pr-4">RDS db.t3.micro</td>
                <td className="py-2">$0.50</td>
              </tr>
              <tr>
                <td className="py-2 pr-4">ElastiCache cache.t3.micro</td>
                <td className="py-2">$0.50</td>
              </tr>
              <tr>
                <td className="py-2 pr-4">Amazon MQ mq.t3.micro</td>
                <td className="py-2">$0.80</td>
              </tr>
              <tr>
                <td className="py-2 pr-4">NAT Gateway</td>
                <td className="py-2">$1.10</td>
              </tr>
              <tr>
                <td className="py-2 pr-4">ALB</td>
                <td className="py-2">$0.80</td>
              </tr>
            </tbody>
          </table>
        </div>

        <h3 className="mt-6 text-lg font-medium">Torn down (~$0.11/month)</h3>
        <div className="mt-2 overflow-x-auto">
          <table className="w-full text-sm text-muted-foreground">
            <thead>
              <tr className="border-b text-left">
                <th className="pb-2 pr-4 font-medium text-foreground">
                  Resource
                </th>
                <th className="pb-2 font-medium text-foreground">Cost/month</th>
              </tr>
            </thead>
            <tbody className="divide-y">
              <tr>
                <td className="py-2 pr-4">S3 state bucket</td>
                <td className="py-2">~$0.01</td>
              </tr>
              <tr>
                <td className="py-2 pr-4">ECR images</td>
                <td className="py-2">~$0.10</td>
              </tr>
              <tr>
                <td className="py-2 pr-4">MongoDB Atlas (free tier)</td>
                <td className="py-2">$0</td>
              </tr>
            </tbody>
          </table>
        </div>

        <p className="mt-4 text-sm text-muted-foreground leading-relaxed">
          This cost profile is why the home server remains the primary
          production deployment.
        </p>
      </section>
    </div>
  );
}
