FROM golang:1.22-alpine as builder

ENV CGO_ENABLED=0
ENV GOOS=linux
ENV GOARCH=amd64

WORKDIR /build
COPY . .
RUN go install
RUN go build --ldflags "-extldflags -static -s -w" -o oss-auto-cert main.go

FROM alpine:latest

LABEL maintainer="nekoimi <nekoimime@gmail.com>"

COPY --from=builder /build/oss-auto-cert   /usr/bin/oss-auto-cert

RUN apk add tzdata \
    && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && echo "Asia/Shanghai" > /etc/timezone \
    && apk del tzdata

WORKDIR /workspace

ENTRYPOINT ["oss-auto-cert"]