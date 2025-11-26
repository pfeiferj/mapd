# Roadmap

## v2.0.0 - release
These items are planned to be completed before creating a v2.0.0 release.

- [x] Allow overriding recommended and default settings
- [x] Allow overriding download menu
- [ ] Refactor objects for better code flow
- [x] Improve logic for holding the curve triggered speed state
- [ ] Output additional details about what is triggering the suggested speed (held speed limits, upcoming speed limit activated, etc.)
- [ ] Add accept speed limit and override logic options similar to frogpilot behavior
- [ ] Add accept speed limit input for openpilot fork use
- [ ] Advanced integration docs
- [ ] Github actions for building, linting, etc.
- [ ] Docs on building, linting, etc.
- [x] Implement current speed limit input for fork use (car based, navigation based)


## Planned but unscheduled
- [ ] Extended kalman filter for better gps position
- [ ] Custom path inputs for navigation based curve speed control
- [ ] Record routes with actual curve dynamics data
- [ ] Vision Curve roll detection and correction
- [ ] Current lane outputs (Estimate which lane we are currently in based on position, maps, and openpilot lane data)
- [ ] Vision Curve upcoming path correction (prevent phantom curves at some intersections, help detect leaving current road)
- [ ] Comma prime connection detection (disable data usage on comma prime)
- [ ] Live download maps
- [ ] Download maps within x-distance of current location
- [ ] Flag locations driver overrode speed limit output
- [ ] Limited support for conditional speed limits (only simple conditions that are parseable and time based)
- [ ] Additional map files for stop sign/stop light locations, possibly some other node based things
- [ ] Web server for viewing map data
- [ ] Map override editor (for things like setting preferred speed on roads where speed limit doesn't make sense)
- [ ] Zone data (city, county, state, nation)
