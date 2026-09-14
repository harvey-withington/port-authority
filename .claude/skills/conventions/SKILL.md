---
name: conventions
description: Port Authority's contributor conventions - architecture rules, knowledge base and grouping rules, frontend contract, tests, fixtures and commit message format. Use before changing code in this repo, writing a commit message, or adding a dock or device to the knowledge base.
---

# Port Authority conventions

Read this before changing code. The README says what the app is; this
says how changes are made so they fit.

## Architecture

- `core/*` never imports anything under `platform/`. A new OS is a new
  provider behind `provider.Provider`, and nothing else changes.
- Consumers check `ProviderCaps`, never `runtime.GOOS`.
- The provider observes and measures. It never mutates device
  configuration, and it never corrects what the OS reports: corrections
  belong in `core/enrich` or the UI, where they can be safe on every
  machine.
- Enrichment derives, it does not observe. Anything it stamps on a
  snapshot (enclosures, a router's enclosure tie, resolved names) is
  recomputed on every pass, so a replayed fixture is re-derived under the
  current rules rather than trusted.
- Every field the provider may fail to read is optional in the model and
  the app degrades to its previous behaviour without it. The USB4 router
  property set (`platform/win/usb4.go`) is undocumented and is the
  standing example.

## Grouping never guesses

A dock draws as one box only because the knowledge base says which hubs
are inside it. The rules in `core/enrich/enclosure.go` fold hubs on two
kinds of evidence: the OS stating a pairing (a socket's USB 2 and USB 3
halves), or a knowledge base entry. They never fold on the shape of the
tree or on names looking alike, because a grouping rule that can absorb a
box hides a whole object when it is wrong. The `dock-not-recognised`
insight asks the user instead. Keep it that way.

## Knowledge base

`core/kb` stacks three layers, later ones winning by dock id and by hub:
shipped (`data/docks.json`, `data/devices.json`), shared (community, fetched from
usb-device-kb) and local (the user's own, written by the app). All layers
use the `DockEntry` schema. See `docs/decisions/0002-knowledge-base-layers.md`.

The data itself lives in the public MIT repo
https://github.com/harvey-withington/usb-device-kb, cloned into this
project as `usb-device-kb/` (ignored by git, like `plan/`). Edit docks and
devices there, where the validator and schema are, commit, then
`go run ./tools/syncdata` copies the files into `core/kb/data` and writes
`PROVENANCE`. Never edit `core/kb/data/*.json` directly.

Adding a dock to `docks.json`:

- `hubs`: every logical hub the dock exposes, as lower-case `vid:pid`.
  Never a chipset hub found in many docks; those go in
  `generic_internal_hubs`.
- `usb4`: the vendor and model strings the dock's router announces (read
  them with `go run ./tools/usb4props`), not the router's vid:pid, which
  is the bridge silicon.
- `verified`: `hardware` when checked on a real dock, otherwise where the
  data came from and when. `port_map` entries must be verified on
  hardware; a guessed label is worse than none.
- Insight rules that judge a link need an expectation from the knowledge
  base (`uplink.max_link`, a device's `max_link`); without one they stay
  quiet rather than assume.

## Frontend

`frontend/UI-CONVENTIONS.md` is the contract: tokens, components, props,
keyboard behaviour. Update it in the same change as any shared
component or pattern. Its ground rules in short:

- Svelte 5 runes, typed props via `interface Props`, strict TypeScript,
  never `any`.
- Every user-visible string goes through `t('key')` in
  `src/lib/locales/en.json`.
- Colours and sizes are tokens in `src/app.css`; components never
  hardcode a colour, and colour is never the only signal.
- No component over about 300 lines; extract a child when one grows.
- Errors the user should know about are rendered, never only logged. No
  `alert()` or `confirm()`: destructive actions go through
  `ConfirmDialog`.
- Keyed `{#each}` blocks use stable ids, never the index.
- Shared DOM behaviours are actions in `src/lib/actions.ts`; components
  never construct a `ResizeObserver` themselves.
- Pure logic lives in `src/lib` and is unit-tested; components receive
  plain values as props and small cross-cutting state lives in rune
  modules under `src/lib/*.svelte.ts`.

## Tests and checks

- Go: `go build ./... && go vet ./... && go test ./...`.
- Frontend: `cd frontend && npm run check && npm run test:run`.
- A UI change is not done until it has been seen rendered: the `run-app`
  skill serves a fixture and screenshots the page in headless Edge.
- Pure logic gets a unit test in `lib/`; components get a smoke test in
  `components.test.ts` that renders real fixture data and asserts the
  text a user relies on.

## Fixtures

`testdata/fixtures/*.json` are real snapshots and the regression suite
for the knowledge base. Capture with `pactl snapshot --pretty`, then
always `go run ./tools/scrubfixture <file>`: it replaces serial numbers
and the machine's name and is a no-op on a scrubbed file. Grep the file
case-insensitively for anything personal before committing it.

## Commit messages

One line: a conventional-commit prefix, then the changes as brief clauses
separated by ` / `, no body.

```
feat: severity glyphs on findings / clickable flags filter the panel / host name on the computer box
fix: scrubber no longer re-scrubs already scrubbed ids
```

## Documentation

- `README.md` is for people who run or contribute to the app.
- `docs/decisions/` holds decision records (ADRs); write one when a
  choice shapes the architecture or rules out an alternative for a
  reason a later reader would otherwise have to rediscover.
- Planning notes and TODOs do not live in this repo.
