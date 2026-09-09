<script lang="ts">
  // Physical or logical: whether the diagram draws the boxes on the desk or
  // the controllers and hubs the OS reports. Sits next to ViewSwitch, which
  // chooses the rendering rather than what is in it.
  import { Boxes, Network } from 'lucide-svelte'
  import type { DetailLevel } from '../lib/view.svelte'
  import { t } from '../lib/i18n.svelte'

  interface Props {
    value: DetailLevel
    onChange: (next: DetailLevel) => void
  }

  let { value, onChange }: Props = $props()

  const OPTIONS: readonly DetailLevel[] = ['physical', 'logical']
</script>

<div class="switch" role="group" aria-label={t('detail.label')}>
  {#each OPTIONS as option (option)}
    <button
      class:active={option === value}
      aria-pressed={option === value}
      title={t(`detail.${option}.hint`)}
      onclick={() => onChange(option)}
    >
      {#if option === 'physical'}<Boxes size={13} aria-hidden="true" />{:else}<Network size={13} aria-hidden="true" />{/if}
      {t(`detail.${option}`)}
    </button>
  {/each}
</div>

<style>
  .switch {
    display: inline-flex;
    padding: 2px;
    border: 1px solid var(--border);
    border-radius: 999px;
    background: var(--bg-surface);
  }
  button {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 2px 9px;
    border: 0;
    border-radius: 999px;
    background: transparent;
    color: var(--text-muted);
    font-size: var(--font-size-xs);
    font-weight: 600;
    text-transform: none;
    letter-spacing: 0;
  }
  button:hover {
    color: var(--text-strong);
  }
  button.active {
    background: var(--bg-elevated);
    color: var(--text-strong);
    box-shadow: 0 1px 2px var(--shadow);
  }
</style>
