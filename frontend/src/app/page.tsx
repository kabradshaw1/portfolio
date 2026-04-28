import Link from "next/link";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from "@/components/ui/card";

export default function Home() {
  return (
    <div className="min-h-screen bg-background text-foreground">
      <div className="mx-auto max-w-3xl px-6 py-16">
        {/* Name & Bio */}
        <h1 className="text-4xl font-bold">Kyle Bradshaw</h1>
        <p className="mt-6 text-lg text-muted-foreground leading-relaxed">
          Software engineer focused on building production systems with modern
          tooling. Since August 2022, I&apos;ve been working full-time on
          personal projects and consulting, with a focus on Go, TypeScript, and
          cloud-native infrastructure. This portfolio showcases three areas of
          specialization — agentic AI systems, Go backend services, and
          full-stack Java development.
        </p>
        <p className="mt-4 text-sm text-muted-foreground leading-relaxed">
          Every service in this portfolio ships Prometheus metrics to a live{" "}
          <a
            href="https://grafana.kylebradshaw.dev/d/system-overview/system-overview?orgId=1&from=now-1h&to=now&timezone=browser"
            target="_blank"
            rel="noopener noreferrer"
            className="underline hover:text-foreground transition-colors"
          >
            Grafana dashboard
          </a>
          .
        </p>

        {/* Sections */}
        <h2 className="mt-16 text-2xl font-semibold">Portfolio</h2>
        <div className="mt-6 grid gap-4">
          <Link href="/go" className="block">
            <Card className="hover:ring-foreground/20 transition-all">
              <CardHeader>
                <CardTitle>Go Backend Developer</CardTitle>
                <CardDescription>
                  Ecommerce platform built with Go, PostgreSQL, Redis, and
                  RabbitMQ
                </CardDescription>
              </CardHeader>
              <CardContent>
                <p className="text-muted-foreground text-sm">
                  Microservices architecture with JWT authentication, product
                  catalog, cart, orders, and asynchronous worker pools —
                  deployed on Kubernetes.
                </p>
              </CardContent>
            </Card>
          </Link>
          <Link href="/database" className="block">
            <Card className="hover:ring-foreground/20 transition-all">
              <CardHeader>
                <CardTitle>Database Engineering</CardTitle>
                <CardDescription>
                  Production PostgreSQL — pooling, replication, optimization,
                  partitioning, migration safety, and reliability
                </CardDescription>
              </CardHeader>
              <CardContent>
                <p className="text-muted-foreground text-sm">
                  Real benchmarks against PostgreSQL 16, transaction-mode
                  PgBouncer pooling, an async streaming read replica with a
                  separate reporting pool, range partitioning with materialized
                  views, a custom AST-based migration linter, and verified
                  point-in-time recovery.
                </p>
              </CardContent>
            </Card>
          </Link>
          <Link href="/ai" className="block">
            <Card className="hover:ring-foreground/20 transition-all">
              <CardHeader>
                <CardTitle>AI Engineer</CardTitle>
                <CardDescription>
                  Document Q&A Assistant built with RAG, FastAPI, Qdrant, and
                  Ollama
                </CardDescription>
              </CardHeader>
              <CardContent>
                <p className="text-muted-foreground text-sm">
                  A full-stack retrieval-augmented generation system
                  demonstrating PDF ingestion, vector search, prompt
                  engineering, and streaming LLM responses.
                </p>
              </CardContent>
            </Card>
          </Link>
          <Link href="/observability" className="block">
            <Card className="hover:ring-foreground/20 transition-all">
              <CardHeader>
                <CardTitle>Observability</CardTitle>
                <CardDescription>
                  Production-journey instrumentation — Prometheus metrics, Loki
                  logs, Jaeger traces, and live alerting
                </CardDescription>
              </CardHeader>
              <CardContent>
                <p className="text-muted-foreground text-sm">
                  Three-pillar stack with deploy annotations, Kubernetes event
                  exporter, gRPC client interceptors, saga-stalled alerts, and
                  Kafka-header trace propagation across the async boundary.
                </p>
              </CardContent>
            </Card>
          </Link>
          <Link href="/cicd" className="block">
            <Card className="hover:ring-foreground/20 transition-all">
              <CardHeader>
                <CardTitle>CI/CD Pipeline</CardTitle>
                <CardDescription>
                  Unified GitHub Actions workflow with a live QA environment at
                  qa.kylebradshaw.dev for pre-prod review
                </CardDescription>
              </CardHeader>
              <CardContent>
                <p className="text-muted-foreground text-sm">
                  A single workflow handles quality checks, image builds, and
                  deployments for three service stacks — designed for a solo
                  developer with automated spec-to-production delivery. See
                  what&apos;s currently staged for production review on the
                  CI/CD page.
                </p>
              </CardContent>
            </Card>
          </Link>
          <Link href="/aws" className="block">
            <Card className="hover:ring-foreground/20 transition-all">
              <CardHeader>
                <CardTitle>Infrastructure & Deployment</CardTitle>
                <CardDescription>
                  Production Kubernetes on a home server, AWS-ready with
                  Terraform and EKS
                </CardDescription>
              </CardHeader>
              <CardContent>
                <p className="text-muted-foreground text-sm">
                  Two deployment architectures for the same services — a
                  cost-effective Minikube cluster with Cloudflare Tunnel serving
                  production today, and a one-command AWS deployment with EKS,
                  RDS, ElastiCache, and Amazon MQ.
                </p>
              </CardContent>
            </Card>
          </Link>
          <Link href="/security" className="block">
            <Card className="hover:ring-foreground/20 transition-all">
              <CardHeader>
                <CardTitle>Security</CardTitle>
                <CardDescription>
                  Defense-in-depth across the stack — application, CI/CD,
                  Kubernetes, and the hardened Linux host that runs it all
                </CardDescription>
              </CardHeader>
              <CardContent>
                <p className="text-muted-foreground text-sm">
                  Six CI security gates, JWT + httpOnly cookies, pod security
                  contexts, Sealed Secrets for GitOps-friendly secret
                  management, UFW default-deny firewall, Tailscale-only SSH,
                  auditd, sysctl hardening, and a lynis baseline score of 77.
                </p>
              </CardContent>
            </Card>
          </Link>
          <Link href="/java" className="block">
            <Card className="hover:ring-foreground/20 transition-all">
              <CardHeader>
                <CardTitle>Full Stack Java Developer</CardTitle>
                <CardDescription>
                  Task Management System built with Spring Boot, GraphQL, and
                  Kubernetes
                </CardDescription>
              </CardHeader>
              <CardContent>
                <p className="text-muted-foreground text-sm">
                  Microservices architecture with PostgreSQL, MongoDB, Redis,
                  RabbitMQ, Google OAuth, and CI/CD automation.
                </p>
              </CardContent>
            </Card>
          </Link>
        </div>
      </div>
    </div>
  );
}
