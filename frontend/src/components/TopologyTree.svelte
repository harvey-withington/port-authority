<script lang="ts">
  import type { Topology } from '../lib/api/types'
  import type { TreeContext } from '../lib/tree'
  import { t } from '../lib/i18n.svelte'
  import CollectionWarnings from './CollectionWarnings.svelte'
  import ControllerNode from './ControllerNode.svelte'
  import Usb4Section from './Usb4Section.svelte'

  interface Props {
    topology: Topology | null
    ctx: TreeContext
    loading: boolean
  }

  let { topology, ctx, loading }: Props = $props()

  const routers = $derived(topology?.usb4 ?? [])
</script>

<div class="tree">
  <CollectionWarnings warnings={topology?.warnings ?? []} />

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
    /* Scrolls inside its pane; the app shell never scrolls. */
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    overflow-x: hidden;
  }
  .empty {
    padding: var(--space-4);
    color: var(--text-muted);
    text-align: center;
  }
</style>
