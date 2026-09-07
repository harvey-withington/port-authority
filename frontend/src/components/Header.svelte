<script lang="ts">
  import { Palette, Usb } from 'lucide-svelte'
  import type { ProviderCaps } from '../lib/api/types'
  import type { ConnectionState } from '../lib/connection'
  import { t } from '../lib/i18n.svelte'
  import CapabilityChips from './CapabilityChips.svelte'
  import ConnectionBadge from './ConnectionBadge.svelte'

  interface Props {
    appName: string
    version: string | null
    state: ConnectionState
    capabilities: ProviderCaps | null
    legendOpen: boolean
    onToggleLegend: () => void
  }

  let { appName, version, state, capabilities, legendOpen, onToggleLegend }: Props = $props()
</script>

<header class="header">
  <div class="brand">
    <Usb size={18} aria-hidden="true" />
    <h1>{appName}</h1>
    {#if version}<span class="version mono">{t('header.version', { version })}</span>{/if}
    <span class="tagline">{t('app.tagline')}</span>
  </div>
  <div class="right">
    <CapabilityChips {capabilities} />
    <ConnectionBadge {state} />
    <button class="legend-toggle" aria-pressed={legendOpen} onclick={onToggleLegend} title={legendOpen ? t('header.legend.hide') : t('header.legend')}>
      <Palette size={14} aria-hidden="true" />
      {t('header.legend')}
    </button>
  </div>
</header>

<style>
  .header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    flex-wrap: wrap;
    min-height: var(--header-height);
    padding: var(--space-2) var(--space-4);
    background: var(--bg-surface);
    border-bottom: 1px solid var(--border-muted);
  }
  .brand {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    color: var(--accent-light);
  }
  h1 {
    font-size: var(--font-size-lg);
    font-weight: 700;
    color: var(--text-primary);
  }
  .version {
    color: var(--text-muted);
    font-size: var(--font-size-xs);
  }
  .tagline {
    color: var(--text-muted);
    font-size: var(--font-size-sm);
  }
  .right {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    flex-wrap: wrap;
  }
  .legend-toggle {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 3px 10px;
    border: 1px solid var(--border);
    border-radius: 999px;
    background: transparent;
    color: var(--text-secondary);
    font-size: var(--font-size-sm);
  }
  .legend-toggle:hover,
  .legend-toggle[aria-pressed='true'] {
    color: var(--text-strong);
    border-color: var(--border-hover);
    background: var(--bg-subtle);
  }
  @media (max-width: 900px) {
    .tagline { display: none; }
  }
</style>
