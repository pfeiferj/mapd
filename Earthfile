VERSION 0.7
FROM golang:1.21-alpine3.18
WORKDIR /mapd

deps:
    COPY go.mod go.sum ./
    RUN go mod download
    SAVE ARTIFACT go.mod AS LOCAL go.mod
    SAVE ARTIFACT go.sum AS LOCAL go.sum

build:
    FROM +deps
    COPY *.go .
    COPY *.json .
    RUN CGO_ENABLED=0 go build -ldflags="-extldflags=-static -s -w" -o build/mapd
    SAVE ARTIFACT build/mapd /mapd AS LOCAL build/mapd

all:
  BUILD \
    --platform=linux/arm64 \
    +build
