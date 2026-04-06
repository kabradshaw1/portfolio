FROM golang:1.26-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /ecommerce-service ./cmd/server

FROM alpine:3.19

RUN adduser -D -u 1001 appuser
COPY --from=builder /ecommerce-service /ecommerce-service
USER appuser

EXPOSE 8092
ENTRYPOINT ["/ecommerce-service"]
