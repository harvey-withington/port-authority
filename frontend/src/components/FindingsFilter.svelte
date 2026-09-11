<script lang="ts">
  // The findings panel's filter, in its header. A select rather than a
  // chip so the device can be changed here as well as by clicking another
  // flag, and "All findings" is always one choice away.
  import { X } from 'lucide-svelte'
  import type { Severity } from '../lib/api/types'
  import type { DeviceIndex } from '../lib/topology'
  import { nameForId } from '../lib/topology'
  import { idSetKey } from '../lib/ids'
  import { findingFilter } from '../lib/findingFilter.svelte'
  import { t } from '../lib/i18n.svelte'

  interface Props {
    /** Normalised device id -> worst severity, so every flagged device is offered. */
    flagged: ReadonlyMap<string, Severity>
    index: DeviceIndex
  }

  let { flagged, index }: Props = $props()

  interface Option {
    key: string
    ids: readonly string[]
    name: string
  }

  const current = $derived(findingFilter.current)
  const value = $derived(current ? idSetKey(current.ids) : '')
  const options = $derived.by((): Option[] => {
    const out: Option[] = [...flagged.keys()]
      .map((id) => ({ key: idSetKey([id]), ids: [id], name: nameForId(index, id) ?? id }))
      .sort((a, b) => a.name.localeCompare(b.name))
    // A box's flag stands for several devices, which no single-device
    // option covers, so the active filter is offered as its own entry.
    if (current && !out.some((o) => o.key === value)) out.unshift({ key: value, ids: current.ids, name: current.name })
    return out
  })

  function pick(key: string): void {
    const option = options.find((o) => o.key === key)
    if (option) findingFilter.show(option.ids, option.name)
    else findingFilter.clear()
  }
</script>

{#if options.length > 0}
  <div class="filter" class:active={current !== null}>
    <label class="showing" for="findings-filter">{t('findings.filter.showing')}</label>
    <select id="findings-filter" {value} onchange={(e) => pick(e.currentTarget.value)}>
      <option value="">{t('findings.filter.all')}</option>
      {#each options as option (option.key)}
        <option value={option.key}>{option.name}</option>
      {/each}
    </select>
    {#if current}
      <button class="clear" onclick={() => findingFilter.clear()} aria-label={t('findings.filter.clear')} title={t('findings.filter.clear')}>
        <X size={12} aria-hidden="true" />
      </button>
    {/if}
  </div>
{/if}

<style>
  .filter {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    padding: 2px 2px 2px var(--space-2);
    border: 1px solid var(--border);
    border-radius: 999px;
    background: var(--bg-surface);
    font-size: var(--font-size-xs);
    color: var(--text-muted);
  }
  /* Narrowed: the control takes the accent so the shorter list is
     visibly the filter's doing rather than the app's. */
  .filter.active {
    border-color: var(--accent);
    color: var(--text-secondary);
  }
  select {
    max-width: 200px;
    padding: 1px var(--space-1);
    border: 0;
    border-radius: 999px;
    background: transparent;
    color: var(--text-strong);
    font: inherit;
    font-weight: 600;
    text-overflow: ellipsis;
  }
  select:hover,
  select:focus-visible {
    background: var(--bg-elevated);
    outline: none;
  }
  .clear {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 18px;
    height: 18px;
    padding: 0;
    border: 0;
    border-radius: 999px;
    background: transparent;
    color: var(--text-secondary);
  }
  .clear:hover,
  .clear:focus-visible {
    background: var(--bg-elevated);
    color: var(--text-strong);
  }
</style>
