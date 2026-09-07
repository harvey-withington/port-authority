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
- Shared DOM behaviours are actions in `src/lib/actions.ts` (`flash`, `clickOutside`).

## Data flow

`App.svelte` discovers the API (`lib/discover.ts`: Wails `Status()` polling or
`?api=` / loopback default), then creates one `LiveStore` (`lib/live.svelte.ts`).
The store owns connection state, the snapshot, insights, throughput and the
timeline; components receive plain values as props. Pure logic lives in `lib/`
and is unit-tested: `format`, `link`, `ids`, `topology`, `insights`, `timeline`,
`throughput`, `colors`, `discover`, `api/client`.

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

Class swatches (icon + label), the speed ramp (fill + ink), the health ring.

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
named by an insight, meta (port, ports in use, power, iso reservation, claimed
speed when it differs, serial), and a `ThroughputMeter` for non-hubs. Hubs
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

### ClassIcon
| Prop | Type |
|---|---|
| `cls` | `DeviceClass` |
| `size` | `number` (default 15) |

lucide icon per class token, coloured with `--class-*`, with a localized tooltip.

## Keyboard

- Hub toggles and Details toggles are real buttons with `aria-expanded`.
- Device names in cards and the timeline are buttons; activating one scrolls
  and flashes the device in the tree.
- The legend and the capability hint close with their close button or an outside click.
