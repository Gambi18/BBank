FROM golang:1.26-alpine AS builder

WORKDIR /usr/src/app

# pre-copy/cache go.mod for pre-downloading dependencies and only redownloading them in subsequent builds if they change
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -v -o /usr/local/bin/app ./...

FROM alpine:3.21
RUN apk --no-cache add ca-certificates
WORKDIR /root/

COPY --from=builder /usr/local/bin/app .

CMD ["./app"]
