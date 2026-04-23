FROM golang:1.26-alpine AS builder

WORKDIR /app/analytics-service
COPY pkg/ /app/pkg/
COPY analytics-service/go.mod analytics-service/go.sum ./
RUN go mod download
COPY analytics-service/ .
ARG BUILD_VERSION=dev
ARG BUILD_COMMIT=unknown
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-X github.com/kabradshaw1/portfolio/go/pkg/buildinfo.Version=${BUILD_VERSION} -X github.com/kabradshaw1/portfolio/go/pkg/buildinfo.GitSHA=${BUILD_COMMIT}" \
    -o /analytics-service ./cmd/server

FROM alpine:3.19

RUN adduser -D -u 1001 appuser

COPY --from=builder /analytics-service /analytics-service

USER appuser

EXPOSE 8094
ENTRYPOINT ["/analytics-service"]
