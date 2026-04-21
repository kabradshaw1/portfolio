FROM migrate/migrate:v4.17.0 AS migrate

FROM golang:1.26-alpine AS builder

WORKDIR /app/ecommerce-service
COPY pkg/ /app/pkg/
COPY product-service/ /app/product-service/
COPY ecommerce-service/go.mod ecommerce-service/go.sum ./
RUN go mod download
COPY ecommerce-service/ .
RUN CGO_ENABLED=0 GOOS=linux go build -o /ecommerce-service ./cmd/server

FROM alpine:3.19

# postgresql-client is not pinned because Alpine's rolling repos rotate
# available versions; locking one breaks rebuilds when Alpine updates.
# hadolint ignore=DL3018
RUN apk add --no-cache postgresql-client \
    && adduser -D -u 1001 appuser

COPY --from=builder /ecommerce-service /ecommerce-service
COPY --from=migrate /usr/local/bin/migrate /usr/local/bin/migrate
COPY ecommerce-service/migrations/ /migrations/
COPY ecommerce-service/seed.sql /seed.sql

USER appuser

EXPOSE 8092
ENTRYPOINT ["/ecommerce-service"]
