import Link from "next/link";

export default function GoPage() {
  return (
    <div className="mx-auto max-w-3xl px-6 py-12">
      <h1 className="mt-8 text-3xl font-bold">Go Backend Developer</h1>

      <section className="mt-8">
        <p className="text-muted-foreground leading-relaxed">
          Microservices ecommerce platform built with Go, demonstrating
          RESTful API design, JWT authentication, PostgreSQL, Redis caching,
          and asynchronous order processing with RabbitMQ — deployed on
          Kubernetes.
        </p>
      </section>

      <section className="mt-12">
        <h2 className="text-2xl font-semibold">Ecommerce Platform</h2>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          A full-stack ecommerce storefront with product browsing, shopping
          cart, and order management. Two Go microservices handle
          authentication and core ecommerce operations, backed by PostgreSQL,
          Redis, and RabbitMQ.
        </p>

        <h3 className="mt-6 text-lg font-medium">Tech Stack</h3>
        <ul className="mt-2 list-disc pl-6 text-muted-foreground space-y-1">
          <li>2 Go microservices (auth-service, ecommerce-service)</li>
          <li>Gin HTTP framework with JWT authentication</li>
          <li>PostgreSQL (users, products, carts, orders)</li>
          <li>Redis product caching for fast reads</li>
          <li>RabbitMQ asynchronous order processing</li>
          <li>Prometheus metrics instrumentation</li>
          <li>Next.js + TypeScript frontend</li>
          <li>Docker Compose (local dev), Kubernetes (production)</li>
        </ul>
      </section>

      <section className="mt-12">
        <Link
          href="/go/ecommerce"
          className="inline-flex items-center gap-2 rounded-lg bg-primary px-6 py-3 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors"
        >
          Open Store &rarr;
        </Link>
      </section>
    </div>
  );
}
