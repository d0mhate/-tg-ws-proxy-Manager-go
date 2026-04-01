FROM golang:alpine AS builder

LABEL maintainer="d0mhate"

RUN apk add --update upx

WORKDIR /build

# сначала копируем go.mod и go.sum для кэширования зависимостей
COPY go.* ./
# COPY vendor ./vendor
RUN go mod download

COPY . .

ENV GO111MODULE=on \
    CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

RUN go build -mod=vendor -ldflags="-s -w" -o /app/tg-ws-proxy cmd/tg-ws-proxy/main.go

RUN upx --lzma /app/tg-ws-proxy

#########################

# scratch
FROM alpine:latest

COPY --from=builder /app/tg-ws-proxy /bin/tg-ws-proxy

ENTRYPOINT ["/bin/tg-ws-proxy"]

CMD ["--host", "0.0.0.0"]
