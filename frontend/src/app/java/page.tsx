import Link from "next/link";
import { MermaidDiagram } from "@/components/MermaidDiagram";

export default function JavaPage() {
  return (
    <div className="mx-auto max-w-3xl px-6 py-12">
      <h1 className="mt-8 text-3xl font-bold">Full Stack Java Developer</h1>

      <section className="mt-8">
        <p className="text-muted-foreground leading-relaxed">
          Full-stack microservices architecture with Spring Boot, GraphQL, and
          event-driven communication. This section demonstrates a task
          management platform built with four Java services, PostgreSQL,
          MongoDB, Redis, and RabbitMQ — deployed on Kubernetes.
        </p>
        <p className="mt-4 text-sm text-muted-foreground leading-relaxed">
          Every Java service exposes Prometheus metrics to a live{" "}
          <a
            href="https://api.kylebradshaw.dev/grafana/"
            target="_blank"
            rel="noopener noreferrer"
            className="underline hover:text-foreground transition-colors"
          >
            Grafana dashboard
          </a>
          .
        </p>
      </section>

      <section className="mt-12">
        <h2 className="text-2xl font-semibold">Task Management System</h2>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          A full-stack project management application demonstrating Spring Boot
          microservices, PostgreSQL, MongoDB, Redis, RabbitMQ, GraphQL, Google
          OAuth, and Kubernetes — all orchestrated with Docker Compose and
          CI/CD via GitHub Actions.
        </p>

        <h3 className="mt-6 text-lg font-medium">Tech Stack</h3>
        <ul className="mt-2 list-disc pl-6 text-muted-foreground space-y-1">
          <li>4 Spring Boot microservices (gateway, task, activity, notification)</li>
          <li>GraphQL API gateway with JWT authentication</li>
          <li>PostgreSQL (tasks), MongoDB (activity logs), Redis (notifications)</li>
          <li>RabbitMQ event-driven architecture</li>
          <li>Google OAuth 2.0 login</li>
          <li>Next.js + TypeScript + Apollo Client frontend</li>
          <li>Minikube Kubernetes deployment (production), Docker Compose (local dev)</li>
          <li>CI/CD with GitHub Actions, Testcontainers, security scanning</li>
        </ul>
      </section>

      <section className="mt-12">
  <h2 className="text-2xl font-semibold">Architecture</h2>
  <p className="mt-4 text-muted-foreground leading-relaxed">
    Four Spring Boot services fronted by a GraphQL gateway. Writes land in
    Postgres; events fan out through RabbitMQ to the activity and
    notification services, which persist into MongoDB.
  </p>
  <div className="mt-6">
    <MermaidDiagram
      chart={`flowchart LR
  FE[Next.js Frontend]
  GW[gateway-service<br/>GraphQL]
  TS[task-service<br/>Spring Boot]
  AS[activity-service<br/>Spring Boot]
  NS[notification-service<br/>Spring Boot]
  PG[(PostgreSQL)]
  MG[(MongoDB)]
  RD[(Redis cache)]
  MQ{{RabbitMQ}}
  FE -->|GraphQL| GW
  GW -->|REST| TS
  GW -->|REST| AS
  GW -->|REST| NS
  TS --> PG
  TS -->|publish task.events| MQ
  MQ -->|consume| AS
  MQ -->|consume| NS
  AS --> MG
  AS --> RD
  NS --> MG`}
    />
  </div>

  <h3 className="mt-10 text-xl font-semibold">Request flow: Create a task</h3>
  <p className="mt-4 text-muted-foreground leading-relaxed">
    One click traces through the gateway, into task-service, onto RabbitMQ,
    and fans out to activity and notification consumers in parallel.
  </p>
  <div className="mt-6">
    <MermaidDiagram
      chart={`sequenceDiagram
  participant U as User
  participant FE as Frontend
  participant GW as gateway
  participant TS as task-service
  participant PG as Postgres
  participant MQ as RabbitMQ
  participant AS as activity-service
  participant NS as notification-service
  participant MG as MongoDB
  U->>FE: Click "Create task"
  FE->>GW: mutation createTask
  GW->>TS: POST /tasks
  TS->>PG: INSERT task
  PG-->>TS: ok
  TS->>MQ: publish task.created
  TS-->>GW: 201 task
  GW-->>FE: task payload
  par fan-out
    MQ->>AS: task.created
    AS->>MG: insert activity
  and
    MQ->>NS: task.created
    NS->>MG: insert notification
  end
  FE->>GW: poll myNotifications (30s)
  GW->>NS: GET /notifications
  NS-->>FE: unread badge updates`}
    />
  </div>
</section>

      <section className="mt-12">
        <Link
          href="/java/tasks"
          className="inline-flex items-center gap-2 rounded-lg bg-primary px-6 py-3 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors"
        >
          Open Task Manager &rarr;
        </Link>
      </section>
    </div>
  );
}
