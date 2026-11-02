FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

ENV CGO_ENABLED=0
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags "-s -w -X github.com/nekoimi/oss-auto-cert/internal/cli.Version=$VERSION" -o oss-auto-cert ./cmd/oss-auto-cert

FROM alpine:latest

LABEL maintainer="nekoimi <nekoimime@gmail.com>"

COPY --from=builder /build/oss-auto-cert   /usr/bin/oss-auto-cert

RUN apk add --no-cache ca-certificates tzdata \
    && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && echo "Asia/Shanghai" > /etc/timezone

RUN mkdir -p /etc/oss-auto-cert /var/lib/oss-auto-cert

WORKDIR /workspace

ENTRYPOINT ["oss-auto-cert"]
