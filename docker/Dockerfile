# Build stage
# Use a Debian-based builder to match the Ubuntu runner (better for CGO/glibc compatibility)
FROM golang:1.25.0-alpine AS builder

# Use build arguments to change the binary name
ARG BINARY_NAME=recommender-backend

WORKDIR /app

# Copy module files from the backend directory
COPY A1CE_recommender_code/backend/go.mod ./

# Copy the source code from the backend directory
COPY A1CE_recommender_code/backend/ .

# Run go get to generate go.sum
RUN go get a1ce-recommender-backend

# Build the Go application
# CGO_ENABLED=1 is required for github.com/mattn/go-sqlite3
RUN CGO_ENABLED=1 GOOS=linux go build -o ${BINARY_NAME} .

# Final stage
FROM ubuntu:22.04

# Re-declare ARG and set as ENV so it persists in the runner stage
ARG BINARY_NAME=recommender-backend
ENV BINARY_NAME=${BINARY_NAME}

# Create a non-root user
RUN groupadd -r appuser && useradd -r -g appuser appuser

WORKDIR /app

# Install CA certificates for HTTPS requests, and sqlite3 if needed for debug
RUN apt-get update && apt-get install -y ca-certificates sqlite3 && rm -rf /var/lib/apt/lists/*

# Copy the binary from the builder stage
COPY --chown=appuser:appuser --from=builder /app/${BINARY_NAME} .

# Copy necessary data files (JSON, CSV, DB) that the application might need to read at runtime
COPY --chown=appuser:appuser --from=builder /app/*.json ./
COPY --chown=appuser:appuser --from=builder /app/*.csv ./
COPY --chown=appuser:appuser --from=builder /app/*.db ./

# Change ownership of the /app directory so the application can write to the sqlite db directory (required for creating journal files)
RUN chown appuser:appuser /app

# Switch to the non-root user
USER appuser

# Expose the application port
EXPOSE 8080

# Run the executable
# ENTRYPOINT ["/bin/sh", "-c", "./${BINARY_NAME}"]
