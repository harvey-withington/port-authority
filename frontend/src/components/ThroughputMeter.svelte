<script lang="ts">
  import { ArrowDown, ArrowUp } from 'lucide-svelte'
  import type { LinkSpeed, ThroughputSample } from '../lib/api/types'
  import { linkBitrate } from '../lib/link'
  import { formatThroughput, linkLabel } from '../lib/format'
  import { speedColorVar } from '../lib/colors'
  import { t } from '../lib/i18n.svelte'

  interface Props {
    /** Latest sample, or null when the device has been idle for a while. */
    sample: ThroughputSample | null
    /** Negotiated link; the bar is scaled so 100% means this link is saturated. */
    speed: LinkSpeed
    /** False when the provider cannot measure throughput at all. */
    enabled: boolean
  }

  let { sample, speed, enabled }: Props = $props()

  const linkBps = $derived(linkBitrate(speed))
  const read = $derived(sample?.read_bps ?? 0)
  const write = $derived(sample?.write_bps ?? 0)
  const pct = (bps: number): number => (linkBps > 0 ? Math.min(100, (bps / linkBps) * 100) : 0)
  const readPct = $derived(pct(read))
  const writePct = $derived(Math.min(100 - readPct, pct(write)))
  const active = $derived(read + write > 0)
  const title = $derived(
    !enabled
      ? t('meter.unavailable')
      : active
        ? t('meter.title', {
            read: formatThroughput(read),
            write: formatThroughput(write),
            percent: `${Math.round(readPct + writePct)}%`,
            link: linkLabel(speed),
          })
        : t('meter.idle'),
  )
</script>

<div class="meter" class:active class:disabled={!enabled} {title}>
  <div class="bar" role="meter" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(readPct + writePct)} aria-label={title}>
    <div class="fill read" style:width={`${readPct}%`} style:background={speedColorVar(speed)}></div>
    <div class="fill write" style:width={`${writePct}%`} style:background={speedColorVar(speed)}></div>
  </div>
  <span class="values">
    {#if active}
      <span class="value"><ArrowDown size={11} aria-label={t('meter.read')} />{formatThroughput(read)}</span>
      <span class="value"><ArrowUp size={11} aria-label={t('meter.write')} />{formatThroughput(write)}</span>
    {:else}
      <span class="value idle">{enabled ? formatThroughput(0) : t('meter.unavailable')}</span>
    {/if}
  </span>
</div>

<style>
  .meter {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin-top: var(--space-1);
    font-size: var(--font-size-xs);
    color: var(--text-secondary);
    font-variant-numeric: tabular-nums;
  }
  .bar {
    flex: 0 1 160px;
    height: var(--meter-height);
    background: var(--meter-track);
    border-radius: var(--meter-height);
    overflow: hidden;
    display: flex;
  }
  .fill {
    height: 100%;
    transition: width var(--duration-slow) var(--ease-out);
  }
  .fill.write {
    opacity: 0.55;
  }
  .values {
    display: inline-flex;
    gap: var(--space-2);
  }
  .value {
    display: inline-flex;
    align-items: center;
    gap: 2px;
  }
  .idle {
    color: var(--text-muted);
  }
  .disabled .bar {
    opacity: 0.4;
  }
</style>
