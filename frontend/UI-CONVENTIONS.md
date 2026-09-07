# Port Authority UI conventions

This file is a contract: when you add or change a shared component, token or
pattern, update it in the same change.

## Ground rules

- Svelte 5 runes only (`$state`, `$derived`, `$props`, `$effect`); typed props via an
  `interface Props`. Strict TypeScript, never `any`.
- Every user-visible string goes through `t('key', params)` from
  `src/lib/i18n.svelte.ts`; keys live in `src/lib/locales/en.json`.
- Colours and sizes are CSS custom properties in `src/app.css`; components never
  hardcode a colour. Dark is the default palette, light is provided under both
  `@media (prefers-color-scheme: light)` and `[data-theme="light"]`.
- No component over ~300 lines. Keyed `{#each}` blocks use stable ids (device id,
  insight key, timeline entry id), never the index.
- Errors the user should know about are rendered (`ConnectionBanner`,
  `StartingScreen`), never only logged. No `alert()` / `confirm()`.
- Colour is never the only signal: class colours ride on an icon, speed colours
  carry a text label, health is a ring plus a tooltip, severity has an icon.
- Shared DOM behaviours are actions in `src/lib/actions.ts` (`flash`, `clickOutside`,
  `observeWidth`). Components never construct `ResizeObserver` themselves (jsdom has none).

## Data flow

`App.svelte` discovers the API (`lib/discover.ts`: Wails `Status()` polling or
`?api=` / loopback default), then creates one `LiveStore` (`lib/live.svelte.ts`).
The store owns connection state, the snapshot, insights, throughput and the
timeline; components receive plain values as props. Pure logic lives in `lib/`
and is unit-tested: `format`, `link`, `ids`, `topology`, `ports`, `insights`,
`timeline`, `throughput`, `colors`, `graph`, `discover`, `api/client`.

