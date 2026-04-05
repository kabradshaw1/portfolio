import Link from "next/link";

export default function JavaPage() {
  return (
    <div className="mx-auto max-w-3xl px-6 py-12">
      <Link
        href="/"
        className="text-sm text-muted-foreground hover:text-foreground transition-colors"
      >
        &larr; Home
      </Link>

      <h1 className="mt-8 text-3xl font-bold">Full Stack Java Developer</h1>

      <section className="mt-8">
        <p className="text-muted-foreground leading-relaxed">
          Full-stack microservices architecture with Spring Boot, GraphQL, and
          event-driven communication. This section demonstrates a task
          management platform built with four Java services, PostgreSQL,
          MongoDB, Redis, and RabbitMQ — deployed on Kubernetes.
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
