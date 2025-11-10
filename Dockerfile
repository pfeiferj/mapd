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
RUN PATH=$PATH:/home/root/go/bin make

RUN mv build/mapd ./mapd
RUN mv scripts/* ./

CMD ["./docker_entry.sh"]
