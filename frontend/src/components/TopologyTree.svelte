<script lang="ts">
  import { TriangleAlert } from 'lucide-svelte'
  import type { Topology } from '../lib/api/types'
  import type { TreeContext } from '../lib/tree'
  import { t } from '../lib/i18n.svelte'
  import ControllerNode from './ControllerNode.svelte'
  import Usb4Section from './Usb4Section.svelte'

  interface Props {
    topology: Topology | null
    ctx: TreeContext
    loading: boolean
  }

  let { topology, ctx, loading }: Props = $props()

  const warnings = $derived(topology?.warnings ?? [])
  const routers = $derived(topology?.usb4 ?? [])
</script>

<div class="tree">
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

  {#if !topology}
    <p class="empty">{loading ? t('tree.loading') : t('tree.empty')}</p>
  {:else if topology.controllers.length === 0}
    <p class="empty">{t('tree.empty')}</p>
  {:else}
    {#each topology.controllers as controller (controller.id)}
      <ControllerNode {controller} {ctx} />
    {/each}
  {/if}

  {#if routers.length > 0}
    <Usb4Section {routers} />
  {/if}
</div>

<style>
  .tree {
    display: flex;
    flex-direction: column;
  }
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
  .warnings .icon {
    display: inline-flex;
    margin-top: 2px;
  }
  .warnings ul {
    margin-top: 2px;
  }
  .empty {
    padding: var(--space-4);
    color: var(--text-muted);
    text-align: center;
  }
</style>
