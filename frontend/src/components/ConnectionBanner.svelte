<script lang="ts">
  import { RefreshCw, TriangleAlert, WifiOff } from 'lucide-svelte'
  import type { ConnectionState } from '../lib/connection'
  import { POLL_INTERVAL_MS } from '../lib/live.svelte'
  import { t } from '../lib/i18n.svelte'

  interface Props {
    state: ConnectionState
    error: string | null
    apiBase: string
    onRetry: () => void
  }

  let { state, error, apiBase, onRetry }: Props = $props()

  const degraded = $derived(state === 'polling' || state === 'offline')
  const visible = $derived(degraded || error !== null)
  const tone = $derived(state === 'offline' ? 'danger' : 'warning')
</script>

{#if visible}
  <div class="banner tone-{tone}" role="alert">
    <span class="icon" aria-hidden="true">
      {#if state === 'offline'}<WifiOff size={16} />{:else}<TriangleAlert size={16} />{/if}
    </span>
    <div class="text">
      {#if state === 'offline'}
        <strong>{t('banner.offline.title')}</strong>
        <span>{t('banner.offline.body', { base: apiBase })}</span>
      {:else if state === 'polling'}
        <strong>{t('banner.polling.title')}</strong>
        <span>{t('banner.polling.body', { seconds: POLL_INTERVAL_MS / 1000 })}</span>
      {/if}
      {#if error}<span class="error mono">{t('banner.error', { error })}</span>{/if}
    </div>
    <button class="retry" onclick={onRetry}>
      <RefreshCw size={13} aria-hidden="true" />
      {t('banner.retry')}
    </button>
  </div>
{/if}

<style>
  .banner {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    margin: var(--space-3) var(--space-4) 0;
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-md);
    border: 1px solid transparent;
    font-size: var(--font-size-sm);
  }
  .tone-warning {
    background: var(--warning-bg);
    color: var(--warning-text);
    border-color: color-mix(in srgb, var(--warning) 40%, transparent);
  }
  .tone-danger {
    background: var(--danger-bg);
    color: var(--danger-text);
    border-color: color-mix(in srgb, var(--danger) 40%, transparent);
  }
  .icon {
    display: inline-flex;
    flex-shrink: 0;
  }
  .text {
    display: flex;
    flex-direction: column;
    gap: 2px;
    flex: 1;
    min-width: 0;
  }
  .error {
    font-size: var(--font-size-xs);
    opacity: 0.85;
    word-break: break-word;
  }
  .retry {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 4px 10px;
    border-radius: var(--radius-sm);
    border: 1px solid currentColor;
    background: transparent;
    color: inherit;
    font-size: var(--font-size-sm);
    white-space: nowrap;
  }
  .retry:hover {
    background: var(--bg-subtle-hover);
  }
</style>
