<script lang="ts">
  import { ChevronDown, ChevronRight, Cpu, Dock, HardDrive, Laptop, TriangleAlert, Zap } from 'lucide-svelte'
  import type { GraphNode } from '../lib/graph'
  import type { Severity } from '../lib/api/types'
  import type { TreeContext } from '../lib/tree'
  import { childEntries, deviceName, routerName } from '../lib/topology'
  import { sameId } from '../lib/ids'
  import { linkBitrate } from '../lib/link'
  import { USB4_ROUTER_SPEED, classToken, speedColorVar } from '../lib/colors'
  import { formatBitrate, formatThroughput, linkLabel } from '../lib/format'
  import { expanded } from '../lib/expanded.svelte'
  import { focus } from '../lib/focus.svelte'
  import { flash } from '../lib/actions'
  import { t } from '../lib/i18n.svelte'
  import ClassIcon from './ClassIcon.svelte'
  import LinkBadge from './LinkBadge.svelte'
  import SocketStrip from './SocketStrip.svelte'

  interface Props {
    node: GraphNode
    ctx: TreeContext
    /** True for a few seconds after an insight first names this device. */
    pulse: boolean
  }

  let { node, ctx, pulse }: Props = $props()

  const device = $derived(node.device ?? null)
  const isDevice = $derived(node.kind === 'device')
  // A box stands for several hubs at once, so its counts come from the node
  // rather than from any one device.
  const isBox = $derived(node.kind === 'box')
  const boxKind = $derived(node.enclosure?.kind ?? 'hub')
  const members = $derived(node.members ?? [])
  const usedSockets = $derived(node.sockets.filter((s) => s.occupied).length)
  const childCount = $derived(isBox ? usedSockets : device ? childEntries(device).length : 0)
  const portTotal = $derived(isBox ? node.sockets.length : (device?.hub?.port_count ?? 0))
  const collapsible = $derived(isBox ? childCount > 0 : isDevice && device?.hub !== undefined && childCount > 0)
  const toggleId = $derived(isBox ? node.id : device?.id)
  const open = $derived(toggleId ? expanded.isExpanded(toggleId) : true)
  const name = $derived(
    node.kind === 'controller' ? (node.controller?.name ?? '')
      : node.kind === 'router' ? (node.router ? routerName(node.router) : '')
        : node.kind === 'carried' ? (node.label ?? '')
          : isBox ? (node.enclosure?.name || (boxKind === 'host' ? t('box.host') : device ? deviceName(device) : t('box.unknown')))
            : device ? deviceName(device) : '',
  )

  // Insights can name hub-tree devices or USB4 routers; both ids are
  // normalised PnP ids. A box wears the worst flag of anything folded into
  // it, so a warning about a dock's third hub still shows on the dock.
  const RANK: Record<Severity, number> = { info: 0, warning: 1, critical: 2 }
  const flaggable = $derived(isDevice || node.kind === 'router')
  const severity = $derived.by((): Severity | null => {
    if (!isBox) return flaggable ? (ctx.flagged.get(node.id) ?? null) : null
    let worst: Severity | null = null
    for (const id of members) {
      const found = ctx.flagged.get(id)
      if (found && (worst === null || RANK[found] > RANK[worst])) worst = found
    }
    return worst
  })
  const focusId = $derived(device?.id ?? node.router?.id ?? null)
  const focused = $derived.by(() => {
    if (isBox) return members.some((id) => sameId(focus.id, id))
    return focusId !== null && sameId(focus.id, focusId)
  })
  const flashToken = $derived(focused ? focus.token : null)
  const accent = $derived(
    node.kind === 'controller' ? 'var(--accent-light)'
      : node.kind === 'router' ? speedColorVar(USB4_ROUTER_SPEED)
        : node.kind === 'carried' ? 'var(--class-storage)'
          : isBox ? (boxKind === 'host' ? 'var(--accent-light)' : 'var(--class-hub)')
            : `var(--class-${classToken(device?.class ?? 'unknown')})`,
  )

  // Live traffic through this node's uplink, scaled to what that uplink can carry.
  const uplinkSpeed = $derived(node.port?.negotiated_link ?? 'unknown')
  const capacity = $derived(node.kind === 'controller' ? (node.controller?.max_bandwidth ?? 0) : linkBitrate(uplinkSpeed))
  const capacityLabel = $derived(node.kind === 'controller' ? formatBitrate(capacity) : linkLabel(uplinkSpeed))
  const showTraffic = $derived(ctx.showMeter && node.bps > 0 && capacity > 0 && (isDevice || isBox || node.kind === 'controller'))
  const trafficPct = $derived(capacity > 0 ? Math.min(100, Math.round((node.bps / capacity) * 100)) : 0)
  const trafficTitle = $derived(t('graph.node.traffic', { rate: formatThroughput(node.bps), percent: `${trafficPct}%`, link: capacityLabel }))
  const trafficColor = $derived(node.kind === 'controller' ? 'var(--accent-light)' : speedColorVar(uplinkSpeed))

  const sockets = $derived(node.sockets)
  const hiddenLabel = $derived(node.hiddenCount === 1 ? t('graph.hidden.one') : t('graph.hidden', { n: node.hiddenCount }))
  const toggleLabel = $derived(open ? t('tree.collapse', { name }) : t('tree.expand', { name }))
