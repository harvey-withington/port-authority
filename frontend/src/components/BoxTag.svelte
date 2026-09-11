<script lang="ts">
  // The little word on a box saying where its grouping came from: "dock"
  // from the knowledge base, "your dock" when the user set it up here, in
  // which case it can also be forgotten again.
  import { Trash2 } from 'lucide-svelte'
  import type { Enclosure } from '../lib/api/types'
  import { dockEditor } from '../lib/dockEditor.svelte'
  import { t } from '../lib/i18n.svelte'

  interface Props {
    enclosure: Enclosure | null
    /** The box's display name, for the forget button's label. */
    name: string
  }

  let { enclosure, name }: Props = $props()

  const local = $derived(enclosure?.kind === 'dock' && enclosure.source === 'local')
</script>

{#if enclosure?.kind === 'dock'}
  <span class="tag" class:local>{local ? t('box.tag.localDock') : t('box.tag.dock')}</span>
  {#if local}
    <button class="forget" title={t('box.forget', { name })} aria-label={t('box.forget', { name })} onclick={() => dockEditor.forget(enclosure)}>
      <Trash2 size={12} aria-hidden="true" />
    </button>
  {/if}
{/if}

<style>
  /* Says the box came from the knowledge base rather than from the bus. */
  .tag {
    flex-shrink: 0;
    padding: 0 5px;
    border-radius: var(--radius-sm);
    background: var(--bg-subtle);
    color: var(--text-muted);
    font-size: var(--font-size-xs);
    font-weight: 600;
    line-height: 1.5;
  }
  .tag.local {
    background: color-mix(in srgb, var(--accent) 18%, var(--bg-subtle));
    color: var(--text-secondary);
  }
  .forget {
    display: inline-flex;
    flex-shrink: 0;
    padding: 2px;
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-muted);
  }
  .forget:hover,
  .forget:focus-visible {
    background: var(--bg-subtle-hover);
    color: var(--danger);
  }
</style>
