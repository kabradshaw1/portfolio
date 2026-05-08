# What I Learned Building Async Reliability with RabbitMQ and Kafka

## Feed Post

I used both RabbitMQ and Kafka in the same Go ecommerce system.

Not because "more tools" is better, but because they solved different problems:

- RabbitMQ coordinates checkout saga commands, replies, retries, compensation,
  DLQs, and replay.
- Kafka carries durable order events for analytics, event sourcing, CQRS
  projections, replay, and consumer-lag monitoring.

The biggest lesson: async systems are only production-grade when failures are
designed as first-class workflows.

#Golang #Kafka #RabbitMQ #Microservices #BackendEngineering

## Article

One of the most useful parts of this portfolio project was building two
different async patterns in the same Go ecommerce system.

I used RabbitMQ for checkout saga coordination and Kafka for event streams,
analytics, and projections. They are both message infrastructure, but they are
not interchangeable in the way I used them.

RabbitMQ was a better fit for command/reply workflows where a service needs to
ask another service to do something. Kafka was a better fit for recording facts
that other services can consume, replay, and use to build read models.

That distinction shaped the design.

## RabbitMQ: Coordinating the Checkout Saga

Checkout is a workflow with several steps:

- create the order
- reserve cart items
- validate stock
- create payment
- clear the cart
- complete or compensate the order

Those steps cross service boundaries. The order-service owns the saga and
publishes commands through RabbitMQ. The cart-service receives commands like
`reserve.items` and `clear.cart`, then replies with events like `items.reserved`
or `cart.cleared`.

RabbitMQ worked well here because the messages represent commands and replies.
They are part of a business workflow with compensation paths, bounded retries,
and operator recovery.

The key reliability work was not just "send a message." It was:

- publisher confirms
- reconnect-aware publishing
- bounded retry behavior
- explicit dead-letter queues
- enough message envelope context to debug failures
- graceful shutdown
- crash recovery from the last known saga step

The DLQ became an operational feature, not an afterthought. Failed messages can
be inspected through an admin panel, including the order ID, saga step, routing
key, and error that caused the nack. After the root cause is fixed, the message
can be replayed back through the saga exchange.

That is the difference between a queue that hides failure and a queue that helps
operators recover from it.

## Kafka: Recording Durable Order Facts

Kafka served a different purpose.

When orders move through the saga, the order-service publishes domain events
such as order created, reserved, payment initiated, payment completed, and
completed. Those events become an append-only stream.

That stream powers two important capabilities.

First, the analytics-service consumes order and cart events and maintains
sliding-window aggregations: revenue per hour, trending products, and cart
abandonment signals. This avoids waiting for batch analytics to tell me what
happened yesterday. The system can show what is happening now.

Second, the order-projector consumes order events and builds CQRS read models:
a full order timeline, a denormalized order summary, and hourly order stats.
Those read models are independent from the order-service write database.

That separation matters. Reporting and timeline reads do not compete with
checkout writes. The read schema can evolve around UI query patterns. If the
projection needs to be rebuilt, the projector can replay from Kafka.

## Idempotency Is Not Optional

Async systems deliver messages more than once. Consumers restart. Offsets can
be retried. A publish can succeed while an acknowledgement fails.

That means idempotency is not a nice-to-have. It is part of the contract.

For the saga, duplicate commands and replies need to be safe. For the projector,
timeline inserts and summary updates need to tolerate repeated events. For
analytics, consumer behavior needs to avoid turning a retry into corrupted
aggregates.

The pattern I came back to was simple: every async boundary needs an answer to
"what happens if this message is processed twice?"

If there is no answer, the system is not reliable yet.

## Observability Makes Async Work Debuggable

The hardest bugs in async systems happen after the original request has
returned.

A checkout request can create a pending order successfully, then fail minutes
later in a consumer. A Kafka projector can fall behind without any user-facing
HTTP endpoint failing. A DLQ can slowly fill while the main service still looks
healthy.

So I added metrics and tracing around the async layer:

- Kafka consumer lag
- DLQ depth
- saga progress counters
- publish outcomes
- worker health
- trace propagation through RabbitMQ and Kafka headers

The goal is to make the async path visible enough that an alert can lead to the
right logs, and the logs can lead to the right trace.

## What I Learned

RabbitMQ and Kafka solved different problems.

RabbitMQ fit a command-oriented saga where retry, compensation, and operator
replay mattered. Kafka fit durable event streams where replay, projection, and
analytics mattered.

The larger lesson was that messaging is not reliability by itself. Reliability
comes from the surrounding engineering:

- clear message contracts
- retry classification
- DLQs with useful context
- idempotent consumers
- replay paths
- metrics and trace propagation
- focused tests around failure behavior

Async architecture adds power, but it also adds distance between cause and
effect. The job of the engineer is to make that distance observable,
recoverable, and boring to operate.
