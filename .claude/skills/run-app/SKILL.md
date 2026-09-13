---
name: run-app
description: Launch Port Authority's API on a fixture and screenshot the Svelte UI in headless Edge, to see a frontend change rendered. Use when asked to run, preview, or screenshot the app, or to confirm a UI change works for real.
---

# Run and screenshot Port Authority

The UI is a plain HTTP + WebSocket consumer, so "running the app" for a
visual check means: fixture-backed API on a spare port, the built frontend
on a preview server, headless Edge pointed at both. No Wails window needed.

## Do not touch port 7911

The user's built `port-authority.exe` is usually already listening on
`127.0.0.1:7911` with live hardware data. Check before starting anything:

```bash
netstat -ano | grep LISTENING | grep ":7911 "
```

If it is taken, leave it alone and use 7912 below. Never `taskkill` that PID.

## 1. API on a fixture (port 7912)

```bash
cd s:/Local/Code/Projects/port-authority/port-authority-1.0
go run ./cmd/pactl serve --addr 127.0.0.1:7912 --kb-dir "" --fixture testdata/fixtures/deviant-caldigit-ts4-ssd-on-tb4.json
```

`--kb-dir ""` keeps the run read-only: no local docks, no community fetch,
so the screenshot shows the shipped knowledge base only. To exercise the
"set up this dock" flow, point `--kb-dir` at a scratch folder instead.

Run it in the background, then poll rather than sleep:

```bash
for i in $(seq 1 60); do curl -sf http://127.0.0.1:7912/api/v1/health >/dev/null && break; sleep 1; done
curl -s http://127.0.0.1:7912/api/v1/topology | head -c 300
```

The mock provider reports `throughput: false`, so live-flow visuals (meter
bars, edge flow overlay) never appear this way; unit tests cover them.
Other fixtures live in `testdata/fixtures/`.

## 2. Frontend preview (port 4173)

`vite preview` serves `frontend/dist`, so build first and rebuild after every change:

```bash
cd s:/Local/Code/Projects/port-authority/port-authority-1.0/frontend
npx vite build
npx vite preview --port 4173 --strictPort     # background
for i in $(seq 1 30); do curl -sf http://localhost:4173/ >/dev/null && break; sleep 1; done
```

The page finds the API from the `?api=` query parameter (see `src/lib/discover.ts`).

## 3. Screenshot with headless Edge

`chromium-cli` is not installed on this machine. Edge is, and its
`--screenshot` mode works:

```bash
S="$SCRATCHPAD"    # any writable dir; use the session scratchpad
"/c/Program Files (x86)/Microsoft/Edge/Application/msedge.exe" \
  --headless=new --disable-gpu --no-first-run --no-default-browser-check \
  --user-data-dir="$S\\edge-profile-$RANDOM" \
  --window-size=1700,1000 --hide-scrollbars --virtual-time-budget=8000 \
  --screenshot="$S\\graph.png" \
  "http://localhost:4173/?api=http://127.0.0.1:7912"
```

Then Read the PNG and actually look at it. Gotchas that were hit:

- Pass Windows-style paths (`C:\...`) to `--screenshot` and `--user-data-dir`;
  a `/c/...` path exits 0 and writes nothing.
- Use a **fresh** `--user-data-dir` every run. A lingering headless instance
  on a reused profile absorbs the launch: exit 0, no file.
- `--virtual-time-budget` lets the WebSocket hello and first snapshot arrive
  before the capture; without it the screenshot shows the connecting state.
- The screenshot is dark theme (system preference). Light theme cannot be
  forced from the command line; tokens in `src/app.css` cover both.
- Edge cannot click, so the Tree view and hub toggles are only reachable by
  changing the default in `src/lib/view.svelte.ts` temporarily, or by trusting
  the component tests in `src/components/components.test.ts`.

## 4. Stop what you started

Kill by port, and only your own ports:

```bash
for port in 7912 4173; do
  pid=$(netstat -ano | grep LISTENING | grep ":$port " | awk '{print $NF}' | head -1)
  [ -n "$pid" ] && taskkill //PID $pid //T //F
done
```

## Checks that belong with a UI change

```bash
cd frontend && npx svelte-check --tsconfig ./tsconfig.json && npx vitest run
```
