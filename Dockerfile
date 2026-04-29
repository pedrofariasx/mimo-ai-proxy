#
# File: Dockerfile
# Project: mimoproxy
# Purpose: Container definition
# Created: 2026-04-28
#

# Build stage
FROM golang:1.26-alpine AS builder

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -tags netgo -a -installsuffix cgo -o mimoproxy main.go

# Final stage
FROM alpine:latest

# Install CA certificates for HTTPS requests
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Create data directory for SQLite
RUN mkdir -p /app/data && chmod 777 /app/data

# Copy the binary from the builder stage
COPY --from=builder /app/mimoproxy .
# Copy .env.example as template if needed, though we expect .env to be mounted
COPY .env.example .env

EXPOSE 3000

CMD ["./mimoproxy"]
