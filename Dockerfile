FROM golang:1.25-alpine AS builder

ENV CGO_ENABLED=0
ENV GOOS=linux
ENV GOARCH=amd64

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build --ldflags "-extldflags -static -s -w" -o oss-auto-cert main.go

FROM alpine:latest

LABEL maintainer="nekoimi <nekoimime@gmail.com>"

COPY --from=builder /build/oss-auto-cert   /usr/bin/oss-auto-cert

RUN apk add --no-cache tzdata \
    && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && echo "Asia/Shanghai" > /etc/timezone

WORKDIR /workspace

ENTRYPOINT ["oss-auto-cert"]