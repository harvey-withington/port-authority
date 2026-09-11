<script lang="ts">
  // The question before anything is thrown away. Never the browser's own
  // confirm(): this one says what will happen, in the app's words.
  import { t } from '../lib/i18n.svelte'
  import Dialog from './Dialog.svelte'

  interface Props {
    title: string
    body: string
    confirmLabel: string
    /** Paints the confirm button in the danger colour. */
    danger?: boolean
    /** True while the action runs; both buttons wait. */
    busy?: boolean
    /** Why the last attempt failed, shown above the buttons. */
    error?: string | null
    onConfirm: () => void
    onCancel: () => void
  }

  let { title, body, confirmLabel, danger = false, busy = false, error = null, onConfirm, onCancel }: Props = $props()
</script>

<Dialog {title} onClose={onCancel} width="440px">
  <p class="body">{body}</p>
  {#if error}
    <p class="error" role="alert">{error}</p>
  {/if}
  {#snippet footer()}
    <button class="btn" onclick={onCancel} disabled={busy}>{t('dialog.cancel')}</button>
    <button class="btn primary" class:danger onclick={onConfirm} disabled={busy}>{confirmLabel}</button>
  {/snippet}
</Dialog>

<style>
  .body {
    color: var(--text-body);
  }
  .error {
    margin-top: var(--space-3);
    color: var(--danger);
    font-size: var(--font-size-sm);
  }
</style>
