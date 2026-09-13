# 0002. Knowledge base layers, the user's own docks, and where USB4 link data comes from

Date: 2026-09-12. Status: accepted.

## Context

A dock only draws as one box when the knowledge base lists its internal
hubs, and its USB4 router only folds into that box when the knowledge base
records the vendor and model strings the router announces. The shipped
data held one dock. Every other dock in the world drew as a chain of hub
boxes with a router floating beside the laptop, and nothing the user could
do in the app would change that.

Two further gaps: the diagram assumed every USB4 link ran at 40 Gbps, and no
public dataset maps docks to their hub ids.

## Decision

### Three layers, one catalog

`core/kb` builds one immutable, atomically swapped catalog from three
layers, later layers winning by dock id and by hub id:

| Layer | Source | Where | Who writes it |
|---|---|---|---|
| shipped | `SourceShipped` | embedded `data/docks.json`, `data/devices.json` | maintainers, via PRs |
| shared | `SourceShared` | the `latest` release of github.com/harvey-withington/usb-device-kb, fetched by `core/community` with If-None-Match and cached under `<kb dir>/shared/` | the community, through that repo's issue form and review |
| local | `SourceLocal` | `<UserConfigDir>/PortAuthority/kb/docks.json` | the user, through the app |

Every layer uses the same `DockEntry` schema, so an entry can move between
them unchanged. The data lives in its own public MIT repository,
usb-device-kb, cloned into the project as an ignored subfolder and vendored
into `core/kb/data` by `tools/syncdata`, which records the source commit.
Sharing sends nothing from the app: the "share" button on one of the
user's docks opens the repository's issue form in the browser with the
entry filled in, a workflow there validates it and opens a pull request, a
maintainer merges, and a second workflow republishes both files as assets
of a rolling `latest` release. `core/community` fetches those at start,
after applying its cache, and re-reads the machine when they changed.
GitHub is the whole infrastructure.

Each dock carries `Source`, `Verified` and `Notes`; the enclosure stamped on
a hub carries the source too, so the UI can offer "forget this dock" only
on the user's own entries. A local entry with a shipped dock's hubs takes
those hubs over, which is how a user corrects a wrong grouping without
waiting for a release.

### The user's own docks

`GET/POST /api/v1/kb/docks` and `DELETE /api/v1/kb/docks/{id}` are the only
writes the API accepts. A write re-reads the machine and broadcasts a
`topology_changed` carrying a `resnapshot` event, so every consumer redraws.
The rule `dock-not-recognised` points at a chain of loose hubs (or a router
no dock claimed) and the finding card offers "Set up this dock…", which
opens a dialog listing every loose hub, pre-ticked from the finding, with a
name and the router suggested. Saving writes the local layer.

`pactl` loads the local layer for every command; `pactl serve --kb-dir ""`
runs read-only, which the tests and the fixture screenshots use.

### USB4 link speed and router strings

Windows' inbox USB4 driver stamps every router devnode with an undocumented
property set, GUID `5DF7E321-1C1B-4CE2-B4FA-55F4A5BC2CB6`, found by listing
every property on a router (`tools/usb4props`). The values match the USB4
router and lane adapter configuration spaces, whose encodings Linux names
in `drivers/thunderbolt/tb_regs.h`: pid 9 and 10 are the DROM vendor and
model strings, 13/14 the product's USB vid:pid, 16/17 its USB4 vid:pid, 18
the revision, and 20/21 the negotiated lane speed (0x8 Gen 2, 0x4 Gen 3,
0x2 Gen 4) and width (1 or 2 lanes). The provider reads them into
`USB4Router.Vendor/Model/USBVendorID/USBProductID/LinkGen/LinkLanes/
NegotiatedLink`.

Consequences:

- a dock's router is matched to its knowledge base entry by the DROM
  strings directly, with the driver name parsed only as a fallback for old
  fixtures;
- the diagram draws USB4 cables at their negotiated speed and judges a
  dock's cable against the dock's uplink maximum;
- rule `usb4-link-below-max` reports a dock or device linked slower than
  the knowledge base says it can go. It found DEVIANT's TS4 on a 20 Gbps
  link the day it was written.

The property set is undocumented, so a driver update could move it. Every
field is optional and the app degrades to the old behaviour (assumed
40 Gbps, name parsing) when the set is absent.

## Data sources considered

There is no public dataset of dock hub ids. What exists, and what was done
with it:

- **fwupd quirk files** (LGPL-2.1+): `plugins/dell-dock`, `lenovo-dock`,
  `genesys`, `parade-usbhub`, `intel-usb4` list the USB ids each dock's
  firmware updater claims. Facts, not expression; the Dell WD19/WD22 and
  Lenovo ThinkPad USB4 Dock entries were seeded from them with the source
  named in `verified`. Their port lists and router strings are still blank.
- **Published teardowns with `lsusb` output** (watchmysys.com's HP
  Thunderbolt Dock G4): seeded the same way.
- **usb.ids** (already shipped): names hub products but rarely says which
  dock they live in.
- **Dan S. Charlton's USB4/TB4 dock tables** (120+ docks): the best public
  port lists, but a blog under copyright with no dataset; use for manual
  verification, not scraping.
- **thunderbolttechnology.net/products** and the Framework community dock
  megathread: product names and anecdotes, no ids.
- Paid APIs: none found that carry USB ids per dock. The USB-IF product
  database lists certified products without internal hub ids.

The realistic supply of complete, verified entries is users of this app
sharing what the dialog produced, which is what the shared layer is for.

## Alternatives rejected

- Matching a router to a dock by fuzzy comparison of its DROM model against
  hub product strings ("TS4" appears in "TS4 USB3.2 Gen2 HUB"). It would
  work for the TS4 and silently mis-fold the next dock; grouping rules that
  can absorb a box stay explicit (see `plan/` notes on the internal-port
  fold).
- Guessing a dock from hub nesting alone. A chain of hubs is what a dock
  looks like, but also what a hub plugged into a hub looks like; the rule
  only asks, it never folds.
