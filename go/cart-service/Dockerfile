FROM migrate/migrate:v4.17.0 AS migrate

# Cart-service: REST :8096, gRPC :9096
FROM golang:1.26-alpine AS builder

WORKDIR /app/cart-service
COPY pkg/ /app/pkg/
COPY auth-service/ /app/auth-service/
COPY product-service/ /app/product-service/
COPY cart-service/go.mod cart-service/go.sum ./
RUN go mod download
COPY cart-service/ .
RUN CGO_ENABLED=0 GOOS=linux go build -o /cart-service ./cmd/server

FROM alpine:3.19

# hadolint ignore=DL3018
RUN apk add --no-cache postgresql-client \
    && adduser -D -u 1001 appuser

COPY --from=builder /cart-service /cart-service
COPY --from=migrate /usr/local/bin/migrate /usr/local/bin/migrate
COPY cart-service/migrations/ /migrations/

USER appuser

EXPOSE 8096 9096
ENTRYPOINT ["/cart-service"]
