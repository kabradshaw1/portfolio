FROM golang:1.26-alpine AS builder

WORKDIR /app/analytics-service
COPY pkg/ /app/pkg/
COPY analytics-service/go.mod analytics-service/go.sum ./
RUN go mod download
COPY analytics-service/ .
RUN CGO_ENABLED=0 GOOS=linux go build -o /analytics-service ./cmd/server

FROM alpine:3.19

RUN adduser -D -u 1001 appuser

COPY --from=builder /analytics-service /analytics-service

USER appuser

EXPOSE 8094
ENTRYPOINT ["/analytics-service"]
