# Build stage
# Use a Debian-based builder to match the Ubuntu runner (better for CGO/glibc compatibility)
FROM golang:1.25.0-alpine AS builder

# Use build arguments to change the binary name
ARG BINARY_NAME=recommender-backend

RUN apk add --no-cache gcc libc-dev build-base

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

# Run commands to install necessary packages
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

# Create a non-root user with specific UID and GID
RUN groupadd -g 1000 appgroup && \
    useradd -u 1000 -g appgroup -s /bin/bash -m appuser

# Set the working directory
WORKDIR /app

# Copy the built binary from the builder stage
COPY --from=builder /app/${BINARY_NAME} ./${BINARY_NAME}

# Change ownership of the app directory to the non-root user
RUN chown -R appuser:appgroup /app

# Create directories that the application might need to write to
RUN mkdir -p /app/logs /tmp && \
    chown -R appuser:appgroup /app/logs

# Copy necessary data files (JSON, CSV, DB) that the application might need to read at runtime
COPY --from=builder /app/*.json ./
COPY --from=builder /app/*.csv ./
COPY --from=builder /app/*.db ./

# Switch to the non-root user
USER 1000:1000

# Expose the application port
EXPOSE 8080

# Run the executable
# ENTRYPOINT ["/bin/sh", "-c", "./${BINARY_NAME}"]
