# Multi-stage build for Keeper CI environment
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git make gcc musl-dev bubblewrap strace libseccomp-dev

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=0.1.0-dev" -o bin/keeper ./cmd/keeper

# Final image for running tests
FROM alpine:latest

RUN apk add --no-cache bubblewrap strace libseccomp-dev iptables iproute2

COPY --from=builder /app/bin/keeper /usr/local/bin/keeper
COPY --from=builder /app/internal/container/testdata /keeper-testdata

WORKDIR /workspace

CMD ["keeper", "--help"]