`lib/ports.ts` turns a hub's port list into the physical sockets a person can
see. `socketKind(port)` is `usb-c` / `usb-a` / `internal` / `unknown` (no
connector data means unknown, and a port nobody can reach is internal);
`isUsb4(port)` is true for the USB4 port maximums. Windows reports one hub
port per logical port, so one chassis socket appears twice - once on the USB 3
hub and once on its USB 2 companion. `visiblePorts(hub, topology, index?)`
folds the pair back into one: an empty port is dropped when its companion
resolves and is faster (or equal and lower-numbered), a port with a device is
never dropped, and an unresolvable companion keeps both halves. Companions are
resolved through `indexHubPaths` (normalised hub `device_path` -> hub), built
once per layout; `normalizeHubPath` strips a `\\?\` / `\\.\` prefix and
upper-cases.

Small pieces of UI state that must outlive a render are rune modules under
`lib/*.svelte.ts`: `expanded` (collapsed hubs), `focus` (show-me requests),
`view` (diagram or tree, persisted as `pa-connected-view`).

Device ids are compared case-insensitively (`lib/ids.ts`); the throughput map and
device index are keyed by `normalizeId(id)`.

## Tokens (src/app.css)

| Family | Tokens | Use |
|---|---|---|
| Surface | `--bg-base`, `--bg-surface`, `--bg-elevated`, `--bg-subtle`, `--bg-subtle-hover`, `--bg-overlay` | page, panes, cards, hover |
| Border | `--border`, `--border-muted`, `--border-hover` | |
| Text | `--text-primary`, `--text-strong`, `--text-body`, `--text-secondary`, `--text-muted`, `--text-faint` | |
| Accent | `--accent`, `--accent-hover`, `--accent-light`, `--accent-glow-1..3` | brand, flash |
| Semantic | `--success`, `--warning`, `--danger`, `--info` and `--*-bg` / `--*-text` | banners, chips |
| On-colour | `--on-color`, `--on-color-dark`, `--on-color-muted` | text on solid fills |
| Severity | `--severity-info|warning|critical` (+ `-bg`) | insight stripe, flagged devices |
| Connection | `--conn-live|polling|offline|connecting` | `ConnectionBadge` dot |
| Link health | `--link-good|slow|idle` | `LinkBadge` ring |
| Device class | `--class-storage|video|audio|hid|network|display|wireless|printer|serial|imaging|vendor|hub|composite|unknown` | icon, left edge, device names |
| Speed ramp | `--speed-low|full|high|ss5|ss10|ss20|usb4-20|usb4-40|usb4-80` (+ `--speed-none|unknown`), each with `-ink` | badge fill, meter fill, USB4 section |
| Meter | `--meter-track`, `--meter-height` | |
| Type / spacing | `--font-sans`, `--font-mono`, `--font-size-xs..xl`, `--space-1..6`, `--radius-sm|md|lg`, `--tree-indent`, `--stripe-width`, `--header-height` | |
| Motion | `--duration-fast|normal|slow`, `--ease-out`, `--ease-in-out` | |

The class hues and the speed ramp were stepped in OKLCH and checked with the
dataviz skill's `validate_palette.js` (lightness band, chroma floor, adjacent
normal-vision and CVD separation, surface contrast) for both palettes. The
speed ramp runs cool to hot so faster reads as "more"; each `-ink` token is
the text colour that clears contrast on that fill. Mapping helpers live in
`src/lib/colors.ts` (`classToken`, `classColorVar`, `speedToken`,
`speedColorVar`, `speedInkVar`).

Model classes without their own token fold in: `billboard` -> display,
`smartcard` -> vendor. The knowledge-base "display" kind is not exposed on the
device JSON yet; `classToken(cls, kbKind)` accepts it for when it is.

## Components (src/components)

### Header
| Prop | Type | Notes |
|---|---|---|
| `appName` | `string` | from Wails Status or `t('app.name')` |
| `version` | `string \| null` | |
| `state` | `ConnectionState` | |
| `capabilities` | `ProviderCaps \| null` | |
| `legendOpen` | `boolean` | |
| `onToggleLegend` | `() => void` | |

Contains `ConnectionBadge` (`state`) and `CapabilityChips` (`capabilities`).
When `throughput` is off the chip is a button that opens the localized hint
(`caps.throughput.hint`); it closes on outside click via `clickOutside`.

### Legend
| Prop | Type |
|---|---|
| `onClose` | `() => void` |

Class swatches (icon + label), the speed ramp (fill + ink), the socket shapes
(`PortSocket` glyph + name, including the USB4 bolt), the health ring, and the
diagram's line encoding (thickness, dashed, flow).

### ViewSwitch
| Prop | Type |
|---|---|
| `value` | `ConnectedView` (`'graph' \| 'tree'`) |
| `onChange` | `(next: ConnectedView) => void` |

Segmented control in the "What is connected" pane header; buttons carry
`aria-pressed`. `App.svelte` binds it to `lib/view.svelte.ts`.

### CollectionWarnings
| Prop | Type |
|---|---|
| `warnings` | `readonly string[]` |

The `role="alert"` block for non-fatal snapshot warnings. Rendered by both
`TopologyTree` and `TopologyGraph` so the warnings never depend on the view.

### TopologyGraph -> GraphEdges + GraphNode
| Component | Props |
|---|---|
| `TopologyGraph` | `topology: Topology \| null`, `ctx: TreeContext`, `loading: boolean` |
| `GraphEdges` | `edges: GraphEdge[]`, `width`, `height`, `pulsing: ReadonlySet<string>`, `nameOf: (id) => string`, `showTraffic: boolean` |
| `GraphNode` | `node: GraphNode`, `ctx`, `pulse: boolean` |
| `SocketStrip` | `sockets: SocketSlot[]` |

The hero view. `lib/graph.ts` (`layoutGraph`) turns the snapshot into
absolutely positioned nodes and cubic edges: one left-to-right tree per
controller with the root hub folded into the controller node, then the USB4
router chain with PCIe-carried devices as dashed leaf nodes. Nodes are HTML
(so `ClassIcon` and `LinkBadge` are reused as-is) over one SVG edge layer.

Hub-like nodes (a controller with a root hub, and any device with `hub`) carry
a socket strip: `node.sockets` is one `SocketSlot` (`{ port, y, occupied }`)
per `visiblePorts` entry, spaced `SOCKET_PITCH` (24 px) apart with `SOCKET_PAD`
(10 px) top and bottom, and the node grows to `max(NODE_HEIGHT, pad*2 +
n*pitch)`. `SocketStrip` is an inset panel `SOCKET_STRIP_WIDTH` (48 px) wide
down the node's right edge - `--bg-subtle` behind a `--border-muted` left
border, so it reads as the device's physical edge - with each glyph centred in
it and its centre at exactly the slot's `y`, so a child's edge leaves the
socket it is plugged into (`x1` = parent right edge, `y1` = `parent.y +
slot.y`). Such nodes reserve 56 px of right padding, stop the traffic bar at
the panel, and move the collapse toggle to the bottom of the main column.
Because nodes now differ in height, a parent taller than its children's span
keeps the row it started on and its whole DFS subtree slides down under it;
the row cursor then clears the parent's own bottom edge.

Edge encoding, per the handoff spec:

- thickness = capacity (`edgeWidth`, monotonic in `LinkSpeed`);
- colour = health (`--link-good|slow|idle`), and because colour is never the
  only signal a slow link is also dashed;
- flow = live utilisation: a moving dash overlay in the speed colour whose
  opacity rises with `bps / linkBitrate`. A hub's uplink carries the sum of
  its subtree (`subtreeBps`), so a busy SSD lights the dock's uplink too.
  Nothing flows when `ctx.showMeter` is false.

Nodes reuse the tree's signals (class stripe and icon, `LinkBadge`, severity
tint and warning icon, a 3 px traffic bar with `role="meter"`). Both hub-tree
devices and USB4 routers can be flagged and focused, since insights name
either kind of id. Hubs toggle
with the same `expanded` store as the tree, so the two views stay in sync;
a collapsed hub shows "{n} hidden". A device an insight has just named
pulses for six seconds (node ring and incoming edge), then keeps the
flagged tint. Focus requests scroll and flash the node via `flash`.

Zoom fits the pane width (clamped 0.5..1) until the toolbar is used
(0.5..1.5, fit button returns to auto). Motion (flow, pulse, position
transitions) is disabled under `prefers-reduced-motion`.

### ConnectionBanner
| Prop | Type | Notes |
|---|---|---|
| `state` | `ConnectionState` | shown for `polling` / `offline`, or whenever `error` is set |
| `error` | `string \| null` | last service error, rendered verbatim |
| `apiBase` | `string` | |
| `onRetry` | `() => void` | resets backoff and reconnects |

### StartingScreen
| Prop | Type |
|---|---|
| `error` | `string \| null` |

### TopologyTree -> ControllerNode -> DeviceNode
| Component | Props |
|---|---|
| `TopologyTree` | `topology: Topology \| null`, `ctx: TreeContext`, `loading: boolean` |
| `ControllerNode` | `controller: Controller`, `ctx` |
| `DeviceNode` (recursive) | `device: Device`, `port: Port \| null`, `depth: number`, `ctx` |
| `Usb4Section` | `routers: USB4Router[]` |

`TreeContext` (`lib/tree.ts`): `flagged` (normalised id -> worst severity),
`throughput` (`ThroughputMap`), `showMeter` (provider capability).

`DeviceNode` shows: class icon and a left edge in the class colour, the name
(`deviceName`), `LinkBadge`, a warning icon plus severity tint when the device is
named by an insight, meta (a `PortSocket` glyph and the port number, ports in
use, power, iso reservation, claimed speed when it differs, serial), and a
`ThroughputMeter` for non-hubs. Hubs
collapse/expand; the collapsed set is stored per device id in localStorage
(`lib/expanded.svelte.ts`). Focus requests (`lib/focus.svelte.ts`) expand the
ancestors, scroll the node into view and flash it via the `flash` action.
Root hubs are folded into the controller header (ports in use).

### LinkBadge
| Prop | Type |
|---|---|
| `negotiated` | `LinkSpeed` |
| `max` | `LinkSpeed` |
| `claimed` | `LinkSpeed` |

Fill = negotiated speed token, text = short label (`link.short.*`). Ring =
health (`linkHealth`: good when negotiated >= min(claimed, port max), slow below,
idle when unknown). A second swatch in the port-max colour appears when the
port could do a different speed. Tooltip carries the full sentence.

### ThroughputMeter
| Prop | Type | Notes |
|---|---|---|
| `sample` | `ThroughputSample \| null` | null = idle |
| `speed` | `LinkSpeed` | bar is 100% when read+write saturate this link |
| `enabled` | `boolean` | false renders "Live throughput is off" |

Read fills solid, write fills at reduced opacity, both in the speed colour;
`role="meter"` with `aria-valuenow` as a percentage.

### InsightList / InsightCard
| Component | Props |
|---|---|
| `InsightList` | `insights: Insight[]`, `index: DeviceIndex`, `onFocusDevice: (id) => void` |
| `InsightCard` | `insight: Insight`, `index`, `onFocusDevice` |

Severity stripe and icon, title, explanation, "What to do", affected devices as
`DeviceName` buttons, and a Details expander with evidence bullets, confidence
and the rule id. Empty state: `insights.empty.title`.

### Timeline
| Prop | Type |
|---|---|
| `entries` | `TimelineEntry[]` (newest first, max 100) |
| `index` | `DeviceIndex` |
| `onFocusDevice` | `(id: string) => void` |

Relative timestamps refresh every 10 s; hover shows the wall-clock time.

### DeviceName
| Prop | Type | Notes |
|---|---|---|
| `id` | `string` | any case |
| `index` | `DeviceIndex` | |
| `onFocus` | `(id) => void` (optional) | makes it a button when the device is in the snapshot |
| `fallback` | `string` (optional) | shown for ids no longer in the snapshot |

Renders the name in the device's class colour so a device is recognisable
across the tree, the insight cards and the timeline.

### PortSocket
| Prop | Type | Notes |
|---|---|---|
| `port` | `Port` | |
| `occupied` | `boolean` | empty sockets draw at reduced opacity |
| `size` | `number` (default 14) | width; the 40x16 viewBox sets the height |

The physical socket as an inline SVG, in the proportions of the real thing.
The viewBox is graph pixels at `size` 40, which is what `SocketStrip` passes:
USB-A is a 28x12 rounded rectangle with a 20x4 tongue in the upper half,
USB-C a 26x10 pill with an 18x3 inner pill, internal a 12 px dotted circle
with a 4 px centre mark, unknown a plain 24x10 rectangle with no inner shape.
The socket is centred on the viewBox and the USB4 bolt sits in the reserved
left margin, so glyphs line up whether or not they carry one. The inner shape
takes `speedColorVar(max_link)` and is *filled* only when the socket is
occupied - an empty socket outlines it and drops to 45% opacity - and the bolt
is `speedColorVar('usb4_40')`, so shape, fill and colour never carry the
meaning alone. The outline is `currentColor`, so the parent picks the ink
(`--text-secondary` in the strip and the tree). `role="img"` plus a `<title>`
give the sentence: port number, socket kind, port maximum, occupied or empty,
and the printed `label` / `position` when the provider reports them. The
diagram uses the full 40 px via `SocketStrip`; `DeviceNode` keeps the small
default before the port number so the tree does not bloat, and `Legend` draws
its samples at 34 px.

### ClassIcon
| Prop | Type |
|---|---|
| `cls` | `DeviceClass` |
| `size` | `number` (default 15) |

lucide icon per class token, coloured with `--class-*`, with a localized tooltip.

## Keyboard

- Hub toggles (tree and diagram) and Details toggles are real buttons with `aria-expanded`.
- Device names in cards and the timeline are buttons; activating one scrolls
  and flashes the device in whichever view is showing.
- The view switch and the diagram's zoom buttons are labelled buttons; the
  fit button carries `aria-pressed` while auto-fit is active.
- The legend and the capability hint close with their close button or an outside click.
