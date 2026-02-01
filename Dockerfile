# Build Stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install git for fetching dependencies (if needed)
RUN apk add --no-cache git

# Copy go.mod and go.sum first to leverage layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY . .

# Build the binary
# CGO_ENABLED=0 for static binary
RUN CGO_ENABLED=0 GOOS=linux go build -o api cmd/api/main.go

# Run Stage
FROM alpine:latest

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/api .

# Expose port
EXPOSE 8080

# Run
CMD ["./api"]
