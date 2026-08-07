# syntax=docker/dockerfile:1

# --- Build stage -------------------------------------------------------------
# Compiles a static binary. Dependencies are downloaded in their own layer so
# that editing source code does not re-download the module cache.
FROM golang:1.25-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO is disabled so the binary is fully static and carries no libc dependency.
# -trimpath keeps build machine paths out of the image, and the ldflags strip
# the symbol table and DWARF data, roughly halving the binary.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/server ./cmd/server

# --- Test stage --------------------------------------------------------------
# Not built by default. `docker build --target test` runs the suite against the
# exact sources that produce the image.
FROM build AS test
RUN go vet ./... && go test ./tests/... -count=1

# --- Runtime stage -----------------------------------------------------------
FROM alpine:3.20 AS runtime

# ca-certificates for any outbound TLS; wget (busybox) backs the health check.
RUN apk add --no-cache ca-certificates && \
    addgroup -g 10001 -S abacus && \
    adduser -u 10001 -S abacus -G abacus

COPY --from=build /out/server /usr/local/bin/server

# Running unprivileged means a container escape does not start from root.
USER abacus:abacus

EXPOSE 8080

ENV APP_ENV=production \
    PORT=8080

# The service has no dependencies, so liveness and readiness are one question.
HEALTHCHECK --interval=15s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --quiet --spider --tries=1 http://127.0.0.1:8080/api/v1/health || exit 1

ENTRYPOINT ["/usr/local/bin/server"]
