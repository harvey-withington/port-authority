// The one place a diagram node gets its display name, shared by the node
// card and the edge tooltips so the two can never disagree.
import type { GraphNode } from './graph'
import { deviceName, routerName } from './topology'
import { t } from './i18n.svelte'

export function graphNodeName(node: GraphNode): string {
  switch (node.kind) {
    case 'controller':
      return node.controller?.name ?? ''
    case 'router':
      return node.router ? routerName(node.router) : ''
    case 'carried':
      return node.label ?? ''
    case 'box':
      // A box the knowledge base named keeps that name; otherwise the
      // computer is "This computer" and a lone hub is named by its uplink hub.
      if (node.enclosure?.name) return node.enclosure.name
      if (node.enclosure?.kind === 'host') return t('box.host')
      return node.device ? deviceName(node.device) : t('box.unknown')
    case 'device':
      return node.device ? deviceName(node.device) : ''
  }
}
