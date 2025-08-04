FROM golang:1.23-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod ./

# Download dependencies and generate go.sum
RUN go mod download && go mod tidy

# Copy source code
COPY . .

# Get dependencies and build the application
RUN go get -d -v ./... && CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o tm2-service .

# Final stage
FROM alpine:latest

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1001 -S tm2 && \
    adduser -u 1001 -S tm2 -G tm2

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/tm2-service .

# Change ownership to non-root user
RUN chown -R tm2:tm2 /app

# Switch to non-root user
USER tm2

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the application
CMD ["./tm2-service"] 