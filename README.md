# Openpilot mapd
Provides openpilot with data from mapd

## Using
### Integrating With Openpilot
Each release will have a pre-compiled static binary attached for use with
openpilot on a comma device. Without any additional code the binary will not run
or change openpilot behavior. A reference implementation for managing downloads
of the binary and using data output from this daemon is located in
[pfeifer-openpilot-patches](https://github.com/pfeiferj/openpilot/tree/pfeifer-openpilot-patches/mapd).

### mapd inputs
Inputs are described in [docs/inputs.md](./docs/inputs.md).

### mapd outputs
Outputs are described in [docs/outputs.md](./docs/outputs.md).

## Build
This project uses [earthly](https://github.com/earthly/earthly/) for its build
system. To install earthly follow the instructions at the
[get earthly page](https://earthly.dev/get-earthly)

### Format Code
```bash
earthly +format
```

### Lint
```bash
earthly +lint
```

### Test
```bash
earthly +test
```

### Update Snapshot Tests
```bash
earthly +update-snapshots
```

### Build capnp Files
```bash
earthly +compile-capnp
```

### Build Release Binary
NOTE: This will be built for ARM64 to be used on a comma device and my not work
on your computer
```bash
earthly +build-release
```

### Build Binary
NOTE: This will be built for your current archetecture and may not work on a
comma device
```bash
earthly +build
```
