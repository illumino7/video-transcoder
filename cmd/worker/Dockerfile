# ── Build Stage ──────────────────────────────────────
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/worker ./cmd/worker

# ── Runtime Stage ────────────────────────────────────
FROM alpine:3.21

RUN apk add --no-cache ca-certificates docker-cli bash

COPY --from=builder /bin/worker /bin/worker
COPY scripts/ /scripts/

EXPOSE 8080

ENTRYPOINT ["/bin/worker"]
