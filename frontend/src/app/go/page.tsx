import Link from "next/link";

export default function GoPage() {
  return (
    <div className="mx-auto max-w-3xl px-6 py-12">
      <h1 className="mt-8 text-3xl font-bold">Go Backend Developer</h1>

      {/* Bio Section */}
      <section className="mt-8">
        <p className="text-muted-foreground leading-relaxed">
          Go is my preferred language due to its readability, simplicity, and
          strong performance. It’s my first choice for many backend tasks, and
          I’ve used it to build microservices, automation scripts, and
          command-line tools with a focus on clean, efficient design.
        </p>
      </section>

      {/* Project Section */}
      <section className="mt-12">
        <h2 className="text-2xl font-semibold">Ecommerce Platform</h2>

        <p className="mt-4 text-muted-foreground leading-relaxed">
          Microservices ecommerce platform built with Go, demonstrating
          RESTful API design, JWT authentication, PostgreSQL, Redis caching,
          and asynchronous order processing with RabbitMQ. Deployed using
          Docker and Kubernetes.
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

        <div className="mt-6">
          <Link
            href="/go/ecommerce"
            className="inline-flex items-center gap-2 rounded-lg bg-primary px-6 py-3 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors"
          >
            View Project &rarr;
          </Link>
        </div>
      </section>
    </div>
  );
}