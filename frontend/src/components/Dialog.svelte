<script module lang="ts">
  let seq = 0
</script>

<script lang="ts">
  // A modal: one thing to settle before anything else. The backdrop and
  // Escape both close it, and focus lands on the first control so the
  // keyboard works from the start.
  import { X } from 'lucide-svelte'
  import type { Snippet } from 'svelte'
  import { t } from '../lib/i18n.svelte'

  interface Props {
    title: string
    onClose: () => void
    children: Snippet
    /** Buttons, right-aligned under the body. */
    footer?: Snippet
    /** Widest the panel gets; it shrinks to the viewport below that. */
    width?: string
  }

  let { title, onClose, children, footer, width = '520px' }: Props = $props()

  const titleId = `dialog-title-${++seq}`
  let panel = $state<HTMLElement | null>(null)

  $effect(() => {
    panel?.querySelector<HTMLElement>('input, select, textarea, button')?.focus()
  })

  function onKeydown(e: KeyboardEvent): void {
    if (e.key === 'Escape') {
      e.preventDefault()
      onClose()
    }
  }
</script>

<svelte:window onkeydown={onKeydown} />

<div class="overlay">
  <button class="backdrop" tabindex="-1" aria-label={t('dialog.close')} onclick={onClose}></button>
  <div class="panel" role="dialog" aria-modal="true" aria-labelledby={titleId} style:max-width={width} bind:this={panel}>
    <header>
      <h2 id={titleId}>{title}</h2>
      <button class="close" aria-label={t('dialog.close')} title={t('dialog.close')} onclick={onClose}>
        <X size={16} aria-hidden="true" />
      </button>
    </header>
    <div class="body">
      {@render children()}
    </div>
    {#if footer}
      <footer>{@render footer()}</footer>
    {/if}
  </div>
</div>

<style>
  .overlay {
    position: fixed;
    inset: 0;
    z-index: 30;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: var(--space-4);
  }
  .backdrop {
    position: absolute;
    inset: 0;
    border: 0;
    padding: 0;
    background: var(--bg-overlay);
    cursor: default;
  }
  .panel {
    position: relative;
    width: 100%;
    max-height: calc(100vh - 2 * var(--space-4));
    display: flex;
    flex-direction: column;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    box-shadow: 0 12px 40px var(--shadow);
  }
  header {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-3) var(--space-4);
    border-bottom: 1px solid var(--border-muted);
  }
  h2 {
    flex: 1;
    font-size: var(--font-size-lg);
    font-weight: 600;
    color: var(--text-strong);
  }
  .close {
    display: inline-flex;
    padding: 4px;
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-muted);
  }
  .close:hover,
  .close:focus-visible {
    background: var(--bg-subtle-hover);
    color: var(--text-strong);
  }
  /* The body is the one part that scrolls, so long lists never push the buttons off screen. */
  .body {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    padding: var(--space-4);
  }
  footer {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
    padding: var(--space-3) var(--space-4);
    border-top: 1px solid var(--border-muted);
  }
</style>
