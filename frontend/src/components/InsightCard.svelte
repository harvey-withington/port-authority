<script lang="ts">
  import { ChevronDown, ChevronRight, CircleAlert, Info, OctagonAlert } from 'lucide-svelte'
  import type { Insight } from '../lib/api/types'
  import type { DeviceIndex } from '../lib/topology'
  import { formatConfidence } from '../lib/format'
  import { t } from '../lib/i18n.svelte'
  import DeviceName from './DeviceName.svelte'

  interface Props {
    insight: Insight
    index: DeviceIndex
    onFocusDevice: (id: string) => void
  }

  let { insight, index, onFocusDevice }: Props = $props()

  let open = $state(false)

  const deviceIds = $derived(insight.device_ids ?? [])
  const evidence = $derived(insight.evidence ?? [])
  const Icon = $derived(insight.severity === 'critical' ? OctagonAlert : insight.severity === 'warning' ? CircleAlert : Info)
</script>

<article class="card sev-{insight.severity}" style:--stripe={`var(--severity-${insight.severity})`}>
  <header>
    <span class="icon" aria-label={t(`insight.severity.${insight.severity}`)} title={t(`insight.severity.${insight.severity}`)}>
      <Icon size={16} />
    </span>
    <h3>{insight.title}</h3>
  </header>
  <p class="explanation">{insight.explanation}</p>
  {#if insight.suggestion}
    <p class="suggestion"><strong>{t('insight.suggestion')}:</strong> {insight.suggestion}</p>
  {/if}
  {#if deviceIds.length > 0}
    <p class="devices">
      <span class="label">{t('insight.devices')}:</span>
      {#each deviceIds as id (id)}
        <DeviceName {id} {index} onFocus={onFocusDevice} />
      {/each}
    </p>
  {/if}
  <button class="details-toggle" aria-expanded={open} onclick={() => (open = !open)}>
    {#if open}<ChevronDown size={13} />{:else}<ChevronRight size={13} />{/if}
    {open ? t('insight.details.hide') : t('insight.details.show')}
  </button>
  {#if open}
    <div class="details">
      {#if evidence.length > 0}
        <h4>{t('insight.evidence')}</h4>
        <ul>
          {#each evidence as line (line)}<li>{line}</li>{/each}
        </ul>
      {/if}
      <p class="meta">
        <span>{t('insight.confidence', { percent: formatConfidence(insight.confidence) })}</span>
        <span class="mono">{t('insight.rule', { rule: insight.rule_id })}</span>
      </p>
    </div>
  {/if}
</article>

<style>
  .card {
    position: relative;
    padding: var(--space-3) var(--space-3) var(--space-3) calc(var(--space-3) + var(--stripe-width));
    background: var(--bg-elevated);
    border: 1px solid var(--border-muted);
    border-radius: var(--radius-md);
    overflow: hidden;
  }
  .card::before {
    content: '';
    position: absolute;
    left: 0;
    top: 0;
    bottom: 0;
    width: var(--stripe-width);
    background: var(--stripe);
  }
  header {
    display: flex;
    align-items: flex-start;
    gap: var(--space-2);
  }
  .icon {
    display: inline-flex;
    margin-top: 2px;
    flex-shrink: 0;
  }
  h3 {
    font-size: var(--font-size-md);
    font-weight: 600;
    color: var(--text-primary);
  }
  .explanation {
    margin-top: var(--space-2);
    color: var(--text-body);
  }
  .suggestion {
    margin-top: var(--space-2);
    color: var(--text-strong);
  }
  .devices {
    margin-top: var(--space-2);
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    font-size: var(--font-size-sm);
  }
  .label {
    color: var(--text-muted);
  }
  .details-toggle {
    margin-top: var(--space-2);
    display: inline-flex;
    align-items: center;
    gap: 3px;
    padding: 2px 6px 2px 2px;
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-secondary);
    font-size: var(--font-size-sm);
  }
  .details-toggle:hover {
    background: var(--bg-subtle-hover);
    color: var(--text-strong);
  }
  .details {
    margin-top: var(--space-2);
    padding: var(--space-2) var(--space-3);
    background: var(--bg-subtle);
    border-radius: var(--radius-sm);
    font-size: var(--font-size-sm);
  }
  h4 {
    margin-bottom: var(--space-1);
    font-size: var(--font-size-xs);
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-muted);
  }
  .details ul {
    list-style: disc;
    padding-left: var(--space-4);
    color: var(--text-body);
  }
  .meta {
    display: flex;
    gap: var(--space-3);
    margin-top: var(--space-2);
    color: var(--text-muted);
    font-size: var(--font-size-xs);
  }
</style>
