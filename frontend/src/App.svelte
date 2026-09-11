<script lang="ts">
  import { onMount } from 'svelte'
  import { Status } from '../wailsjs/go/main/App'
  import type { Severity } from './lib/api/types'
  import { browserDiscovery, isWailsHost, pollStatus, type Discovered } from './lib/discover'
  import { createLive, type LiveStore } from './lib/live.svelte'
  import { flaggedDevices } from './lib/insights'
  import { ancestorIds, type DeviceIndex } from './lib/topology'
  import { emptyThroughput } from './lib/throughput'
  import type { TreeContext } from './lib/tree'
  import { expanded } from './lib/expanded.svelte'
  import { focus } from './lib/focus.svelte'
  import { findingFilter } from './lib/findingFilter.svelte'
  import { view } from './lib/view.svelte'
  import { layout, MAX_DIAGRAM_SHARE, MIN_DIAGRAM_SHARE } from './lib/layout.svelte'
  import { relativeTime } from './lib/format'
  import { draggableWindow } from './lib/actions'
  import { t } from './lib/i18n.svelte'
  import Header from './components/Header.svelte'
  import Legend from './components/Legend.svelte'
  import ConnectionBanner from './components/ConnectionBanner.svelte'
  import StartingScreen from './components/StartingScreen.svelte'
  import ViewSwitch from './components/ViewSwitch.svelte'
  import DetailSwitch from './components/DetailSwitch.svelte'
  import ZoomControl from './components/ZoomControl.svelte'
  import TopologyGraph from './components/TopologyGraph.svelte'
  import TopologyTree from './components/TopologyTree.svelte'
  import InsightList from './components/InsightList.svelte'
  import FindingsFilter from './components/FindingsFilter.svelte'
  import Timeline from './components/Timeline.svelte'
  import Pane from './components/Pane.svelte'
  import PaneSplitter from './components/PaneSplitter.svelte'
  import DockDialogs from './components/DockDialogs.svelte'

  let discovered = $state.raw<Discovered | null>(null)
  let startError = $state<string | null>(null)
  let live = $state.raw<LiveStore | null>(null)
  let legendOpen = $state(false)
  /** Height of the pane area, so a drag on the splitter converts to a share. */
  let paneHeight = $state(0)

  onMount(() => {
    const abort = new AbortController()
    let store: LiveStore | null = null
    void (async () => {
      try {
        const found = isWailsHost()
          ? await pollStatus(() => Status(), {
              signal: abort.signal,
              onUpdate: (s) => (startError = s.error ?? null),
              onFailure: (e) => (startError = t('starting.statusFailed', { error: e.message })),
            })
          : browserDiscovery(window.location.search)
        if (abort.signal.aborted) return
        discovered = found
        store = createLive(found.apiBase, found.streamUrl)
        live = store
        store.start()
      } catch (e) {
        if (!abort.signal.aborted) startError = e instanceof Error ? e.message : String(e)
      }
    })()
    return () => {
      abort.abort()
      store?.stop()
    }
  })

  const appName = $derived(discovered?.appName ?? t('app.name'))
  const flagged = $derived<ReadonlyMap<string, Severity>>(live ? flaggedDevices(live.insights) : new Map())
  // A snippet is its own function, so the `live` narrowing above does not
  // reach into it; the index it needs is read here instead.
  const deviceIndex = $derived<DeviceIndex>(live?.index ?? new Map())
  const ctx = $derived<TreeContext>({
    flagged,
    throughput: live?.throughput ?? emptyThroughput(),
    showMeter: live?.capabilities?.throughput ?? false,
    docks: live?.docks,
  })

  function focusDevice(id: string): void {
    if (!live) return
    expanded.expand(ancestorIds(live.index, id))
    focus.request(id)
  }
</script>

