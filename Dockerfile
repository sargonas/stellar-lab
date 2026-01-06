# Build stage
FROM golang:1.21-alpine AS builder

# Install build dependencies for CGO (SQLite)
RUN apk add --no-cache gcc musl-dev

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY *.go ./

# Build with CGO enabled for SQLite
RUN CGO_ENABLED=1 GOOS=linux go build -a -ldflags '-linkmode external -extldflags "-static"' -o stellar-lab .

# Runtime stage - minimal image
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/stellar-lab .

# Create data directory
RUN mkdir -p /data

# Expose ports
EXPOSE 8080 7867

# Default entrypoint - all config via env vars or CLI args
ENTRYPOINT ["/app/stellar-lab"]
