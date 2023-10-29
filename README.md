# Openpilot mapd
Provides openpilot with data from mapd


## Build
### Build capnp files
```bash
capnp compile -I ../go-capnp/std -ogo offline.capnp
```

### Build Release Binary
```bash
CGO_ENABLED=0 go build -ldflags="-extldflags=-static -s -w"
```

NOTE: Must be built for ARM64 to be used on device
