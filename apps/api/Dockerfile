# ---- Builder stage ----
# golang:1.25-alpine provides Go 1.25.x (latest patch). The go.mod
# requires go 1.25.11; any 1.25.x image satisfies this.
FROM golang:1.25-alpine AS builder
WORKDIR /build

# Cache module downloads.
COPY apps/api/go.mod apps/api/go.sum ./
RUN go mod download

# Copy source and build the binary.
COPY apps/api/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /platform-api ./cmd/server

# ---- Runtime stage ----
FROM alpine:3.20

# ca-certificates: HTTPS calls to Lago / ZITADEL / PSPs
# tzdata: correct timezone handling for usage timestamps
RUN apk add --no-cache ca-certificates tzdata wget && \
    adduser -D -u 1000 platform

USER platform
WORKDIR /app

COPY --from=builder /platform-api .
COPY --from=builder /build/db/migrations ./db/migrations

ENV HTTP_ADDR=:8080
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://localhost:8080/health || exit 1

ENTRYPOINT ["./platform-api"]
