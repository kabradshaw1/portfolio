FROM migrate/migrate:v4.17.0 AS migrate

FROM golang:1.26-alpine AS builder

WORKDIR /app/auth-service
COPY pkg/ /app/pkg/
COPY auth-service/go.mod auth-service/go.sum ./
RUN go mod download
COPY auth-service/ .
RUN CGO_ENABLED=0 GOOS=linux go build -o /auth-service ./cmd/server

FROM alpine:3.19

# postgresql-client is not pinned because Alpine's rolling repos rotate
# available versions; locking one breaks rebuilds when Alpine updates.
# hadolint ignore=DL3018
RUN apk add --no-cache postgresql-client \
    && adduser -D -u 1001 appuser

COPY --from=builder /auth-service /auth-service
COPY --from=migrate /usr/local/bin/migrate /usr/local/bin/migrate
COPY auth-service/migrations/ /migrations/

USER appuser

EXPOSE 8091
ENTRYPOINT ["/auth-service"]
