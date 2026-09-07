<script lang="ts">
  import { ChevronDown, ChevronRight, TriangleAlert } from 'lucide-svelte'
  import type { Device, Port } from '../lib/api/types'
  import type { TreeContext } from '../lib/tree'
  import { childEntries, deviceName } from '../lib/topology'
  import { normalizeId, sameId } from '../lib/ids'
  import { isLinkKnown } from '../lib/link'
  import { formatBitrate, formatPower, linkDescription } from '../lib/format'
  import { classToken } from '../lib/colors'
  import { sampleFor } from '../lib/throughput'
  import { expanded } from '../lib/expanded.svelte'
  import { focus } from '../lib/focus.svelte'
  import { flash } from '../lib/actions'
  import { t } from '../lib/i18n.svelte'
  import ClassIcon from './ClassIcon.svelte'
  import LinkBadge from './LinkBadge.svelte'
  import ThroughputMeter from './ThroughputMeter.svelte'
  import DeviceNode from './DeviceNode.svelte'

  interface Props {
    device: Device
    /** The hub port the device is plugged into; null for a root hub. */
    port: Port | null
    depth: number
    ctx: TreeContext
  }

  let { device, port, depth, ctx }: Props = $props()

  const name = $derived(deviceName(device))
  const isHub = $derived(device.hub !== undefined)
  const children = $derived(childEntries(device))
  const open = $derived(expanded.isExpanded(device.id))
  const severity = $derived(ctx.flagged.get(normalizeId(device.id)) ?? null)
  const sample = $derived(sampleFor(ctx.throughput, device.id))
  const flashToken = $derived(sameId(focus.id, device.id) ? focus.token : null)
  const classColor = $derived(`var(--class-${classToken(device.class)})`)
  const severityColor = $derived(severity ? `var(--severity-${severity})` : 'transparent')
  const showClaim = $derived(port !== null && isLinkKnown(device.claimed_speed) && device.claimed_speed !== port.negotiated_link)
  const showMeter = $derived(!isHub && (ctx.showMeter || sample !== null))
</script>

<li class="device" data-device-id={device.id}>
  <div
    class="row"
    class:flagged={severity !== null}
    style:--class-color={classColor}
    style:--severity-color={severityColor}
    use:flash={{ token: flashToken }}
  >
    {#if isHub && children.length > 0}
      <button
        class="toggle"
        aria-expanded={open}
        aria-label={open ? t('tree.collapse', { name }) : t('tree.expand', { name })}
        onclick={() => expanded.toggle(device.id)}
      >
        {#if open}<ChevronDown size={14} />{:else}<ChevronRight size={14} />{/if}
      </button>
    {:else}
      <span class="toggle spacer" aria-hidden="true"></span>
    {/if}

    <ClassIcon cls={device.class} />

    <div class="main">
      <div class="name-line">
        <span class="name">{name}</span>
        {#if port}
          <LinkBadge negotiated={port.negotiated_link} max={port.max_link} claimed={device.claimed_speed} />
        {/if}
        {#if severity}
          <span class="flag sev-{severity}" title={t('tree.flagged')}><TriangleAlert size={13} aria-label={t('tree.flagged')} /></span>
        {/if}
      </div>
      <div class="meta">
        {#if port}<span>{t('tree.port', { n: port.number })}</span>{/if}
        {#if device.hub}
          <span>{t('tree.hub.ports', { used: children.length, total: device.hub.port_count })}</span>
          {#if device.hub.bus_powered}<span>{t('tree.hub.busPowered')}</span>{/if}
        {/if}
        {#if device.power_draw_ma > 0}<span>{formatPower(device.power_draw_ma)}</span>{/if}
        {#if device.iso_reserved > 0}<span>{t('tree.iso', { rate: formatBitrate(device.iso_reserved) })}</span>{/if}
        {#if showClaim}<span>{t('tree.claimed', { speed: linkDescription(device.claimed_speed) })}</span>{/if}
        {#if device.serial_number}<span class="mono">{t('tree.serial', { serial: device.serial_number })}</span>{/if}
      </div>
      {#if showMeter}
        <ThroughputMeter {sample} speed={port?.negotiated_link ?? 'unknown'} enabled={ctx.showMeter} />
      {/if}
    </div>
  </div>

  {#if isHub && open && children.length > 0}
    <ul class="children" style:--depth={depth + 1}>
      {#each children as child (child.device.id)}
        <DeviceNode device={child.device} port={child.port} depth={depth + 1} {ctx} />
      {/each}
    </ul>
  {/if}
</li>

<style>
  .row {
    display: flex;
    align-items: flex-start;
    gap: var(--space-2);
    padding: var(--space-1) var(--space-2) var(--space-1) var(--space-1);
    border-left: var(--stripe-width) solid var(--class-color);
    border-radius: var(--radius-sm);
    transition: background var(--duration-fast);
  }
  .row:hover {
    background: var(--bg-subtle);
  }
  .row.flagged {
    background: color-mix(in srgb, var(--severity-color) 10%, transparent);
    box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--severity-color) 45%, transparent);
  }
  .toggle {
    width: 18px;
    height: 18px;
    flex-shrink: 0;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    padding: 0;
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-muted);
    margin-top: 1px;
  }
  .toggle:hover:not(.spacer) {
    background: var(--bg-subtle-hover);
    color: var(--text-strong);
  }
  .main {
    flex: 1;
    min-width: 0;
  }
  .name-line {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-wrap: wrap;
  }
  .name {
    font-weight: 600;
    color: var(--text-strong);
  }
  .flag {
    display: inline-flex;
  }
  .meta {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1) var(--space-3);
    color: var(--text-muted);
    font-size: var(--font-size-xs);
  }
  .children {
    margin-left: var(--tree-indent);
    border-left: 1px dashed var(--border-muted);
    padding-left: var(--space-1);
  }
</style>
