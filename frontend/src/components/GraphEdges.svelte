<script lang="ts">
  import type { GraphEdge } from '../lib/graph'
  import { speedColorVar } from '../lib/colors'
  import { formatThroughput, linkDescription } from '../lib/format'
  import { t } from '../lib/i18n.svelte'

  interface Props {
    edges: GraphEdge[]
    width: number
    height: number
    /** Normalised node ids whose incoming edge should pulse with the node. */
    pulsing: ReadonlySet<string>
    /** Display name for a node id, used in the hover text. */
    nameOf: (id: string) => string
    /** Whether the provider streams throughput; the flow overlay only shows when it does. */
    showTraffic: boolean
  }

  let { edges, width, height, pulsing, nameOf, showTraffic }: Props = $props()

  const MIN_FLOW_WIDTH = 1.5
  const FLOW_WIDTH_RATIO = 0.55

  /** A horizontal S-curve from the parent's right edge to the child's left edge. */
  function path(e: GraphEdge): string {
    const mx = (e.x1 + e.x2) / 2
    return `M ${e.x1} ${e.y1} C ${mx} ${e.y1}, ${mx} ${e.y2}, ${e.x2} ${e.y2}`
  }

  function title(e: GraphEdge): string {
    const parts = [
      t('graph.edge.title', { name: nameOf(e.to), speed: linkDescription(e.speed) }),
      t(`link.health.${e.health}`),
    ]
    if (showTraffic) {
      parts.push(e.bps > 0
        ? t('graph.edge.traffic', { rate: formatThroughput(e.bps), percent: `${Math.round(e.utilization * 100)}%` })
        : t('graph.edge.idle'))
    }
    return parts.join('. ')
  }

  function flowWidth(e: GraphEdge): number {
    return Math.max(MIN_FLOW_WIDTH, e.width * FLOW_WIDTH_RATIO)
  }
</script>

<svg class="edges" {width} {height} viewBox="0 0 {width} {height}" aria-hidden="true">
  {#each edges as e (e.id)}
    <g class="edge health-{e.health}" class:pulse={pulsing.has(e.to)} data-edge-to={e.to}>
      <title>{title(e)}</title>
      <path class="hit" d={path(e)} />
      <path class="base" d={path(e)} style:stroke-width={`${e.width}px`} />
      {#if showTraffic && e.bps > 0}
        <path
          class="flow"
          d={path(e)}
          style:stroke={speedColorVar(e.speed)}
          style:stroke-width={`${flowWidth(e)}px`}
          style:opacity={0.35 + 0.65 * e.utilization}
        />
      {/if}
    </g>
  {/each}
</svg>

<style>
  .edges {
    position: absolute;
    left: 0;
    top: 0;
    overflow: visible;
    pointer-events: none;
  }
  .edge {
    pointer-events: stroke;
  }
  path {
    fill: none;
    stroke-linecap: round;
  }
  .hit {
    stroke: transparent;
    stroke-width: 14px;
  }
  .base {
    stroke: var(--link-idle);
    transition: d var(--duration-slow) var(--ease-out);
  }
  .health-good .base {
    stroke: var(--link-good);
    opacity: 0.85;
  }
  .health-slow .base {
    stroke: var(--link-slow);
    stroke-dasharray: 8 5;
  }
  .health-idle .base {
    opacity: 0.7;
  }
  .edge:hover .base {
    opacity: 1;
  }
  .flow {
    stroke-dasharray: 5 11;
    animation: edge-flow 1.1s linear infinite;
  }
  @keyframes edge-flow {
    from { stroke-dashoffset: 32; }
    to { stroke-dashoffset: 0; }
  }
  .edge.pulse .base {
    animation: edge-pulse 1.2s var(--ease-in-out) 4;
  }
  @keyframes edge-pulse {
    0%, 100% { opacity: 0.85; }
    50% { opacity: 0.3; }
  }
  @media (prefers-reduced-motion: reduce) {
    .flow,
    .edge.pulse .base {
      animation: none;
    }
    .base {
      transition: none;
    }
  }
</style>
