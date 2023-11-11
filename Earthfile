VERSION 0.7
FROM golang:1.21-alpine3.18
WORKDIR /mapd

deps:
    COPY go.mod go.sum ./
    RUN go mod download
    RUN go install honnef.co/go/tools/cmd/staticcheck@latest
    RUN go install mvdan.cc/gofumpt@latest
    SAVE ARTIFACT go.mod AS LOCAL go.mod
    SAVE ARTIFACT go.sum AS LOCAL go.sum

build:
    FROM +deps
    COPY *.go .
    COPY *.json .
    RUN CGO_ENABLED=0 go build -ldflags="-extldflags=-static -s -w" -o build/mapd
    SAVE ARTIFACT build/mapd /mapd AS LOCAL build/mapd

lint:
    FROM +deps
    COPY *.go .
    COPY *.json .
    RUN staticcheck -f stylish .
    RUN test -z $(gofumpt -l -d .)

format:
    FROM +deps
    COPY *.go .
    COPY *.json .
    RUN gofumpt -l -w .
    SAVE ARTIFACT ./*.go AS LOCAL ./

build-release:
  BUILD --platform=linux/arm64 +build
