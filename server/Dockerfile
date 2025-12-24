# Build stage
FROM golang:1.21-alpine AS builder

RUN apk add --no-cache git gcc musl-dev

WORKDIR /build

# Copy the entire project for proper module resolution
COPY . .

WORKDIR /build/server

# Download dependencies
RUN go mod download

# Build the binary
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o wire-socket-server ./cmd/server

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache \
    ca-certificates \
    iptables \
    ip6tables \
    iproute2

# Create non-root user (but we need root for TUN device)
RUN addgroup -S wiresocket && adduser -S wiresocket -G wiresocket

WORKDIR /app

# Copy binary
COPY --from=builder /build/server/wire-socket-server /app/

# Copy default config
COPY server/config.yaml /app/config.yaml.example

# Create data directory
RUN mkdir -p /data /etc/wireguard && \
    chown -R wiresocket:wiresocket /data

# Environment
ENV GIN_MODE=release

# Expose ports
# 8080 - HTTP API
# 443  - WebSocket tunnel
# 51820/udp - WireGuard (if not using tunnel)
EXPOSE 8080 443 51820/udp

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget -q --spider http://localhost:8080/health || exit 1

# Volume for persistent data
VOLUME ["/data", "/etc/wireguard"]

# Run as root (required for TUN device creation)
USER root

ENTRYPOINT ["/app/wire-socket-server"]
CMD ["-config", "/data/config.yaml"]
