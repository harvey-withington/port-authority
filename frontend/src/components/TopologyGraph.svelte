<script lang="ts">
  import { untrack } from 'svelte'
  import { Maximize2, Minus, Plus } from 'lucide-svelte'
  import type { Topology } from '../lib/api/types'
  import type { TreeContext } from '../lib/tree'
  import { layoutGraph } from '../lib/graph'
  import { deviceName } from '../lib/topology'
  import { expanded } from '../lib/expanded.svelte'
  import { observeWidth } from '../lib/actions'
  import { t } from '../lib/i18n.svelte'
  import CollectionWarnings from './CollectionWarnings.svelte'
  import GraphEdges from './GraphEdges.svelte'
  import GraphNodeCard from './GraphNode.svelte'

  interface Props {
    topology: Topology | null
    ctx: TreeContext
    loading: boolean
  }

  let { topology, ctx, loading }: Props = $props()

  const MIN_ZOOM = 0.5
  const MAX_ZOOM = 1.5
  const ZOOM_STEP = 0.15
  /** How long a newly flagged device and its uplink pulse. */
  const PULSE_MS = 6_000

  const layout = $derived(layoutGraph(topology, {
    isExpanded: (id) => expanded.isExpanded(id),
    throughput: ctx.throughput,
  }))
  const nodeById = $derived(new Map(layout.nodes.map((n) => [n.id, n])))

  function nameOf(id: string): string {
    const n = nodeById.get(id)
    if (!n) return ''
    if (n.kind === 'controller') return n.controller?.name ?? ''
    if (n.kind === 'router') return n.router?.name ?? ''
    if (n.kind === 'carried') return n.label ?? ''
    return n.device ? deviceName(n.device) : ''
  }

  // Zoom: fit to the pane width until the user takes over with the buttons.
  let viewportWidth = $state(0)
  let manualZoom = $state<number | null>(null)
  const clamp = (z: number): number => Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, z))
  const fitZoom = $derived(viewportWidth > 0 && layout.width > 0 ? Math.min(1, clamp(viewportWidth / layout.width)) : 1)
  const zoom = $derived(manualZoom ?? fitZoom)
  const zoomLabel = $derived(t('format.percent', { n: Math.round(zoom * 100) }))

  function zoomBy(delta: number): void {
    manualZoom = clamp(zoom + delta)
  }

  // Devices an insight has just named pulse for a while, then settle to the flagged tint.
  let pulsing = $state<ReadonlySet<string>>(new Set())
  let seen = new Set<string>()
  const timers = new Set<ReturnType<typeof setTimeout>>()

  $effect(() => {
    const fresh = [...ctx.flagged.keys()].filter((id) => !seen.has(id))
    seen = new Set(ctx.flagged.keys())
    if (fresh.length === 0) return
    pulsing = new Set([...untrack(() => pulsing), ...fresh])
    const timer = setTimeout(() => {
      timers.delete(timer)
      const next = new Set(pulsing)
      for (const id of fresh) next.delete(id)
      pulsing = next
    }, PULSE_MS)
    timers.add(timer)
  })

  $effect(() => () => {
    for (const timer of timers) clearTimeout(timer)
  })
</script>

<div class="graph" aria-label={t('graph.label')}>
  <CollectionWarnings warnings={topology?.warnings ?? []} />

  {#if !topology}
    <p class="empty">{loading ? t('tree.loading') : t('tree.empty')}</p>
  {:else if layout.nodes.length === 0}
    <p class="empty">{t('tree.empty')}</p>
  {:else}
    <div class="toolbar" role="group" aria-label={t('graph.zoom')}>
      <button class="tool" onclick={() => zoomBy(-ZOOM_STEP)} disabled={zoom <= MIN_ZOOM} aria-label={t('graph.zoomOut')} title={t('graph.zoomOut')}>
        <Minus size={14} aria-hidden="true" />
      </button>
      <span class="zoom mono" aria-live="polite">{zoomLabel}</span>
      <button class="tool" onclick={() => zoomBy(ZOOM_STEP)} disabled={zoom >= MAX_ZOOM} aria-label={t('graph.zoomIn')} title={t('graph.zoomIn')}>
        <Plus size={14} aria-hidden="true" />
      </button>
      <button class="tool" onclick={() => (manualZoom = null)} aria-pressed={manualZoom === null} aria-label={t('graph.zoomFit')} title={t('graph.zoomFit')}>
        <Maximize2 size={14} aria-hidden="true" />
      </button>
    </div>

    <div class="viewport" use:observeWidth={(w) => (viewportWidth = w)}>
      <div class="canvas" style:width={`${layout.width * zoom}px`} style:height={`${layout.height * zoom}px`}>
        <div class="scaled" style:transform={`scale(${zoom})`} style:width={`${layout.width}px`} style:height={`${layout.height}px`}>
          <GraphEdges edges={layout.edges} width={layout.width} height={layout.height} {pulsing} {nameOf} showTraffic={ctx.showMeter} />
          {#each layout.nodes as node (node.id)}
            <GraphNodeCard {node} {ctx} pulse={pulsing.has(node.id)} />
          {/each}
        </div>
      </div>
    </div>
  {/if}
</div>

<style>
  .graph {
    display: flex;
    flex-direction: column;
    position: relative;
  }
  .toolbar {
    align-self: flex-end;
    margin-bottom: var(--space-2);
    display: inline-flex;
    align-items: center;
    gap: 2px;
    padding: 2px;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 999px;
    box-shadow: 0 2px 8px var(--shadow);
  }
  .tool {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 24px;
    height: 24px;
    padding: 0;
    border: 0;
    border-radius: 999px;
    background: transparent;
    color: var(--text-secondary);
  }
  .tool:hover:not(:disabled),
  .tool[aria-pressed='true'] {
    background: var(--bg-subtle-hover);
    color: var(--text-strong);
  }
  .tool:disabled {
    opacity: 0.4;
    cursor: default;
  }
  .zoom {
    min-width: 40px;
    text-align: center;
    font-size: var(--font-size-xs);
    color: var(--text-muted);
    font-variant-numeric: tabular-nums;
  }
  .viewport {
    position: relative;
    overflow: auto;
    max-height: 72vh;
    min-height: 160px;
    border-radius: var(--radius-md);
    background:
      radial-gradient(circle, var(--border-muted) 1px, transparent 1px) 0 0 / 18px 18px,
      var(--bg-base);
  }
  .canvas {
    position: relative;
  }
  .scaled {
    position: absolute;
    left: 0;
    top: 0;
    transform-origin: 0 0;
  }
  .empty {
    padding: var(--space-4);
    color: var(--text-muted);
    text-align: center;
  }
</style>
