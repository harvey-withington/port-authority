<script lang="ts">
  import { TriangleAlert } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'

  interface Props {
    /** Non-fatal collection problems from the snapshot; nothing renders when empty. */
    warnings: readonly string[]
  }

  let { warnings }: Props = $props()
</script>

{#if warnings.length > 0}
  <div class="warnings" role="alert">
    <span class="icon" aria-hidden="true"><TriangleAlert size={14} /></span>
    <div>
      <strong>{t('tree.warnings')}</strong>
      <ul>
        {#each warnings as w (w)}<li>{w}</li>{/each}
      </ul>
    </div>
  </div>
{/if}

<style>
  .warnings {
    display: flex;
    gap: var(--space-2);
    margin: 0 0 var(--space-2);
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-md);
    background: var(--warning-bg);
    color: var(--warning-text);
    font-size: var(--font-size-sm);
  }
  .icon {
    display: inline-flex;
    margin-top: 2px;
  }
  ul {
    margin-top: 2px;
  }
</style>
