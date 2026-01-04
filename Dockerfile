FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY *.go ./

# Build the application
RUN CGO_ENABLED=1 go build -o stellar-mesh

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates sqlite-libs

WORKDIR /root/

# Copy binary from builder
COPY --from=builder /app/stellar-mesh .

# Expose default port
EXPOSE 8080

# Volume for data persistence
VOLUME ["/root/data"]

ENTRYPOINT ["./stellar-mesh"]
CMD ["-address", "0.0.0.0:8080"]
