<script lang="ts">
  import { Cpu } from 'lucide-svelte'
  import type { Controller } from '../lib/api/types'
  import type { TreeContext } from '../lib/tree'
  import { childEntries } from '../lib/topology'
  import { formatBitrate } from '../lib/format'
  import { t } from '../lib/i18n.svelte'
  import DeviceNode from './DeviceNode.svelte'

  interface Props {
    controller: Controller
    ctx: TreeContext
  }

  let { controller, ctx }: Props = $props()

  const root = $derived(controller.root_hub)
  const children = $derived(root ? childEntries(root) : [])
</script>

<section class="controller" data-controller-id={controller.id}>
  <header>
    <span class="icon" aria-hidden="true"><Cpu size={16} /></span>
    <div class="text">
      <h3>{controller.name}</h3>
      <div class="meta">
        <span>{t(`tree.controller.kind.${controller.kind}`)}</span>
        {#if controller.max_bandwidth > 0}<span>{t('tree.controller.max', { rate: formatBitrate(controller.max_bandwidth) })}</span>{/if}
        {#if root?.hub}<span>{t('tree.hub.ports', { used: children.length, total: root.hub.port_count })}</span>{/if}
      </div>
    </div>
  </header>
  {#if children.length > 0}
    <ul class="devices">
      {#each children as child (child.device.id)}
        <DeviceNode device={child.device} port={child.port} depth={1} {ctx} />
      {/each}
    </ul>
  {/if}
</section>

<style>
  .controller {
    padding: var(--space-2) 0;
  }
  header {
    display: flex;
    align-items: flex-start;
    gap: var(--space-2);
    padding: var(--space-1) var(--space-2);
    margin-bottom: var(--space-1);
  }
  .icon {
    color: var(--accent-light);
    display: inline-flex;
    margin-top: 2px;
  }
  h3 {
    font-size: var(--font-size-md);
    font-weight: 600;
    color: var(--text-primary);
  }
  .meta {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1) var(--space-3);
    color: var(--text-muted);
    font-size: var(--font-size-xs);
  }
  .devices {
    margin-left: var(--space-2);
  }
</style>
