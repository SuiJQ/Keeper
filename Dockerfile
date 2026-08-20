# Build stage
FROM golang:1.21-alpine AS builder

RUN apk add --no-cache git make

WORKDIR /build

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w -X main.version=0.1.0-dev" \
    -o /keeper ./cmd/keeper

# Final stage - minimal image
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 -S keeper && \
    adduser -u 1000 -S keeper -G keeper

# Copy binary
COPY --from=builder /keeper /usr/local/bin/keeper

# Set user
USER keeper

# Default command
ENTRYPOINT ["keeper"]
CMD ["help"]
