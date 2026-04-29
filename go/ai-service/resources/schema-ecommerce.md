# Ecommerce Data Model

Sanitized ER summary of the ecommerce databases. Each service owns its own database on a shared Postgres instance. No credentials or host names are included here.

## orderdb

### orders
| Column | Type | Notes |
|---|---|---|
| id | uuid PK | |
| user_id | uuid | FK → auth.users.id (cross-DB logical reference) |
| status | text | pending, confirmed, cancelled, failed |
| saga_step | text | reserve_stock, reserve_cart, charge_payment, confirm, rollback |
| total | numeric(12,2) | order total in dollars |
| created_at | timestamptz | |
| updated_at | timestamptz | |

### order_items
| Column | Type | Notes |
|---|---|---|
| id | uuid PK | |
| order_id | uuid FK | → orders.id |
| product_id | uuid | FK → productdb.products.id (cross-DB logical reference) |
| quantity | int | |
| price_at_purchase | numeric(12,2) | snapshot price; immutable after creation |

## productdb

### products
| Column | Type | Notes |
|---|---|---|
| id | uuid PK | |
| name | text | |
| category | text | |
| price | numeric(12,2) | current list price |
| stock | int | available inventory |
| created_at | timestamptz | |

## cartdb

### cart_items
| Column | Type | Notes |
|---|---|---|
| id | uuid PK | |
| user_id | uuid | FK → auth.users.id (cross-DB logical reference) |
| product_id | uuid | FK → productdb.products.id (cross-DB logical reference) |
| quantity | int | |
| reserved | bool | true while checkout saga holds a stock reservation |
| created_at | timestamptz | |

## paymentdb

### payments
| Column | Type | Notes |
|---|---|---|
| order_id | uuid PK | FK → orderdb.orders.id (cross-DB logical reference) |
| stripe_payment_intent_id | text | Stripe PI identifier |
| status | text | pending, succeeded, failed, refunded |
| amount | numeric(12,2) | amount charged in dollars |
| created_at | timestamptz | |

### payment_outbox
Transactional outbox table used to guarantee at-least-once delivery of payment events to RabbitMQ without distributed transactions.

| Column | Type | Notes |
|---|---|---|
| id | uuid PK | |
| order_id | uuid | |
| event_type | text | payment.succeeded, payment.failed |
| payload | jsonb | |
| published | bool | false until the outbox relay picks it up |
| created_at | timestamptz | |

## Cross-Database Relationships

Because each service has its own database, foreign keys across database boundaries are enforced at the application layer, not at the DB level:

- `orders.user_id` → `authdb.users.id` — order-service validates the user exists via auth-service JWT claims.
- `order_items.product_id` → `productdb.products.id` — order-service calls product-service (gRPC) to validate product IDs at checkout.
- `cart_items.user_id` → `authdb.users.id` — cart-service validates via JWT.
- `cart_items.product_id` → `productdb.products.id` — cart-service calls product-service to look up prices.
- `payments.order_id` → `orderdb.orders.id` — payment-service receives the order ID from the saga orchestrator.
