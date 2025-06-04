# Multi-stage build: Build stage
FROM golang:1.23-alpine AS builder

# Install git for go mod download (some modules may need it)
RUN apk add --no-cache git

# Set build arguments for cross-compilation
ARG TARGETOS=linux
ARG TARGETARCH=amd64

# Set working directory
WORKDIR /workspace

# Copy go mod files first for better layer caching
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download && go mod verify

# Copy source code
COPY cmd/ cmd/
COPY api/ api/
COPY internal/ internal/

# Build the binary
# CGO_ENABLED=0 ensures static binary with no external dependencies
# -ldflags='-w -s' strips debug information to reduce binary size
# -trimpath removes file system paths from the binary
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -a -ldflags='-w -s' -trimpath -o manager cmd/main.go

# Verify the binary was built
RUN ls -la manager

# Final stage: Minimal runtime image
FROM gcr.io/distroless/static:nonroot

# Set working directory
WORKDIR /

# Copy the binary from builder stage
COPY --from=builder /workspace/manager .

# Use non-root user for security
USER 65532:65532

# Set the entrypoint
ENTRYPOINT ["/manager"]