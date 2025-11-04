FROM ubuntu:24.04
WORKDIR /usr/local/app

# Install the application dependencies
COPY scripts/install-ubuntu-deps.sh ./
RUN ./install-ubuntu-deps.sh

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . /usr/local/app

RUN make capnp-deps
RUN make

RUN useradd app
USER app

CMD ["/usr/local/app/build/mapd"]
