<script lang="ts">
  import { CircleCheck, History, Plug, Radio, RefreshCw, Sparkles, Unplug, Link } from 'lucide-svelte'
  import type { TimelineEntry, TimelineKind } from '../lib/timeline'
  import type { DeviceIndex } from '../lib/topology'
  import { clockTime, relativeTime } from '../lib/format'
  import { t } from '../lib/i18n.svelte'
  import DeviceName from './DeviceName.svelte'

  interface Props {
    entries: TimelineEntry[]
    index: DeviceIndex
    onFocusDevice: (id: string) => void
  }

  let { entries, index, onFocusDevice }: Props = $props()

  // Relative times drift; re-render every 10 s.
  let now = $state(Date.now())
  $effect(() => {
    const timer = setInterval(() => (now = Date.now()), 10_000)
    return () => clearInterval(timer)
  })

  const ICONS: Record<TimelineKind, typeof Plug> = {
    device_added: Plug,
    device_removed: Unplug,
    link_changed: Link,
    resnapshot: RefreshCw,
    insight_added: Sparkles,
    insight_resolved: CircleCheck,
    connection: Radio,
  }

  const isDeviceKind = (k: TimelineKind): boolean => k === 'device_added' || k === 'device_removed' || k === 'link_changed'

  function label(e: TimelineEntry): string {
    switch (e.kind) {
      case 'connection':
        return t(`timeline.connection.${e.connection ?? 'connecting'}`)
      case 'resnapshot':
        return t('timeline.resnapshot', { n: e.detail ?? '' })
      case 'insight_added':
      case 'insight_resolved':
        return t(`timeline.${e.kind}`, { title: e.subject })
      default:
        return t(`timeline.${e.kind}`, { name: e.subject || t('timeline.unknownDevice') })
    }
  }

  function toneClass(e: TimelineEntry): string {
    if (e.kind === 'connection') return e.connection === 'live' ? 'tone-good' : e.connection === 'offline' ? 'tone-bad' : 'tone-warn'
    if (e.kind === 'insight_added') return `sev-${e.severity ?? 'info'}`
    if (e.kind === 'insight_resolved') return 'tone-good'
    if (e.kind === 'device_removed') return 'tone-warn'
    return 'tone-neutral'
  }
</script>

{#if entries.length === 0}
  <p class="empty"><History size={14} aria-hidden="true" /> {t('timeline.empty')}</p>
{:else}
  <ol class="timeline">
    {#each entries as entry (entry.id)}
      {@const Icon = ICONS[entry.kind]}
      <li class="entry {toneClass(entry)}">
        <span class="icon" aria-hidden="true"><Icon size={13} /></span>
        <span class="text">
          {#if isDeviceKind(entry.kind) && entry.deviceId}
            <DeviceName id={entry.deviceId} {index} onFocus={onFocusDevice} fallback={entry.subject || t('timeline.unknownDevice')} />
            <span class="rest">{label({ ...entry, subject: '' }).trim()}</span>
          {:else}
            {label(entry)}
          {/if}
          {#if entry.detail && entry.kind !== 'resnapshot'}<span class="detail">{entry.detail}</span>{/if}
        </span>
        <time class="when" datetime={entry.at} title={clockTime(entry.at)}>{relativeTime(entry.at, now)}</time>
      </li>
    {/each}
  </ol>
{/if}

<style>
  .empty {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    color: var(--text-muted);
    font-size: var(--font-size-sm);
    padding: var(--space-2) 0;
  }
  .timeline {
    display: flex;
    flex-direction: column;
    padding: 0;
    margin: 0;
    list-style: none;
  }
  .entry {
    display: flex;
    align-items: baseline;
    gap: var(--space-2);
    padding: 5px 0;
    border-bottom: 1px solid var(--border-muted);
    font-size: var(--font-size-sm);
    color: var(--text-body);
  }
  .entry:last-child {
    border-bottom: 0;
  }
  .icon {
    display: inline-flex;
    align-self: center;
    flex-shrink: 0;
  }
  .tone-good .icon { color: var(--success); }
  .tone-warn .icon { color: var(--warning); }
  .tone-bad .icon { color: var(--danger); }
  .tone-neutral .icon { color: var(--text-muted); }
  .text {
    flex: 1;
    min-width: 0;
  }
  .detail {
    margin-left: var(--space-1);
    color: var(--text-muted);
    font-size: var(--font-size-xs);
  }
  .when {
    color: var(--text-muted);
    font-size: var(--font-size-xs);
    white-space: nowrap;
    font-variant-numeric: tabular-nums;
  }
</style>
