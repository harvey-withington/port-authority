<script lang="ts">
  import { Usb } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'

  interface Props {
    /** Error reported by the host while starting, if any. */
    error: string | null
  }

  let { error }: Props = $props()
</script>

<div class="starting" role="status" aria-live="polite">
  <div class="logo" aria-hidden="true"><Usb size={36} /></div>
  <h1>{t('starting.title')}</h1>
  <p>{t('starting.body')}</p>
  {#if error}
    <p class="error" role="alert">{t('starting.error', { error })}</p>
  {/if}
</div>

<style>
  .starting {
    min-height: 100vh;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: var(--space-2);
    padding: var(--space-6);
    text-align: center;
    color: var(--text-secondary);
  }
  .logo {
    color: var(--accent-light);
    animation: pulse 1.6s var(--ease-in-out) infinite;
  }
  h1 {
    font-size: var(--font-size-xl);
    color: var(--text-primary);
  }
  .error {
    margin-top: var(--space-3);
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-md);
    background: var(--danger-bg);
    color: var(--danger-text);
    max-width: 520px;
  }
  @keyframes pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.4; }
  }
  @media (prefers-reduced-motion: reduce) {
    .logo { animation: none; }
  }
</style>
