# Build stage
FROM golang:1.23-alpine AS builder

# Install required tools
RUN apk add --no-cache git make

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the collector with otlpeek exporter
RUN go install go.opentelemetry.io/collector/cmd/builder@v0.126.0
RUN builder --config=cmd/builder-config.yaml

# Runtime stage
FROM alpine:latest

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates

# Copy the built collector binary
COPY --from=builder /app/dist/otlpeek /usr/local/bin/otlpeek

# Copy runtime configuration
COPY cmd/config.yaml /etc/otlpeek/config.yaml

# Create non-root user
RUN addgroup -g 1000 otel && \
    adduser -D -s /bin/sh -u 1000 -G otel otel

# Switch to non-root user
USER otel

# Expose the ports
EXPOSE 8080 4317 4318

# Run the collector
ENTRYPOINT ["/usr/local/bin/otlpeek"]
CMD ["--config", "/etc/otlpeek/config.yaml"] 