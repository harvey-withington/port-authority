# usb.ids

Public USB vendor / product ID database used by `core/kb`.

- Source: http://www.linux-usb.org/usb.ids (mirror: https://raw.githubusercontent.com/usbids/usb.ids/master/usb.ids)
- Downloaded: 2026-09-06
- Upstream version: 2026.06.26 (see the `# Version:` line in the file header)
- License: the usb.ids database is dual-licensed, GPL-2.0-or-later OR BSD-3-Clause.
  The file as served by linux-usb.org carries no license block in its header,
  only the maintainer notice and version stamp; that header is kept verbatim.

Only the vendor and product sections are parsed. Class, HID, language and
other trailing sections are embedded but ignored. Do not hand-edit the file;
re-download it and update the date above.

# docks.json and devices.json

Vendored from https://github.com/harvey-withington/usb-device-kb (MIT),
the community knowledge base. Do not edit here: change them in that repo
(cloned as `usb-device-kb/` at the project root), then run
`go run ./tools/syncdata`, which copies them in and records the source
commit in `PROVENANCE`. `overrides.json` is the app's own and stays here.
