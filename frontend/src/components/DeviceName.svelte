<script lang="ts">
  import type { DeviceIndex } from '../lib/topology'
  import { refName } from '../lib/topology'
  import { normalizeId } from '../lib/ids'
  import { classToken } from '../lib/colors'
  import { t } from '../lib/i18n.svelte'

  interface Props {
    /** Device id as the API gave it (any case). */
    id: string
    index: DeviceIndex
    /** When given and the device is in the snapshot, the name becomes a button. */
    onFocus?: (id: string) => void
    /** Shown when the id is not in the snapshot (e.g. a removed device). */
    fallback?: string
  }

  let { id, index, onFocus, fallback }: Props = $props()

  const ref = $derived(index.get(normalizeId(id)) ?? null)
  const name = $derived(ref ? refName(ref) : (fallback ?? id))
  // A router has no USB class; it takes the accent the diagram gives it.
  const color = $derived(
    ref?.device ? `var(--class-${classToken(ref.device.class)})`
      : ref?.router ? 'var(--speed-usb4_40)'
        : 'var(--text-muted)',
  )
</script>

{#if ref && onFocus}
  <button class="device-name link" style:color={color} title={t('insight.showDevice', { name })} onclick={() => onFocus(id)}>
    {name}
  </button>
{:else}
  <span class="device-name" style:color={color} title={ref ? id : t('device.unresolved')}>{name}</span>
{/if}

<style>
  .device-name {
    font-weight: 600;
    background: none;
    border: 0;
    padding: 0;
    font-size: inherit;
    text-align: left;
  }
  .link {
    text-decoration: underline dotted;
    text-underline-offset: 2px;
    border-radius: var(--radius-sm);
  }
  .link:hover,
  .link:focus-visible {
    background: var(--bg-subtle-hover);
    outline: none;
  }
</style>
