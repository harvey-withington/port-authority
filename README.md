# Port Authority

*Working title.* A USB / Thunderbolt insight app for Windows (macOS-ready architecture). It does not just show your USB topology, it explains it: bandwidth budgets, dock and display alt-mode tax, bottleneck detection, plain-language fixes, and live truth-testing of devices.

The full spec is in [docs/port-authority-handoff.md](docs/port-authority-handoff.md).

## Status

**Beta.** The Windows collector, the insight engine, the local API and the desktop UI all work on real hardware. What is not settled is how much of a machine the data can actually describe — see [Known limitations](#known-limitations) before filing a bug, because the most surprising results are usually Windows telling us less than you would expect.

Phase 0 (spike) is complete: the hub IOCTL walk produces a JSON topology dump, and live per-device throughput via ETW is proven. See [docs/decisions/0001-live-throughput-and-elevation.md](docs/decisions/0001-live-throughput-and-elevation.md).

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

The hero view is the topology diagram: the computer on the left, everything plugged into it fanning out to the right. Line thickness is link capacity, colour (plus a dash pattern) is link health, and moving dashes show live transfers, with a hub's uplink carrying the sum of everything behind it. A device named by a new finding pulses. The tree view is one click away and shares the same collapse state.

The diagram has two readings. **Physical** draws the boxes on your desk: the computer as one machine, a dock as one dock, one cable between them, with the sockets down each box's edge. **Logical** draws what Windows reports: every controller, every hub inside a dock, every device. A dock is four or five hubs across two controllers to Windows and one object to you, so the two views disagree on purpose.

## Known limitations

The app reports what Windows tells it, and Windows does not always know. These are limits of the available data rather than bugs, so they are worth recognising before filing one.

- **An occupied USB-C socket can read as empty.** A USB-PD charger is not a USB data device, so it never reaches the USB tree at all. A USB4 drive tunnels PCIe and so appears only as a Thunderbolt router, with nothing in a snapshot tying it to the connector it arrived on. Both need connector-level (UCSI) data the collector does not gather yet.
- **A USB4 dock can appear twice**, once as a box of hubs and once as a Thunderbolt router. A router identifies itself in a different namespace from its hubs, and only the knowledge base can bridge the two; adding the router id to `docks.json` fixes it for that model.
- **Socket counts can be too high.** Windows reports one port per USB 2 / USB 3 half of a physical socket and only sometimes says which two halves pair up. Where it does not say, both are counted.
- **A hub with two chips inside can draw as two boxes.** Only the firmware knows which of its ports reach the outside world, and cheap hubs mark every port as user-connectable. Recognised docks are folded using `docks.json`; anything else is left as reported rather than guessed at.

The rule throughout is that the provider observes and never invents: where the data cannot distinguish two situations, the app shows what it measured instead of picking the prettier answer.

## Security

The API binds to loopback only, and browsers are held to this app's own origins, so a web page you happen to have open cannot read your device list over HTTP or the WebSocket stream. There is no authentication yet, so any program running as you on this machine can read it; token auth is still to come. A snapshot names every attached device and, unless you scrub it, their serial numbers — worth remembering before sharing one.

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
| `docs` | Public: the spec, decision records, anything a contributor should read. Committed here. |
| `plan` | Private: TODOs and planning notes. Its own separate private git repository, never pushed with this one. |

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
