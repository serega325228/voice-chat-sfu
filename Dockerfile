FROM golang:1.26.2-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/sfu ./cmd/sfu

FROM alpine:3.22

WORKDIR /app

RUN apk add --no-cache ca-certificates

COPY --from=builder /out/sfu /app/sfu
COPY config /app/config

ENV CONFIG_PATH=/app/config/docker.yaml

EXPOSE 8081

ENTRYPOINT ["/app/sfu"]
