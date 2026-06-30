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
          Full-stack engineer — React, Go and Python microservices, and LLM/RAG
          integration. Four years of experience, the last stretch spent
          consulting and building production systems independently: designing
          the APIs, shipping the frontends, and running the whole stack on
          Kubernetes. Everything below is deployed and instrumented, not a demo.
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

        {/* Featured Project */}
        <h2 className="mt-16 text-2xl font-semibold">Featured Project</h2>
        <div className="mt-6 grid gap-4">
          <Link href="/galaxy" className="block">
            <Card className="hover:ring-foreground/20 transition-all">
              <CardHeader>
                <CardTitle>GalaxyVoyagers.com</CardTitle>
                <CardDescription>
                  Deployed collaborative sci-fi worldbuilding platform with a
                  Go GraphQL gateway and AI-assisted creation tools
                </CardDescription>
              </CardHeader>
              <CardContent>
                <p className="text-muted-foreground text-sm">
                  Built with Next.js, Apollo Client, Go, gqlgen, gRPC,
                  PostgreSQL, MongoDB, Redis, RabbitMQ, and AI generation.
                  View the architecture walkthrough for the full system design.
                </p>
              </CardContent>
            </Card>
          </Link>
        </div>

        {/* Backend & Data Engineering */}
        <h2 className="mt-16 text-2xl font-semibold">
          Backend &amp; Data Engineering
        </h2>
        <div className="mt-6 grid gap-4">
          <Link href="/go" className="block">
            <Card className="hover:ring-foreground/20 transition-all">
              <CardHeader>
                <CardTitle>Go Ecommerce Platform</CardTitle>
                <CardDescription>
                  Microservices ecommerce platform built with Go, PostgreSQL,
                  Redis, and RabbitMQ
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
          <Link href="/java" className="block">
            <Card className="hover:ring-foreground/20 transition-all">
              <CardHeader>
                <CardTitle>Full-Stack Java</CardTitle>
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
          <Link href="/async" className="block">
            <Card className="hover:ring-foreground/20 transition-all">
              <CardHeader>
                <CardTitle>Asynchronous Systems</CardTitle>
                <CardDescription>
                  Go ecommerce messaging with Kafka event streams, RabbitMQ
                  sagas, DLQs, replay, and production observability
                </CardDescription>
              </CardHeader>
              <CardContent>
                <p className="text-muted-foreground text-sm">
                  Checkout saga command/reply queues, bounded retries, publisher
                  confirms, reconnect-aware RabbitMQ publishing, Kafka-backed
                  order events, CQRS projection, streaming analytics, DLQ
                  envelopes, and traceable recovery paths.
                </p>
              </CardContent>
            </Card>
          </Link>
        </div>

        {/* AI Systems */}
        <h2 className="mt-16 text-2xl font-semibold">AI Systems</h2>
        <div className="mt-6 grid gap-4">
          <Link href="/ai/ir-agent" className="block">
            <Card className="border-primary/30 hover:ring-foreground/20 transition-all">
              <CardHeader>
                <CardTitle>Incident-Response Agent</CardTitle>
                <CardDescription>
                  A LangGraph multi-agent system that triages, investigates,
                  validates, and reports on security incidents — with role-tiered
                  Claude models and measured cost/latency telemetry
                </CardDescription>
              </CardHeader>
              <CardContent>
                <p className="text-muted-foreground text-sm">
                  Four specialized agents pass typed state through a graph with a
                  bounded validator loop. Each node runs the cheapest capable
                  Claude model (Haiku → Opus → Sonnet), and every run reports its
                  real per-role tokens, cost, latency, and savings versus
                  Opus-everywhere. Streamed over SSE with a free, always-on replay
                  demo.
                </p>
              </CardContent>
            </Card>
          </Link>

          <Link href="/ai" className="block">
            <Card className="hover:ring-foreground/20 transition-all">
              <CardHeader>
                <CardTitle>Document Q&amp;A Assistant</CardTitle>
                <CardDescription>
                  A RAG document assistant plus a Kafka-scale sensitive-data
                  classifier — retrieval-augmented generation and applied LLM
                  classification
                </CardDescription>
              </CardHeader>
              <CardContent>
                <p className="text-muted-foreground text-sm">
                  A full-stack retrieval-augmented generation system (FastAPI,
                  Qdrant, Ollama) for PDF Q&amp;A, plus a DSPM classifier that
                  detects sensitive data at Kafka scale with a tiered regex
                  &rarr; NER &rarr; LLM pipeline. Explore both from the AI page.
                </p>
              </CardContent>
            </Card>
          </Link>
        </div>

        {/* Platform & Operations */}
        <h2 className="mt-16 text-2xl font-semibold">
          Platform &amp; Operations
        </h2>
        <div className="mt-6 grid gap-4">
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
                <CardTitle>Infrastructure &amp; Deployment</CardTitle>
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
        </div>
      </div>
    </div>
  );
}
