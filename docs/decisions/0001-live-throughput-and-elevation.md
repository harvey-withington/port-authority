# ADR 0001: Live throughput source and elevation model

Date: 2026-09-06. Status: accepted (Phase 0 spike outcome).

## Context

The spec's riskiest bet was consuming Event Tracing for Windows (ETW) from Go to get live per-device USB throughput, with three named fallbacks if it proved painful. It also left the elevation model open: run the collector elevated always, or elevate only for ETW and deep tests.

## What the spike found

Verified on DEVIANT (Windows 11 Pro 26200, Go 1.24) with `pactl etw-spike`:

- **ETW from pure Go works.** A real-time session over `Microsoft-Windows-USB-USBHUB3`, `USBXHCI` and `UCX` with a hand-written advapi32/tdh binding (no cgo, no third-party library) delivered 13,000 events in 12 seconds with zero lost.
- **Transfers are attributable to devices.** Keyword `HeadersBusTrace` (0x40) on UCX emits `URB_FUNCTION_BULK_OR_INTERRUPT_TRANSFER` and `URB_FUNCTION_ISOCH_TRANSFER` Start/Stop pairs carrying `fid_UsbDevice` (kernel object pointer), `fid_PipeHandle` (endpoint) and a per-function URB struct with the transfer length.
- **The device pointer maps to our topology IDs.** Keyword `Rundown` (0x8000) makes every provider dump its state at enable time. USBHUB3's device rundown pairs `fid_UsbDevice` with `fid_DeviceInterfacePath` (e.g. `\??\USB#VID_8087&PID_0B40#5&270c603&0&4#{...}`), which converts directly to the PnP instance ID that `Device.ID` already uses. It also carries `fid_PortPath` and `fid_HubDevice` for tree placement.
- **Bonus: alt-mode and Type-C data.** The hub rundown per port includes `fid_PortConnectorType`, `fid_DpAltModeSupported`, `fid_Tbt3AltModeSupported`, `fid_Usb4Supported`, `fid_UcmConnectorId` and `fid_RetimerCount`. This answers open question 3: some alt-mode capability is reported, not inferred.
- **Elevation is not strictly required.** Starting a trace session needs either administrator rights or membership of the built-in `Performance Log Users` group. DEVIANT's account is in that group, and the spike ran unelevated. The hub IOCTL walk needs neither.

## Decision

1. **Live throughput comes from ETW**, keywords `Default|HeadersBusTrace|Rundown`, providers USBHUB3 + USBXHCI + UCX (USBPORT + USBHUB added later for EHCI-only hosts). Bytes are counted on transfer completion (`Stop` opcode, `fid_IRP_NtStatus == 0`) and attributed by `fid_UsbDevice`. The disk-counter fallback is not needed.
2. **The collector runs unelevated by default.** Topology, hotplug, insights and the quick speed test all work as a normal user.
3. **Live throughput is an opt-in feature that needs a one-time privileged setup**: the installer (or a "Enable live throughput" button that triggers one UAC prompt) adds the user to `Performance Log Users`. After that, no elevation is ever needed again, and the collector can run as a plain user process or a per-user service. This beats a permanently elevated collector (attack surface, UAC on every launch) and beats an elevated helper process (IPC complexity, still a UAC prompt per session).
4. **The deep capacity test needs no elevation** since it writes ordinary files to free space on a user-confirmed removable volume. Its guardrails are about confirmation, not privilege.

## Consequences

- `ProviderCaps.Throughput` is reported true only when a session can actually be started; the UI shows a "set up live throughput" call to action otherwise.
- The ETW consumer in `platform/win/etw.go` stays hand-written and license-clean. Struct layouts are pinned to SDK 10.0.26100 `tdh.h`; note `TRACE_EVENT_INFO` has a `BinaryXMLSize` field that older references omit.
- Device identity across the two data sources is the PnP instance ID. Open question 2 (stable identity across replug) is narrowed to: instance IDs are stable for devices with serial numbers, and port-path based for those without.
