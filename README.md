# Port Authority

*Working title.* A USB / Thunderbolt insight app for Windows (macOS-ready architecture). It does not just show your USB topology, it explains it: bandwidth budgets, dock and display alt-mode tax, bottleneck detection, plain-language fixes, and live truth-testing of devices.

The full spec is in [docs/port-authority-handoff.md](docs/port-authority-handoff.md).

## Status

Phase 0 (spike) is complete. The Windows hub IOCTL walk produces a JSON topology dump, and live per-device throughput via ETW is proven. See [docs/decisions/0001-live-throughput-and-elevation.md](docs/decisions/0001-live-throughput-and-elevation.md).

```
go run ./cmd/pactl tree            # human-readable tree of what is connected
go run ./cmd/pactl snapshot --pretty
go run ./cmd/pactl caps            # what this provider can measure
go run ./cmd/pactl tree --fixture testdata/fixtures/<file>.json   # replay without hardware
go run ./cmd/pactl etw-spike --seconds 15   # live USB transfer events per device (admin or Performance Log Users)
go run ./cmd/pactl insights        # plain-language findings, e.g. "SSD could be 4x faster on a different port"
go run ./cmd/pactl serve           # local REST API + WebSocket stream on 127.0.0.1:7911 (see core/api)
go run ./cmd/pactl live            # hotplug events and per-device throughput in the terminal
```

The API is live: `pactl serve` watches for hotplug, streams per-device throughput, and pushes
`topology_changed`, `insight_added`, `insight_resolved` and `throughput_sample` events over
`ws://127.0.0.1:7911/api/v1/stream`. Poll `/api/v1/throughput` if you would rather not hold a socket.

The first real finding, from the fixture captured on DEVIANT:

```
[WARNING] Corsair EX400U could be up to 4x faster on a different port
  Corsair EX400U supports 40 Gbps (USB4 / Thunderbolt), but it is plugged into the
  "USB-C Data" port on the front of the CalDigit TS4, which is a 10 Gbps port.
  What to do: Move Corsair EX400U to one of the 2 "Thunderbolt 4" ports on the rear of the CalDigit TS4.
```

## Desktop app

The Wails app at the repo root runs the collector and the API in-process and shows the Svelte UI. The UI is a plain HTTP + WebSocket consumer of `127.0.0.1:7911` (or a free loopback port if that one is taken), so a third-party front-end gets exactly the same data.

```
wails dev      # hot-reloading development window (frontend on http://localhost:5173)
wails build    # build/bin/port-authority.exe
cd frontend && npm run check && npm run test:run   # type-check and unit tests
```

The frontend lives in [frontend/](frontend/); its shared components and colour tokens are documented in [frontend/UI-CONVENTIONS.md](frontend/UI-CONVENTIONS.md). Device classes and link speeds each have their own colour family, and link health (fine versus downgraded) is a separate ring so "how fast" and "is it right" never share a signal.

## Layout

| Path | Role |
|------|------|
| `core/model` | Platform-neutral domain model. Every JSON shape here is public API. |
| `core/provider` | The `Provider` interface every OS implements. |
| `core/provider/mock` | Replays a saved snapshot. Drives the app without hardware. |
| `core/kb` | Knowledge base: usb.ids with `overrides.json`, `docks.json` (port maps with printed labels), `devices.json` (capabilities devices do not report). |
| `core/enrich` | Annotates a topology with knowledge-base data (vendor and product names). |
| `core/insight` | Rule engine producing plain-language findings with evidence and confidence. |
| `core/api` | Local REST API (`/api/v1/topology`, `/insights`, `/devices/{id}`, `/capabilities`). |
| `platform/win` | Windows provider: SetupAPI + hub IOCTLs, and the ETW consumer. |
| `platform` | Picks the native provider by build tag. The only package that imports `platform/*`. |
| `cmd/pactl` | Collector CLI. Later the headless API service. |
| `testdata/fixtures` | Captured real-world topologies used as regression fixtures. |
| `tools/scrubfixture` | Replaces device serial numbers in a fixture with placeholders before it is shared. |

## Contributing a fixture

Interesting setups (a dock that misbehaves, a device that negotiates the wrong speed) are most useful as fixtures. Capture one, scrub the serial numbers, and send it:

```
pactl snapshot --pretty > my-machine.json
go run ./tools/scrubfixture my-machine.json
```

The scrubber rewrites `serial_number` fields and the serial segment of instance IDs in place, keeping everything else byte-for-byte, so the fixture still replays exactly.

Rules that keep the macOS port a "write a DarwinProvider" job:

- `core/*` never imports anything under `platform/`.
- Consumers check `ProviderCaps`, never `runtime.GOOS`.
- The provider observes and measures. It never mutates device configuration.

## Phase 0 outcomes

1. ETW from Go: go. Hand-written advapi32/tdh consumer, no cgo, no GPL dependency.
2. Elevation: collector runs unelevated; live throughput needs a one-time addition to Performance Log Users.
3. Device identity: PnP instance ID, shared by the IOCTL walk and the ETW rundown events.
4. Alt-mode: the USBHUB3 rundown reports DisplayPort, Thunderbolt 3 and USB4 capability per port, so it is partly reported rather than inferred.

Still open: the final name (Port Authority is the working title).
