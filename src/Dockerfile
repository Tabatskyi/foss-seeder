# Build Stage
FROM golang:alpine AS builder

WORKDIR /build

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o foss-seeder ./cmd/foss-seeder

# Final Minimal Runtime Stage
FROM alpine:latest

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata && \
    mkdir -p /app/data

COPY --from=builder /build/foss-seeder /app/foss-seeder

EXPOSE 7474

VOLUME ["/app/data"]

ENTRYPOINT ["/app/foss-seeder"]

