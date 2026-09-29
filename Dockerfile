FROM golang:1.26-alpine AS builder
RUN apk add --no-cache ca-certificates
WORKDIR /src

COPY . .
RUN go mod vendor
RUN CGO_ENABLED=0 go build -mod=vendor -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -H -u 10001 appuser
WORKDIR /app

COPY --from=builder /out/server /app/server
COPY configs/config.docker.yaml /app/configs/config.yaml

RUN mkdir -p /app/storage/uploads \
    && chown -R appuser:appuser /app

USER appuser
EXPOSE 8090

ENV SERVER_ADDR=:8090
ENTRYPOINT ["/app/server", "-config", "/app/configs/config.yaml"]
