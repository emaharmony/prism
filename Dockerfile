# Prism Docker Image
# Multi-stage build: build in golang, run in alpine
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Build
COPY . .
RUN CGO_ENABLED=0 go build -o prism ./cmd/prism-cli/

# Runtime
FROM alpine:3.19

RUN apk add --no-cache ca-certificates

COPY --from=builder /app/prism /usr/local/bin/prism

# Create runs directory
RUN mkdir -p /app/runs /app/policies

WORKDIR /app

EXPOSE 8080

ENTRYPOINT ["prism"]
CMD ["--help"]