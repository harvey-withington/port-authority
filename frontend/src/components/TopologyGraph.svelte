<script lang="ts">
  import { untrack } from 'svelte'
  import type { Topology } from '../lib/api/types'
  import type { TreeContext } from '../lib/tree'
  import type { DetailLevel } from '../lib/view.svelte'
  import { layoutGraph, layoutPhysical } from '../lib/graph'
  import { deviceName, routerName } from '../lib/topology'
  import { expanded } from '../lib/expanded.svelte'
  import { observeWidth } from '../lib/actions'
  import { clampZoom, zoom } from '../lib/zoom.svelte'
  import { t } from '../lib/i18n.svelte'
  import CollectionWarnings from './CollectionWarnings.svelte'
  import GraphEdges from './GraphEdges.svelte'
  import GraphNodeCard from './GraphNode.svelte'

  interface Props {
    topology: Topology | null
    ctx: TreeContext
    loading: boolean
    /** 'physical' draws the boxes on the desk, 'logical' every hub the OS reports. */
    detail: DetailLevel
  }

  let { topology, ctx, loading, detail }: Props = $props()

  /** How long a newly flagged device and its uplink pulse. */
  const PULSE_MS = 6_000

  const layout = $derived((detail === 'physical' ? layoutPhysical : layoutGraph)(topology, {
    isExpanded: (id) => expanded.isExpanded(id),
    throughput: ctx.throughput,
  }))
  const nodeById = $derived(new Map(layout.nodes.map((n) => [n.id, n])))

  function nameOf(id: string): string {
    const n = nodeById.get(id)
    if (!n) return ''
    if (n.kind === 'controller') return n.controller?.name ?? ''
    if (n.kind === 'router') return n.router ? routerName(n.router) : ''
    if (n.kind === 'carried') return n.label ?? ''
    if (n.kind === 'box') return n.enclosure?.name || (n.device ? deviceName(n.device) : t('box.host'))
    return n.device ? deviceName(n.device) : ''
  }

  // Fit to the pane width until the user takes over. Only this component
  // knows the pane's width and the drawing's, so it works the fit out and
  // hands it to the shared store the header control also reads.
  let viewportWidth = $state(0)
  const fitZoom = $derived(viewportWidth > 0 && layout.width > 0 ? Math.min(1, clampZoom(viewportWidth / layout.width)) : 1)
  $effect(() => {
    zoom.setFit(fitZoom)
  })
  const scale = $derived(zoom.value)

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
    <div class="viewport" use:observeWidth={(w) => (viewportWidth = w)}>
      <div class="canvas" style:width={`${layout.width * scale}px`} style:height={`${layout.height * scale}px`}>
        <div class="scaled" style:transform={`scale(${scale})`} style:width={`${layout.width}px`} style:height={`${layout.height}px`}>
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
    /* Fills the pane it is given rather than a share of the window: the
       pane's height is the user's to set with the splitter. */
    flex: 1;
    min-height: 0;
  }
  .viewport {
    position: relative;
    overflow: auto;
    flex: 1;
    min-height: 120px;
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
