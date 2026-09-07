<script lang="ts">
  import { CircleCheck } from 'lucide-svelte'
  import type { Insight } from '../lib/api/types'
  import type { DeviceIndex } from '../lib/topology'
  import { insightKey } from '../lib/insights'
  import { t } from '../lib/i18n.svelte'
  import InsightCard from './InsightCard.svelte'

  interface Props {
    insights: Insight[]
    index: DeviceIndex
    onFocusDevice: (id: string) => void
  }

  let { insights, index, onFocusDevice }: Props = $props()
</script>

{#if insights.length === 0}
  <div class="empty">
    <span class="icon" aria-hidden="true"><CircleCheck size={22} /></span>
    <p class="title">{t('insights.empty.title')}</p>
    <p class="body">{t('insights.empty.body')}</p>
  </div>
{:else}
  <ul class="list" aria-label={insights.length === 1 ? t('insights.count.one') : t('insights.count', { n: insights.length })}>
    {#each insights as insight (insightKey(insight))}
      <li><InsightCard {insight} {index} {onFocusDevice} /></li>
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
    color: var(--success);
    display: inline-flex;
    margin-bottom: var(--space-1);
  }
  .title {
    font-weight: 600;
    color: var(--text-strong);
  }
  .body {
    color: var(--text-muted);
    font-size: var(--font-size-sm);
  }
  .list {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
</style>
