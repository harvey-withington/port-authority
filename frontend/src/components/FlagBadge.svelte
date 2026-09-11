<script lang="ts">
  // The mark on a device an insight names. It is a button: pressing it
  // opens the findings panel narrowed to that device, so the sign leads
  // straight to the text behind it.
  import type { Severity } from '../lib/api/types'
  import { findingFilter } from '../lib/findingFilter.svelte'
  import { t } from '../lib/i18n.svelte'
  import SeverityIcon from './SeverityIcon.svelte'

  interface Props {
    severity: Severity
    /** The ids the findings name: one device, or the flagged members of a box. */
    ids: readonly string[]
    /** What the panel's filter calls this device or box. */
    name: string
    size?: number
  }

  let { severity, ids, name, size = 13 }: Props = $props()

  const label = $derived(t(`flag.${severity}`, { name }))
</script>

<button class="flag sev-{severity}" title={label} aria-label={label} onclick={() => findingFilter.show(ids, name)}>
  <SeverityIcon {severity} {size} />
</button>

<style>
  .flag {
    display: inline-flex;
    flex-shrink: 0;
    padding: 1px;
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
  }
  .flag:hover,
  .flag:focus-visible {
    background: var(--bg-subtle-hover);
  }
</style>
