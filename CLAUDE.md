# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Mapd is a standalone Go daemon used by forks of comma.ai's openpilot. It reads
OpenStreetMap-derived offline map tiles plus openpilot's live car/vision/GPS
state and publishes speed-limit and curve-speed suggestions back to
openpilot over the same IPC bus openpilot itself uses. See `docs/introduction.md`
for the full v1→v2 rewrite history and rationale, and `docs/integration.md`,
`docs/inputs.md`, `docs/outputs.md`, `docs/settings.md`,
`docs/overriding-internal-defaults.md` for the integration contract with
openpilot forks — those docs are the source of truth for the cereal
message/settings schema and should be kept in sync with any change to inputs
or outputs.

## Commands

- `make build` — compile `build/mapd` (runs `capnp` + `go-deps` first).
- `make capnp` — regenerate `*.capnp.go` files from `.capnp` schemas.
  **Always run this after editing any `.capnp` file; never hand-edit a
  generated `.capnp.go` file.**
- `make format` — run `gofumpt -l -w .`.
- `make deps` — install all toolchain deps (go, capnp compiler + go-capnp
  checkout, gofumpt, docker).
- `make docker` / `make dockerx86` — build the arm64/amd64 docker image used
  to cross-compile the release binary (mapd targets comma's arm64 device).
- `make release` — build via docker and copy the arm64 binary into `build/`.
  CI (`.github/workflows/build.yml`) runs this for PRs and `main`, and plain
  `make build` for other branches/pushes.
- `go test ./...` — run tests (currently under `maps/`).
- `go test ./maps/ -run TestName` — run a single test.
- `go vet ./...` — static checks (no separate lint config in the repo).

Binary usage once built: `./mapd` runs the service loop; `./mapd generate`
(alias `g`) builds offline map tiles from a `.osm.pbf` file; `./mapd
interactive` (alias `i`) opens a bubbletea TUI to configure a running/local
instance (settings, map downloads).

## Architecture

### Process loop (`main.go`, root `main` package)
`main.go` is the entrypoint: it loads settings, then runs a fixed-rate loop
(`ms.LOOP_DELAY`, 50ms) that each tick: publishes the current `State` and
`ExtendedState`, reads inbound settings changes (from openpilot via `mapdIn`
or from the local CLI), reads `carState`/`modelV2`/GPS from openpilot's bus,
and — on a new GPS fix — re-resolves position against the loaded offline map
tile, determines the current/next road ("way"), and recomputes curvature
and target-velocity data for curve-speed control. The root package is split
by concern across files rather than subpackages: `state.go` (aggregate
`State` + `SuggestedSpeed()` arbitration), `car_state.go`, `way.go` (current
way selection), `speed_limit.go`, `map_curve.go`, `vision_curve.go`,
`hazard.go`, `upcoming.go` (generic "upcoming event" lookahead helper used by
both speed-limit and hazard/advisory-speed lookahead), `extended_state.go`,
`math.go`.

`State.SuggestedSpeed()` is the arbitration point: it starts from cruise
speed and lowers it based on whichever of speed-limit control, vision-curve
control, and map-curve control is enabled and currently binding — this is
the logic to touch when changing how competing speed sources interact.

### IPC (`cereal/`)
mapd communicates with openpilot using `gomsgq` (a Go reimplementation of
comma.ai's `msgq`), not the old params-file approach from v1. `cereal/`
wraps this: `publisher.go`/`subscriber.go` are generic pub/sub helpers over
capnp messages, `messageCreators.go`/`readers.go` wire up the concrete
mapd/openpilot message types, `gps.go` is a dedicated GPS subscriber. The
capnp schemas live in `cereal/{car,custom,legacy,log,offline}/*.capnp`
(mostly copies/subset of openpilot's own cereal schemas, per the license
notes in `Readme.md`); each has a generated `*.capnp.go` — regenerate with
`make capnp`, never edit by hand.

### Offline map data (`maps/`)
mapd does not use general-purpose OSM routing files; it uses a custom
compact capnp-encoded tile format (`cereal/offline`) purpose-built for low
memory/CPU use on the comma device. `maps/generate_offline.go` implements
`mapd generate`, which parses a planet `.osm.pbf` extract (`map.osm.pbf`) and
writes tiles under `offline/<lat-band>/...` (the `offline/` directory in
this repo — bands like `24`, `26`, ... `48` — holds generated/sample tiles).
`maps/offline.go` reads a tile back into an `Offline` struct exposing
`Ways()`/`Box()`/`Overlap()` with lazily-memoized derived values (see
`utils.Curry`). `maps/way.go` and `way.go` (root package) implement road
geometry queries (`OnWay`, bearing, distance-to-way) and current-way
selection; `maps/conditional_speed.go` parses OSM `maxspeed:conditional`-style
rules for time-based speed limits.

### Current way selection (`way.go`)
`GetCurrentWay` is a fallback cascade, checked in order: stay on the
existing way if still on it and not at an edge → check the precomputed
`NextWays` → rank all "possible ways" near the GPS fix by a heuristic score
(hierarchy rank, bearing alignment, distance, name/ref match with the
previous way) via `selectBestWayAdvanced` → extend the previous way with a
looser tolerance as a last resort. Each outcome tags a `WaySelectionType`
(`current`/`predicted`/`possible`/`extended`/`fail`) that is published in
`mapdOut` for debugging which strategy is in effect.

### Settings & params (`settings/`, `params/`)
`settings/settings.go` holds the live `Settings` singleton, seeded from
`settings/defaults.json` and `settings/recommended.json` and persisted
through openpilot's persistent-params mechanism (`params/params.go` reads
and writes the param files openpilot itself uses, pointed at the file
locations openpilot expects — this is the interop boundary, not a
mapd-owned config format). `settings/download.go` and
`settings/download_menu.json` describe the map-tile download areas exposed
to forks; forks override behavior by supplying their own JSON files as
described in `docs/overriding-internal-defaults.md`, not by editing these
committed defaults.

### CLI/TUI (`cli/`)
Built on `urfave/cli/v3` for subcommand parsing (`cli.go`) and
`charmbracelet/bubbletea` for the interactive TUI (`main_menu.go`,
`settings.go`, `download.go`, `download_progress.go`, `output.go`), used to
configure a running mapd instance or trigger tile downloads/generation from
the terminal (`./mapd interactive`).

### Math helpers (`math/`)
Shared geometry/GPS primitives used across `maps/` and the root package:
`position.go`/`vector.go`/`line.go`/`box.go` for coordinate math and
point-on-segment/bounding-box tests, `curvature.go`/`jerk_calc.go` for curve
detection and jerk-limited target-velocity calculation, `moving_average.go`
for smoothing (e.g. vision curve speed).

### Utilities (`utils/`)
`Curry[T]` (`curry.go`) is a lazy-memoization wrapper used throughout `maps`
and the root package to avoid recomputing derived values within a single
state tick. `tracked_state.go`'s `Float32Tracker` detects value changes and
remembers the prior value (used for change-triggered logic like next-speed
lookahead). `update_tracker.go` rate-limits periodic sends (e.g.
`ExtendedState`'s 1Hz publish cadence).
