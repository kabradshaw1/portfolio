FROM migrate/migrate:v4.17.0 AS migrate

FROM golang:1.26-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /ecommerce-service ./cmd/server

FROM alpine:3.19

RUN apk add --no-cache postgresql-client
RUN adduser -D -u 1001 appuser

COPY --from=builder /ecommerce-service /ecommerce-service
COPY --from=migrate /usr/local/bin/migrate /usr/local/bin/migrate
COPY migrations/ /migrations/
COPY seed.sql /seed.sql

USER appuser

EXPOSE 8092
ENTRYPOINT ["/ecommerce-service"]
