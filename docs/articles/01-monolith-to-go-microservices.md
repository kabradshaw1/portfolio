# From One Go Binary to Production-Style Microservices

## Feed Post

I rebuilt a Go ecommerce backend from a single service into a production-style
microservice system.

The interesting part was not "splitting the code." It was deciding where the
new operational complexity was worth it:

- database-per-service boundaries
- gRPC contracts between services
- RabbitMQ saga orchestration
- Kafka events for analytics and projections
- Kubernetes scaling and observability per service

The biggest lesson: microservices are not an architecture upgrade by default.
They are a tradeoff. The value comes from clear ownership, explicit contracts,
and operational tooling that makes failure visible.

#Golang #Microservices #BackendEngineering #Kubernetes #DistributedSystems

## Article

I started the ecommerce portion of my portfolio as a single Go service.

That first version handled products, carts, and orders in one binary. It was
simple, easy to run, and easy to reason about. That was the right starting
point. A small system should not begin distributed just because distributed
systems look better on an architecture diagram.

But I wanted the portfolio to demonstrate the work a backend engineer does when
a service boundary becomes real. So I decomposed the original ecommerce service
into independent Go services with separate responsibilities:

- auth-service
- product-service
- cart-service
- order-service
- payment-service
- ai-service
- analytics-service
- order-projector

The goal was not to create more moving parts. The goal was to practice the
engineering disciplines that make microservices survivable.

## The First Hard Boundary: Data Ownership

The most important decision was database ownership.

Each service owns its own PostgreSQL database. Product data lives in productdb.
Cart data lives in cartdb. Orders live in orderdb. Payments live in paymentdb.
The services do not share tables.

That sounds straightforward, but it changes the shape of the system. Once tables
are no longer shared, convenience queries disappear. The cart-service cannot
reach into the product tables. The order-service cannot directly mutate cart
rows. Anything crossing a boundary needs an API contract.

That is the point.

The database boundary forces the service boundary to be honest. If another
service needs data, it must ask through a contract that can be versioned,
tested, observed, and evolved.

## gRPC for Internal Contracts

For service-to-service calls, I used gRPC and Protobuf.

The frontend still talks to REST endpoints because that is the browser-facing
surface. Internally, Go services use typed gRPC contracts for operations like
cart lookup, product availability, and payment creation.

This gave me three benefits:

1. Explicit request and response shapes.
2. Generated clients instead of handwritten JSON glue.
3. A better place to add deadlines, metrics, and tracing around internal calls.

One lesson I learned quickly: a gRPC contract is only valuable if it is treated
as a product boundary. That means deadlines, error handling, observability, and
compatibility matter as much as the `.proto` file.

## Checkout Required a Saga

Checkout is where the decomposition becomes real.

The order-service creates a pending order, retrieves cart contents, validates
stock, creates a payment, clears the cart, and marks the order completed. In a
monolith, that can be a local transaction or a tightly coordinated sequence.

Across services, there is no single database transaction.

I used a saga orchestrator pattern. The order-service owns the workflow and
publishes commands through RabbitMQ. The cart-service reserves or releases cart
items. The product-service validates stock. The payment-service handles payment
intent creation. If a later step fails, the saga compensates by releasing
reserved items or refunding payment where needed.

The frontend receives a fast `201 Created` response with the order in a pending
state while the saga continues asynchronously.

That design made the user experience responsive, but it also created a new
requirement: the system needed to expose enough state to debug a workflow after
the HTTP request had already returned.

## Operational Work Became Part of the Feature

The decomposition was not complete when the services compiled.

Each service needed metrics. Each service needed logs with request context. Each
service needed Kubernetes resources, health checks, resource limits, and
horizontal scaling. The saga needed dead-letter handling, recovery behavior, and
visibility into stuck orders.

This was the main lesson: microservices move complexity out of code structure
and into operations.

If that operational layer is missing, the architecture is not production-grade.
It is just a distributed demo.

## What I Would Tell Another Engineer

Start with a monolith until the boundary is obvious.

Split for concrete reasons: independent scaling, clear ownership, different
data lifecycles, failure isolation, or a workflow that benefits from async
coordination. Do not split just because a diagram looks more impressive.

When you do split, make the boundary real:

- own the data separately
- define typed contracts
- add timeouts and retries deliberately
- emit metrics and structured logs
- trace requests across service boundaries
- test the failure paths, not just the happy path

For me, the value of this project was learning that microservice engineering is
less about creating services and more about making their failure modes
understandable.

That is the work I wanted this portfolio to demonstrate.
