# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git gcc musl-dev sqlite-dev

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o /headscale-v2 ./cmd/server

# Runtime stage
FROM alpine:3.19

WORKDIR /app

RUN apk add --no-cache sqlite-libs ca-certificates tzdata

COPY --from=builder /headscale-v2 /app/headscale-v2
COPY --from=builder /app/configs /app/configs

EXPOSE 8080 9090

ENV TZ=UTC

ENTRYPOINT ["/app/headscale-v2", "-conf", "/app/configs/config.yaml"]