FROM migrate/migrate:v4.17.0 AS migrate

# Product-service: REST :8095, gRPC :9095
FROM golang:1.26-alpine AS builder

WORKDIR /app/product-service
COPY pkg/ /app/pkg/
COPY product-service/go.mod product-service/go.sum ./
RUN go mod download
COPY product-service/ .
ARG BUILD_VERSION=dev
ARG BUILD_COMMIT=unknown
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-X github.com/kabradshaw1/portfolio/go/pkg/buildinfo.Version=${BUILD_VERSION} -X github.com/kabradshaw1/portfolio/go/pkg/buildinfo.GitSHA=${BUILD_COMMIT}" \
    -o /product-service ./cmd/server

FROM alpine:3.19

# hadolint ignore=DL3018
RUN apk add --no-cache postgresql-client \
    && adduser -D -u 1001 appuser

COPY --from=builder /product-service /product-service
COPY --from=migrate /usr/local/bin/migrate /usr/local/bin/migrate
COPY product-service/migrations/ /migrations/
COPY product-service/seed.sql /seed.sql

USER appuser

EXPOSE 8095 9095
ENTRYPOINT ["/product-service"]
