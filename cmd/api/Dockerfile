# ── Build Stage ──────────────────────────────────────
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/api ./cmd/api

# ── Runtime Stage ────────────────────────────────────
FROM alpine:3.21

RUN apk add --no-cache ca-certificates
COPY --from=builder /bin/api /bin/api

EXPOSE 3030

ENTRYPOINT ["/bin/api"]
