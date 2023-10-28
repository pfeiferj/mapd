# Openpilot mapd
Provides openpilot with data from mapd

# Build Release Binary
```
CGO_ENABLED=0 go build -ldflags="-extldflags=-static -s -w"
```

NOTE: Must be built for ARM64 to be used on device
