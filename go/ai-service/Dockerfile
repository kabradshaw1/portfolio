FROM golang:1.26-alpine AS builder

WORKDIR /app/ai-service
COPY pkg/ /app/pkg/
COPY ai-service/go.mod ai-service/go.sum ./
RUN go mod download
COPY ai-service/ .
ARG BUILD_VERSION=dev
ARG BUILD_COMMIT=unknown
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-X github.com/kabradshaw1/portfolio/go/pkg/buildinfo.Version=${BUILD_VERSION} -X github.com/kabradshaw1/portfolio/go/pkg/buildinfo.GitSHA=${BUILD_COMMIT}" \
    -o /ai-service ./cmd/server

FROM alpine:3.19

# hadolint ignore=DL3018
RUN apk add --no-cache ca-certificates \
    && adduser -D -u 1001 appuser

COPY --from=builder /ai-service /ai-service

USER appuser

EXPOSE 8093
ENTRYPOINT ["/ai-service"]
