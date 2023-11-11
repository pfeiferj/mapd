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

format-deps:
    FROM +deps
    RUN go install mvdan.cc/gofumpt@latest

format:
    FROM +format-deps
    COPY *.go .
    COPY *.json .
    RUN gofumpt -l -w .
    SAVE ARTIFACT ./*.go AS LOCAL ./

lint-deps:
    FROM +format-deps
    RUN go install honnef.co/go/tools/cmd/staticcheck@latest

lint:
    FROM +lint-deps
    COPY *.go .
    COPY *.json .
    RUN staticcheck -f stylish .
    RUN test -z $(gofumpt -l -d .)


capnp-deps:
    RUN apk add capnproto-dev
    RUN apk add git
    RUN go install capnproto.org/go/capnp/v3/capnpc-go@latest
    RUN git clone https://github.com/capnproto/go-capnp ../go-capnp

compile-capnp:
    FROM +capnp-deps
    COPY *.capnp .
    RUN capnp compile -I ../go-capnp/std -ogo offline.capnp
    SAVE ARTIFACT offline.capnp.go /offline.capnp.go AS LOCAL offline.capnp.go

build-release:
    BUILD --platform=linux/arm64 +build
