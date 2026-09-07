<script lang="ts">
  import type { ConnectionState } from '../lib/connection'
  import { POLL_INTERVAL_MS } from '../lib/live.svelte'
  import { t } from '../lib/i18n.svelte'

  interface Props {
    state: ConnectionState
  }

  let { state }: Props = $props()

  const title = $derived(t(`connection.badge.title.${state}`, { seconds: POLL_INTERVAL_MS / 1000 }))
</script>

<span class="badge state-{state}" {title} role="status" aria-live="polite">
  <span class="dot" aria-hidden="true"></span>
  {t(`connection.${state}`)}
</span>

<style>
  .badge {
    --dot: var(--conn-connecting);
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 3px 10px;
    border: 1px solid var(--border);
    border-radius: 999px;
    font-size: var(--font-size-sm);
    font-weight: 600;
    color: var(--text-strong);
    background: var(--bg-subtle);
  }
  .dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--dot);
  }
  .state-live { --dot: var(--conn-live); }
  .state-polling { --dot: var(--conn-polling); }
  .state-offline { --dot: var(--conn-offline); }
  .state-live .dot {
    box-shadow: 0 0 0 3px color-mix(in srgb, var(--conn-live) 25%, transparent);
  }
</style>
