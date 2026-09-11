<script lang="ts">
  // The second line of a diagram card: what the node is and how much of it
  // is in use, worded per kind.
  import type { GraphNode } from '../lib/graph'
  import { USB4_ROUTER_SPEED } from '../lib/colors'
  import { formatBitrate, linkLabel } from '../lib/format'
  import { routerSpeed } from '../lib/graph'
  import { t } from '../lib/i18n.svelte'

  interface Props {
    node: GraphNode
    /** Devices hanging off the node, or sockets in use for a box. */
    childCount: number
    usedSockets: number
    portTotal: number
    boxKind: string
    members: string[]
  }

  let { node, childCount, usedSockets, portTotal, boxKind, members }: Props = $props()

  const device = $derived(node.device ?? null)
  const isBox = $derived(node.kind === 'box')
</script>

<div class="meta" class:wrap={isBox}>
  {#if node.kind === 'controller' && node.controller}
    <span>{t(`tree.controller.kind.${node.controller.kind}`)}</span>
    {#if device?.hub}<span>{t('tree.hub.ports', { used: childCount, total: device.hub.port_count })}</span>{/if}
  {:else if node.kind === 'router' && node.router}
    <span>{t(`tree.usb4.${node.router.kind}`)}</span>
    <span>{linkLabel(node.router.kind === 'device' ? routerSpeed(node.router) : USB4_ROUTER_SPEED)}</span>
  {:else if node.kind === 'carried'}
    <span>{t('graph.carried')}</span>
  {:else if isBox}
    {#if boxKind === 'host'}
      {#if node.hostModel}<span class="model">{node.hostModel}</span>{/if}
      <span>{members.length === 1 ? t('box.controllers.one') : t('box.controllers', { n: members.length })}</span>
      {#if node.hostRouters}<span>{node.hostRouters === 1 ? t('box.usb4.one') : t('box.usb4', { n: node.hostRouters })}</span>{/if}
    {:else if members.length > 1}
      <span>{t('box.folded', { n: members.length })}</span>
    {/if}
    <span>{node.incomplete ? t('box.sockets.partial', { used: usedSockets }) : t('box.sockets', { used: usedSockets, total: portTotal })}</span>
  {:else if device}
    {#if node.port}<span>{t('tree.port', { n: node.port.number })}</span>{/if}
    {#if device.hub}<span>{t('tree.hub.ports', { used: childCount, total: device.hub.port_count })}</span>{/if}
    {#if device.iso_reserved > 0}<span>{t('tree.iso', { rate: formatBitrate(device.iso_reserved) })}</span>{/if}
  {/if}
</div>

<style>
  .meta {
    display: flex;
    gap: var(--space-3);
    color: var(--text-muted);
    font-size: var(--font-size-xs);
    white-space: nowrap;
    overflow: hidden;
  }
  .meta span {
    overflow: hidden;
    text-overflow: ellipsis;
  }
  /* There is room on a wider card for the counts to wrap rather than clip. */
  .meta.wrap {
    flex-wrap: wrap;
    white-space: normal;
    row-gap: 0;
  }
  .meta.wrap span {
    overflow: visible;
  }
  /* The make and model is the computer's second name; it gets the full width. */
  .model {
    flex-basis: 100%;
    color: var(--text-secondary);
  }
</style>
