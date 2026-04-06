FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /auth-service ./cmd/server

FROM alpine:3.19

RUN adduser -D -u 1001 appuser
COPY --from=builder /auth-service /auth-service
USER appuser

EXPOSE 8091
ENTRYPOINT ["/auth-service"]
