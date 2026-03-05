# Build stage
# Use a Debian-based builder to match the Ubuntu/glibc runner.
# Using Alpine builder with Ubuntu runner causes 'No such file or directory' errors due to musl vs glibc links.
FROM golang:1.25-bookworm AS builder

# Default to "recommender-backend" but allows overrides. 
# Explicitly set to match what the user is trying to run if needed.
ARG BINARY_NAME=recommender-backend

# Install build dependencies for CGO (sqlite3)
RUN apt-get update && apt-get install -y gcc libc6-dev make

WORKDIR /app

# Copy module files and dependencies
COPY A1CE_recommender_code/backend/go.mod ./
# RUN go mod download # Optional: more efficient caching

# Copy the source code
COPY A1CE_recommender_code/backend/ .

RUN go get a1ce-recommender-backend

# Build the Go application with CGO enabled
RUN CGO_ENABLED=1 GOOS=linux go build -o ${BINARY_NAME} .

# Final stage
FROM ubuntu:22.04

# Re-declare ARG and set as ENV so it persists at runtime
ARG BINARY_NAME=recommender-backend
ENV BINARY_NAME=${BINARY_NAME}

# Install runtime dependencies
RUN apt-get update && apt-get install -y ca-certificates sqlite3 && rm -rf /var/lib/apt/lists/*

# Create a non-root group and user
RUN groupadd -g 1000 appgroup && \
    useradd -u 1000 -g appgroup -s /bin/bash -m appuser

WORKDIR /app

# Create necessary directories first
RUN mkdir -p /app/logs /tmp && chown -R appuser:appgroup /app

# Copy binary and data files with correct ownership
COPY --chown=appuser:appgroup --from=builder /app/${BINARY_NAME} ./${BINARY_NAME}
COPY --chown=appuser:appgroup --from=builder /app/*.json ./
COPY --chown=appuser:appgroup --from=builder /app/*.csv ./
COPY --chown=appuser:appgroup --from=builder /app/*.db ./

# Ensure the binary is executable (just as a safeguard)
RUN chmod +x ./${BINARY_NAME}

# Switch to the non-root user
USER 1000:1000

# Expose the application port
EXPOSE 8080

# Run the executable
ENTRYPOINT ["/bin/sh", "-c", "./${BINARY_NAME}"]
