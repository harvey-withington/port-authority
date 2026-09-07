<script lang="ts">
  import { onMount } from 'svelte'
  import { Status } from '../wailsjs/go/main/App'
  import type { Severity } from './lib/api/types'
  import { browserDiscovery, isWailsHost, pollStatus, type Discovered } from './lib/discover'
  import { createLive, type LiveStore } from './lib/live.svelte'
  import { flaggedDevices } from './lib/insights'
  import { ancestorIds } from './lib/topology'
  import { emptyThroughput } from './lib/throughput'
  import type { TreeContext } from './lib/tree'
  import { expanded } from './lib/expanded.svelte'
  import { focus } from './lib/focus.svelte'
  import { view } from './lib/view.svelte'
  import { relativeTime } from './lib/format'
  import { t } from './lib/i18n.svelte'
  import Header from './components/Header.svelte'
  import Legend from './components/Legend.svelte'
  import ConnectionBanner from './components/ConnectionBanner.svelte'
  import StartingScreen from './components/StartingScreen.svelte'
  import ViewSwitch from './components/ViewSwitch.svelte'
  import TopologyGraph from './components/TopologyGraph.svelte'
  import TopologyTree from './components/TopologyTree.svelte'
  import InsightList from './components/InsightList.svelte'
  import Timeline from './components/Timeline.svelte'

  let discovered = $state.raw<Discovered | null>(null)
  let startError = $state<string | null>(null)
  let live = $state.raw<LiveStore | null>(null)
  let legendOpen = $state(false)

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
  const ctx = $derived<TreeContext>({
    flagged,
    throughput: live?.throughput ?? emptyThroughput(),
    showMeter: live?.capabilities?.throughput ?? false,
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
      <div class="legend-slot"><Legend onClose={() => (legendOpen = false)} /></div>
    {/if}
    <ConnectionBanner state={live.state} error={live.error} apiBase={live.apiBase} onRetry={() => live?.retry()} />

    <main class="panes">
      <section class="pane" aria-labelledby="pane-connected">
        <div class="pane-head">
          <h2 id="pane-connected">{t('pane.connected')}</h2>
          <ViewSwitch value={view.current} onChange={(next) => view.set(next)} />
        </div>
        {#if view.current === 'graph'}
          <TopologyGraph topology={live.topology} {ctx} loading={live.state === 'connecting'} />
        {:else}
          <TopologyTree topology={live.topology} {ctx} loading={live.state === 'connecting'} />
        {/if}
      </section>
      <section class="pane" aria-labelledby="pane-found">
        <h2 id="pane-found">{t('pane.found')}</h2>
        <InsightList insights={live.insights} index={live.index} onFocusDevice={focusDevice} />
        <h2 class="changed">{t('pane.changed')}</h2>
        <Timeline entries={live.timeline} index={live.index} onFocusDevice={focusDevice} />
      </section>
    </main>

    <footer class="footer">
      {#if live.capturedAt}<span>{t('footer.captured', { when: relativeTime(live.capturedAt) })}</span>{/if}
      <span class="mono">{t('footer.api', { base: live.apiBase })}</span>
    </footer>
  </div>
{/if}

<style>
  .app {
    min-height: 100vh;
    display: flex;
    flex-direction: column;
  }
  .legend-slot {
    position: absolute;
    right: var(--space-4);
    top: calc(var(--header-height) + var(--space-2));
    z-index: 20;
  }
  .panes {
    flex: 1;
    display: grid;
    grid-template-columns: minmax(0, 3fr) minmax(0, 2fr);
    gap: var(--pane-gap);
    padding: var(--space-4);
    align-items: start;
  }
  .pane {
    background: var(--bg-surface);
    border: 1px solid var(--border-muted);
    border-radius: var(--radius-lg);
    padding: var(--space-3) var(--space-3) var(--space-4);
    min-width: 0;
  }
  h2 {
    font-size: var(--font-size-xs);
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--text-muted);
    margin: 0 var(--space-2) var(--space-2);
  }
  h2.changed {
    margin-top: var(--space-5);
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
  .footer {
    display: flex;
    gap: var(--space-4);
    padding: var(--space-2) var(--space-4);
    border-top: 1px solid var(--border-muted);
    color: var(--text-faint);
    font-size: var(--font-size-xs);
  }
  @media (max-width: 900px) {
    .panes {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
