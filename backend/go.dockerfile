FROM golang:1.26-alpine AS builder

WORKDIR /usr/src/app

# Pre-copy go.mod/go.sum so dependencies are only re-downloaded when they change.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Build the two commands explicitly. `./...` would also try to build test-only
# packages and produces unpredictable binary names once there is more than one main.
RUN go build -v -o /usr/local/bin/api     ./cmd/api \
 && go build -v -o /usr/local/bin/migrate ./cmd/migrate

FROM alpine:3.21
RUN apk --no-cache add ca-certificates
WORKDIR /app

# Run as a non-root user: the API has no reason to own its own filesystem.
RUN adduser -D -u 10001 bbank
COPY --from=builder /usr/local/bin/api     /app/api
COPY --from=builder /usr/local/bin/migrate /app/migrate
USER bbank

EXPOSE 8000
CMD ["/app/api"]
