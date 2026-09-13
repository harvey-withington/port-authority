<script lang="ts">
  // The little word on a box saying where its grouping came from: "dock"
  // from the shipped knowledge base, "community dock" from the shared
  // one, "your dock" when the user set it up here. A dock of the user's
  // own can be shared with the community or forgotten again.
  import { Share2, Trash2 } from 'lucide-svelte'
  import type { DockView, Enclosure } from '../lib/api/types'
  import { dockEditor } from '../lib/dockEditor.svelte'
  import { openExternal, shareDockUrl } from '../lib/community'
  import { t } from '../lib/i18n.svelte'

  interface Props {
    enclosure: Enclosure | null
    /** The box's display name, for the button labels. */
    name: string
    /** The knowledge base entry behind the box, when known; what sharing sends. */
    dock?: DockView
  }

  let { enclosure, name, dock }: Props = $props()

  const local = $derived(enclosure?.kind === 'dock' && enclosure.source === 'local')
  const shared = $derived(enclosure?.kind === 'dock' && enclosure.source === 'shared')
  const tag = $derived(local ? t('box.tag.localDock') : shared ? t('box.tag.sharedDock') : t('box.tag.dock'))
</script>

{#if enclosure?.kind === 'dock'}
  <span class="tag" class:local class:shared>{tag}</span>
  {#if local}
    {#if dock}
      <button class="action" title={t('box.share.hint', { name })} aria-label={t('box.share', { name })} onclick={() => openExternal(shareDockUrl(dock))}>
        <Share2 size={12} aria-hidden="true" />
      </button>
    {/if}
    <button class="action forget" title={t('box.forget', { name })} aria-label={t('box.forget', { name })} onclick={() => dockEditor.forget(enclosure)}>
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
  .tag.shared {
    background: color-mix(in srgb, var(--success) 14%, var(--bg-subtle));
    color: var(--text-secondary);
  }
  .action {
    display: inline-flex;
    flex-shrink: 0;
    padding: 2px;
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-muted);
  }
  .action:hover,
  .action:focus-visible {
    background: var(--bg-subtle-hover);
    color: var(--text-strong);
  }
  .forget:hover,
  .forget:focus-visible {
    color: var(--danger);
  }
</style>
