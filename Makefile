build: go-deps capnp
	go build -ldflags="-extldflags=-static -s -w" -o build/mapd

format:
	gofumpt -l -w .

deps: go-deps capnp-deps format-deps

go-deps: go.mod
	go get

capnp-deps:
	go install capnproto.org/go/capnp/v3/capnpc-go@latest
	git clone https://github.com/capnproto/go-capnp ../go-capnp

format-deps:
	go install mvdan.cc/gofumpt@latest


GO_CAPNP_PATH ?= ../go-capnp/std

capnp: cereal/car/car.capnp.go cereal/custom/custom.capnp.go cereal/legacy/legacy.capnp.go cereal/log/log.capnp.go

cereal/car/car.capnp.go: cereal/car/car.capnp
	capnp compile -I $(GO_CAPNP_PATH) -ogo cereal/car/car.capnp

cereal/custom/custom.capnp.go: cereal/custom/custom.capnp
	capnp compile -I $(GO_CAPNP_PATH) -ogo cereal/custom/custom.capnp

cereal/legacy/legacy.capnp.go: cereal/legacy/legacy.capnp
	capnp compile -I $(GO_CAPNP_PATH) -ogo cereal/legacy/legacy.capnp

cereal/log/log.capnp.go: cereal/log/log.capnp
	capnp compile -I $(GO_CAPNP_PATH) -ogo cereal/log/log.capnp

offline.capnp.go: offline.capnp
	capnp compile -I $(GO_CAPNP_PATH) -ogo offline.capnp
