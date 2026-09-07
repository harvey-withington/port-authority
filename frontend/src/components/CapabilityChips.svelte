<script lang="ts">
  import { Activity, Info, Plug } from 'lucide-svelte'
  import type { ProviderCaps } from '../lib/api/types'
  import { clickOutside } from '../lib/actions'
  import { t } from '../lib/i18n.svelte'

  interface Props {
    capabilities: ProviderCaps | null
  }

  let { capabilities }: Props = $props()

  let hintOpen = $state(false)

  const hotplug = $derived(capabilities?.hotplug ?? false)
  const throughput = $derived(capabilities?.throughput ?? false)
</script>

{#if capabilities}
  <div class="chips">
    <span class="chip" class:on={hotplug} title={t(`caps.hotplug.title.${hotplug ? 'on' : 'off'}`)}>
      <Plug size={12} aria-hidden="true" />
      {t(`caps.hotplug.${hotplug ? 'on' : 'off'}`)}
    </span>
    {#if throughput}
      <span class="chip on" title={t('caps.throughput.title.on')}>
        <Activity size={12} aria-hidden="true" />
        {t('caps.throughput.on')}
      </span>
    {:else}
      <span class="hint-anchor" use:clickOutside={() => (hintOpen = false)}>
        <button class="chip hint-button" aria-expanded={hintOpen} onclick={() => (hintOpen = !hintOpen)}>
          <Activity size={12} aria-hidden="true" />
          {t('caps.throughput.off')}
          <Info size={12} aria-hidden="true" />
        </button>
        {#if hintOpen}
          <div class="hint" role="note">{t('caps.throughput.hint')}</div>
        {/if}
      </span>
    {/if}
  </div>
{/if}

<style>
  .chips {
    display: flex;
    gap: var(--space-2);
    align-items: center;
    flex-wrap: wrap;
  }
  .chip {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 2px 8px;
    border-radius: 999px;
    border: 1px solid var(--border);
    background: transparent;
    color: var(--text-muted);
    font-size: var(--font-size-xs);
    font-weight: 500;
    white-space: nowrap;
  }
  .chip.on {
    color: var(--success-text);
    background: var(--success-bg);
    border-color: transparent;
  }
  .hint-anchor {
    position: relative;
  }
  .hint-button:hover,
  .hint-button:focus-visible {
    border-color: var(--border-hover);
    color: var(--text-strong);
    outline: none;
  }
  .hint {
    position: absolute;
    top: calc(100% + 6px);
    right: 0;
    z-index: 10;
    width: 280px;
    padding: var(--space-2) var(--space-3);
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    box-shadow: 0 6px 20px var(--shadow-lg);
    color: var(--text-body);
    font-size: var(--font-size-sm);
    white-space: normal;
  }
</style>
