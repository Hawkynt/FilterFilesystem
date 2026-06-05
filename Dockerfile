# Build stage
FROM golang:alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make gcc musl-dev

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary and generate the example configuration shipped in the image
RUN make build example-config

# Final stage
FROM alpine:latest

# Install runtime dependencies (FUSE)
RUN apk add --no-cache fuse fuse-dev

# Create a non-root user
RUN addgroup -g 1001 filterfs && \
    adduser -D -s /bin/sh -u 1001 -G filterfs filterfs

# Copy the binary from builder stage
COPY --from=builder /app/bin/filterfs /usr/local/bin/filterfs

# Create mount directories
RUN mkdir -p /mnt/source /mnt/filtered && \
    chown filterfs:filterfs /mnt/source /mnt/filtered

# Copy example configuration
COPY --from=builder /app/filterfs.example.yaml /etc/filterfs/config.yaml

# Switch to non-root user
USER filterfs

# Expose default mount point
VOLUME ["/mnt/source", "/mnt/filtered"]

# Set entrypoint
ENTRYPOINT ["/usr/local/bin/filterfs"]
CMD ["mount", "--config", "/etc/filterfs/config.yaml"]