FROM golang:1.26-alpine AS builder

WORKDIR /app/order-projector
COPY pkg/ /app/pkg/
COPY order-projector/go.mod order-projector/go.sum ./
RUN go mod download
COPY order-projector/ .
ARG BUILD_VERSION=dev
ARG BUILD_COMMIT=unknown
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-X github.com/kabradshaw1/portfolio/go/pkg/buildinfo.Version=${BUILD_VERSION} -X github.com/kabradshaw1/portfolio/go/pkg/buildinfo.GitSHA=${BUILD_COMMIT}" \
    -o /order-projector ./cmd/server

FROM alpine:3.19

RUN adduser -D -u 1001 appuser

COPY --from=builder /order-projector /order-projector

USER appuser

EXPOSE 8097
ENTRYPOINT ["/order-projector"]