{#if !live}
  <StartingScreen error={startError} />
{:else}
  <div class="app">
    <Header
      {appName}
      version={discovered?.version ?? null}
      state={live.state}
      capabilities={live.capabilities}
      {legendOpen}
      onToggleLegend={() => (legendOpen = !legendOpen)}
    />
    {#if legendOpen}
      <div class="legend-slot" use:draggableWindow={{ key: 'pa-legend-position', handle: 'header', initialTop: 56 }}>
        <Legend onClose={() => (legendOpen = false)} />
      </div>
    {/if}
    <ConnectionBanner state={live.state} error={live.error} apiBase={live.apiBase} onRetry={() => live?.retry()} />

    <main class="panes" bind:clientHeight={paneHeight}>
      <!-- With the panels hidden the diagram is the only flex child, and a
           grow factor below 1 would leave the rest of the space empty. -->
      <section
        class="pane diagram"
        style:flex={layout.panelsOpen ? `${layout.diagramShare} 1 0` : '1 1 0'}
        aria-labelledby="pane-connected"
      >
        <div class="pane-head">
          <h2 id="pane-connected">{t('pane.connected')}</h2>
          <div class="switches">
            {#if view.current === 'graph' && live.topology}
              <ZoomControl />
            {/if}
            {#if view.current === 'graph'}
              <DetailSwitch value={view.detail} onChange={(next) => view.setDetail(next)} />
            {/if}
            <ViewSwitch value={view.current} onChange={(next) => view.set(next)} />
          </div>
        </div>
        <div class="pane-body">
          {#if view.current === 'graph'}
            <TopologyGraph topology={live.topology} {ctx} loading={live.state === 'connecting'} detail={view.detail} />
          {:else}
            <TopologyTree topology={live.topology} {ctx} loading={live.state === 'connecting'} />
          {/if}
        </div>
      </section>

      <PaneSplitter
        value={layout.diagramShare}
        min={MIN_DIAGRAM_SHARE}
        max={MAX_DIAGRAM_SHARE}
        track={paneHeight}
        open={layout.panelsOpen}
        onChange={(next) => layout.setDiagramShare(next)}
        onToggle={() => layout.togglePanels()}
      />

      {#if layout.panelsOpen}
        <div class="below" id="pane-panels" style:flex="{1 - layout.diagramShare} 1 0">
          <Pane id="found" title={t('pane.found')}>
            {#snippet controls()}
              <FindingsFilter {flagged} index={deviceIndex} />
            {/snippet}
            <InsightList
              insights={live.insights}
              index={live.index}
              filter={findingFilter.current}
              onFocusDevice={focusDevice}
              onClearFilter={() => findingFilter.clear()}
            />
          </Pane>
          <Pane id="changed" title={t('pane.changed')}>
            <Timeline entries={live.timeline} index={live.index} onFocusDevice={focusDevice} />
          </Pane>
        </div>
      {/if}
    </main>

    <footer class="footer">
      {#if live.capturedAt}<span>{t('footer.captured', { when: relativeTime(live.capturedAt) })}</span>{/if}
      <span class="mono">{t('footer.api', { base: live.apiBase })}</span>
    </footer>
    <DockDialogs {live} />
  </div>
{/if}

<style>
  /* Fixed to the viewport: the shell never scrolls, each panel does. */
  .app {
    height: 100vh;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }
  /* A floating window: the action places it and remembers where it was
     put. Fixed rather than absolute so dragging is measured against the
     viewport, which is also what keeps it on screen after a resize.
     The width is set here rather than on the panel inside: a shrink-to-fit
     box sizes to its content's natural width, and a percentage width on
     the child does not constrain that, so the slot would end up wider than
     what it draws. The drag clamp measures this box, so the overhang
     became an invisible wall. */
  .legend-slot {
    position: fixed;
    left: 0;
    top: 0;
    z-index: 20;
    width: min(420px, calc(100vw - 24px));
  }
  /* The diagram is a left-to-right tree, so it gets the full width and the
     two lists sit side by side underneath rather than stealing from it. */
  .panes {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    padding: var(--space-4);
  }
  .pane {
    background: var(--bg-surface);
    border: 1px solid var(--border-muted);
    border-radius: var(--radius-lg);
    min-width: 0;
  }
  .pane.diagram {
    display: flex;
    flex-direction: column;
    min-height: 0;
    padding: var(--space-3);
    overflow: hidden;
  }
  .pane-body {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }
  .below {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
    gap: var(--pane-gap);
    min-height: 0;
  }
  h2 {
    font-size: var(--font-size-xs);
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--text-muted);
    margin: 0 var(--space-2) var(--space-2);
  }
  .pane-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
    margin: 0 var(--space-2) var(--space-2);
  }
  .pane-head h2 {
    margin: 0;
  }
  .switches {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
  }
  .footer {
    display: flex;
    gap: var(--space-4);
    padding: var(--space-2) var(--space-4);
    border-top: 1px solid var(--border-muted);
    color: var(--text-faint);
    font-size: var(--font-size-xs);
  }
  /* Too narrow for two lists abreast: stack them and let the row scroll. */
  @media (max-width: 900px) {
    .below {
      grid-template-columns: minmax(0, 1fr);
      grid-auto-rows: minmax(0, 1fr);
      overflow-y: auto;
    }
  }
</style>
