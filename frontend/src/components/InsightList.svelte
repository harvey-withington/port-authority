<script lang="ts">
  import { untrack } from 'svelte'
  import { CircleCheck, SearchX } from 'lucide-svelte'
  import type { Insight } from '../lib/api/types'
  import type { DeviceIndex } from '../lib/topology'
  import type { FindingFilter } from '../lib/findingFilter.svelte'
  import { insightKey, insightsMentioning } from '../lib/insights'
  import { findings } from '../lib/findings.svelte'
  import { t } from '../lib/i18n.svelte'
  import InsightCard from './InsightCard.svelte'

  interface Props {
    insights: Insight[]
    index: DeviceIndex
    /** When set, only the findings naming these devices are listed. */
    filter: FindingFilter | null
    onFocusDevice: (id: string) => void
    onClearFilter: () => void
  }

  let { insights, index, filter, onFocusDevice, onClearFilter }: Props = $props()

  const shown = $derived(filter ? insightsMentioning(insights, filter.ids) : insights)
  // Every listed card flashes when a flag is clicked; only the first one
  // scrolls, so the panel lands at the top of the answer, not its end.
  const flashToken = $derived(filter?.token ?? null)

  // A flag click means "show me", which beats a headline the user folded
  // earlier, the way focusing a device expands the hubs above it. Keyed on
  // the token alone, so a card re-folded while the filter is on stays so.
  $effect(() => {
    if (flashToken === null) return
    for (const insight of untrack(() => shown)) findings.open(insightKey(insight))
  })
</script>

{#if insights.length === 0}
  <div class="empty">
    <span class="icon ok" aria-hidden="true"><CircleCheck size={22} /></span>
    <p class="title">{t('insights.empty.title')}</p>
    <p class="body">{t('insights.empty.body')}</p>
  </div>
{:else if filter && shown.length === 0}
  <div class="empty">
    <span class="icon" aria-hidden="true"><SearchX size={22} /></span>
    <p class="title">{t('insights.filtered.empty', { name: filter.name })}</p>
    <button class="show-all" onclick={onClearFilter}>{t('findings.filter.clear')}</button>
  </div>
{:else}
  <ul class="list" aria-label={shown.length === 1 ? t('insights.count.one') : t('insights.count', { n: shown.length })}>
    {#each shown as insight, i (insightKey(insight))}
      <li><InsightCard {insight} {index} {onFocusDevice} {flashToken} scrollOnFlash={i === 0} /></li>
    {/each}
  </ul>
{/if}

<style>
  .empty {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--space-1);
    padding: var(--space-5) var(--space-4);
    text-align: center;
    border: 1px dashed var(--border);
    border-radius: var(--radius-md);
  }
  .icon {
    color: var(--text-muted);
    display: inline-flex;
    margin-bottom: var(--space-1);
  }
  .icon.ok {
    color: var(--success);
  }
  .title {
    font-weight: 600;
    color: var(--text-strong);
  }
  .body {
    color: var(--text-muted);
    font-size: var(--font-size-sm);
  }
  .show-all {
    margin-top: var(--space-2);
    padding: var(--space-1) var(--space-3);
    border: 1px solid var(--border);
    border-radius: 999px;
    background: var(--bg-elevated);
    color: var(--text-strong);
    font-size: var(--font-size-sm);
  }
  .show-all:hover {
    border-color: var(--border-hover);
  }
  .list {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
</style>