</script>

<div
  class="node kind-{node.kind}"
  class:flagged={severity !== null}
  class:pulse
  class:has-sockets={sockets.length > 0}
  style:left={`${node.x}px`}
  style:top={`${node.y}px`}
  style:width={`${node.width}px`}
  style:height={`${node.height}px`}
  style:--node-accent={accent}
  style:--severity-color={severity ? `var(--severity-${severity})` : 'transparent'}
  data-device-id={focusId}
  data-node-id={node.id}
  title={name}
  use:flash={{ token: flashToken }}
>
  <span class="icon" aria-hidden="true">
    {#if node.kind === 'controller'}
      <Cpu size={16} />
    {:else if node.kind === 'router'}
      <Zap size={16} />
    {:else if node.kind === 'carried'}
      <HardDrive size={16} />
    {:else if isBox && boxKind === 'host'}
      <Laptop size={16} />
    {:else if isBox && boxKind === 'dock'}
      <Dock size={16} />
    {:else if device}
      <ClassIcon cls={device.class} size={16} />
    {/if}
  </span>

  <div class="main">
    <div class="name-line">
      <span class="name">{name}</span>
      {#if node.port && device}
        <span class="badge"><LinkBadge negotiated={node.port.negotiated_link} max={node.port.max_link} claimed={device.claimed_speed} /></span>
      {/if}
      {#if isBox && node.enclosure?.kind === 'dock'}
        <span class="tag">{t('box.tag.dock')}</span>
      {/if}
      {#if severity}
        <span class="flag sev-{severity}" title={t('tree.flagged')}><TriangleAlert size={13} aria-label={t('tree.flagged')} /></span>
      {/if}
    </div>
    <div class="meta">
      {#if node.kind === 'controller' && node.controller}
        <span>{t(`tree.controller.kind.${node.controller.kind}`)}</span>
        {#if device?.hub}<span>{t('tree.hub.ports', { used: childCount, total: device.hub.port_count })}</span>{/if}
      {:else if node.kind === 'router' && node.router}
        <span>{t(`tree.usb4.${node.router.kind}`)}</span>
        <span>{linkLabel(USB4_ROUTER_SPEED)}</span>
      {:else if node.kind === 'carried'}
        <span>{t('graph.carried')}</span>
      {:else if isBox}
        {#if boxKind === 'host'}
          <span>{members.length === 1 ? t('box.controllers.one') : t('box.controllers', { n: members.length })}</span>
        {:else if members.length > 1}
          <span>{t('box.folded', { n: members.length })}</span>
        {/if}
        <span>{t('box.sockets', { used: usedSockets, total: portTotal })}</span>
      {:else if device}
        {#if node.port}<span>{t('tree.port', { n: node.port.number })}</span>{/if}
        {#if device.hub}<span>{t('tree.hub.ports', { used: childCount, total: device.hub.port_count })}</span>{/if}
        {#if device.iso_reserved > 0}<span>{t('tree.iso', { rate: formatBitrate(device.iso_reserved) })}</span>{/if}
      {/if}
    </div>
    {#if collapsible && toggleId}
      <button class="toggle" aria-expanded={open} aria-label={toggleLabel} title={toggleLabel} onclick={() => expanded.toggle(toggleId)}>
        {#if open}<ChevronDown size={13} aria-hidden="true" />{:else}<ChevronRight size={13} aria-hidden="true" />{/if}
        {#if !open}<span class="hidden-count">{hiddenLabel}</span>{/if}
      </button>
    {/if}
  </div>

  {#if sockets.length > 0}
    <SocketStrip {sockets} />
  {/if}

  {#if showTraffic}
    <div class="traffic" role="meter" aria-valuemin={0} aria-valuemax={100} aria-valuenow={trafficPct} aria-label={trafficTitle} title={trafficTitle}>
      <div class="fill" style:width={`${trafficPct}%`} style:background={trafficColor}></div>
    </div>
  {/if}
</div>

<style>
  .node {
    position: absolute;
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-1) var(--space-2) var(--space-1) var(--space-2);
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-left: var(--stripe-width) solid var(--node-accent);
    border-radius: var(--radius-md);
    box-shadow: 0 1px 3px var(--shadow);
    overflow: hidden;
    transition: top var(--duration-slow) var(--ease-out), left var(--duration-slow) var(--ease-out), border-color var(--duration-fast);
  }
  .node:hover {
    border-color: var(--border-hover);
    border-left-color: var(--node-accent);
  }
  /* Leave room for the socket strip so it never sits on the name or badge. */
  .node.has-sockets {
    padding-right: 56px;
  }
  /* Tall boxes get a hairline down the inside of the strip so the empty
     middle still reads as one object rather than a gap. */
  .kind-box.has-sockets::after {
    content: '';
    position: absolute;
    top: var(--space-2);
    bottom: var(--space-2);
    right: 48px;
    border-right: 1px solid var(--border-muted);
  }
  .node.flagged {
    background: color-mix(in srgb, var(--severity-color) 12%, var(--bg-elevated));
    box-shadow: 0 0 0 1px color-mix(in srgb, var(--severity-color) 55%, transparent);
  }
  .node.pulse {
    animation: node-pulse 1.2s var(--ease-in-out) 4;
  }
  @keyframes node-pulse {
    0%, 100% { box-shadow: 0 0 0 1px color-mix(in srgb, var(--severity-color) 55%, transparent); }
    50% { box-shadow: 0 0 0 5px color-mix(in srgb, var(--severity-color) 35%, transparent); }
  }
  .kind-controller {
    background: color-mix(in srgb, var(--accent) 8%, var(--bg-elevated));
  }
  /* A box is a physical object, so it gets a heavier frame than a device. */
  .kind-box {
    border-width: 2px;
    border-left-width: var(--stripe-width);
    /* A box with many sockets is tall, and centring its name in all that
       empty space reads as a mistake. The face belongs at the top, beside
       the first sockets. */
    align-items: flex-start;
    padding-top: var(--space-2);
  }
  /* There is room on a wider card for the counts to wrap rather than clip. */
  .kind-box .meta {
    flex-wrap: wrap;
    white-space: normal;
    row-gap: 0;
  }
  .kind-box .meta span {
    overflow: visible;
  }
  .kind-carried {
    border-style: dashed;
    border-left-style: solid;
  }
  .icon {
    display: inline-flex;
    flex-shrink: 0;
    color: var(--node-accent);
  }
  .main {
    flex: 1;
    min-width: 0;
  }
  .name-line {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-width: 0;
  }
  .name {
    font-weight: 600;
    color: var(--text-strong);
    font-size: var(--font-size-sm);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    min-width: 0;
  }
  .badge,
  .flag {
    display: inline-flex;
    flex-shrink: 0;
  }
  /* Says the box came from the knowledge base rather than from the bus. */
  .tag {
    flex-shrink: 0;
    padding: 0 5px;
    border-radius: var(--radius-sm);
    background: var(--bg-subtle);
    color: var(--text-muted);
    font-size: var(--font-size-xs);
    font-weight: 600;
    line-height: 1.5;
  }
  .meta {
    display: flex;
    gap: var(--space-3);
    color: var(--text-muted);
    font-size: var(--font-size-xs);
    white-space: nowrap;
    overflow: hidden;
  }
  .meta span {
    overflow: hidden;
    text-overflow: ellipsis;
  }
  /* Bottom-left of the main column, out of the socket strip's way. */
  .toggle {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    margin-top: 1px;
    padding: 1px 4px 1px 2px;
    border: 0;
    border-radius: var(--radius-sm);
    background: var(--bg-subtle);
    color: var(--text-muted);
    font-size: var(--font-size-xs);
    line-height: 1;
  }
  .toggle:hover {
    background: var(--bg-subtle-hover);
    color: var(--text-strong);
  }
  .hidden-count {
    padding: 0 2px;
    font-weight: 600;
  }
  /* The traffic bar stops at the socket panel rather than running through it. */
  .node.has-sockets .traffic {
    right: 48px;
  }
  .traffic {
    position: absolute;
    left: 0;
    right: 0;
    bottom: 0;
    height: 3px;
    background: var(--meter-track);
  }
  .traffic .fill {
    height: 100%;
    transition: width var(--duration-slow) var(--ease-out);
  }
  @media (prefers-reduced-motion: reduce) {
    .node,
    .node.pulse,
    .traffic .fill {
      animation: none;
      transition: none;
    }
  }
</style>
