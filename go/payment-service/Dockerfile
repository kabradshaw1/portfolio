FROM migrate/migrate:v4.17.0 AS migrate

# Payment-service: REST :8098, gRPC :9098
FROM golang:1.26-alpine AS builder

WORKDIR /app/payment-service
COPY pkg/ /app/pkg/
COPY payment-service/go.mod payment-service/go.sum ./
RUN go mod download
COPY payment-service/ .
ARG BUILD_VERSION=dev
ARG BUILD_COMMIT=unknown
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-X github.com/kabradshaw1/portfolio/go/pkg/buildinfo.Version=${BUILD_VERSION} -X github.com/kabradshaw1/portfolio/go/pkg/buildinfo.GitSHA=${BUILD_COMMIT}" \
    -o /payment-service ./cmd/server

FROM alpine:3.19

# hadolint ignore=DL3018
RUN apk add --no-cache postgresql-client \
    && adduser -D -u 1001 appuser

COPY --from=builder /payment-service /payment-service
COPY --from=migrate /usr/local/bin/migrate /usr/local/bin/migrate
COPY payment-service/migrations/ /migrations/

USER appuser

EXPOSE 8098 9098
ENTRYPOINT ["/payment-service"]
