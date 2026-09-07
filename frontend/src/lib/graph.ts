// Layout for the topology diagram: one left-to-right tree per controller
// (root hub folded into the controller node), then the USB4 router chain.
// Pure and unit-tested; the components only paint what comes out.
//
// Encoding, per the handoff spec: edge thickness is link capacity, edge
// colour (with a dash pattern, so colour is never alone) is link health,
// and the flow overlay is live utilisation of that link.
import type { Controller, Device, LinkSpeed, Port, Topology, USB4Router } from './api/types'
import { normalizeId } from './ids'
import { linkBitrate, linkHealth, linkRank, type LinkHealth } from './link'
import { sampleFor, type ThroughputMap } from './throughput'
import { childEntries } from './topology'
import { indexHubPaths, visiblePorts, type HubPathIndex } from './ports'
import { USB4_ROUTER_SPEED } from './colors'

export const NODE_WIDTH = 272
export const NODE_HEIGHT = 58
export const COLUMN_GAP = 72
export const ROW_GAP = 14
export const GROUP_GAP = 44
export const PADDING = 16
/** Vertical distance between two socket centres in the node's socket strip. */
export const SOCKET_PITCH = 24
/** Space above the first socket and below the last one. */
export const SOCKET_PAD = 10
/** Width of the inset panel the sockets sit in, at the node's right edge. */
export const SOCKET_STRIP_WIDTH = 48

export type GraphNodeKind = 'controller' | 'device' | 'router' | 'carried'

/** One physical socket on a hub-like node, drawn at the node's right edge. */
export interface SocketSlot {
  port: Port
  /** Centre of the socket, in pixels down from the node's top edge. */
  y: number
  /** Whether something is plugged into it. */
  occupied: boolean
}

export interface GraphNode {
  /** Normalised device id, controller id, router id, or a synthetic id for carried devices. */
  id: string
  kind: GraphNodeKind
  /** Top-left corner in layout pixels (before zoom). */
  x: number
  y: number
  width: number
  height: number
  depth: number
  controller?: Controller
  device?: Device
  /** The hub port a device hangs off; null for a controller's folded root hub. */
  port?: Port | null
  router?: USB4Router
  /** Display text for carried (PCIe-tunnelled) devices. */
  label?: string
  /** The physical sockets this node exposes, top to bottom; empty for non-hubs. */
  sockets: SocketSlot[]
  /** Descendants not drawn because this hub is collapsed. */
  hiddenCount: number
  /** Bits per second through this node's uplink: its own sample, or the sum of its subtree. */
  bps: number
}

export interface GraphEdge {
  id: string
  from: string
  to: string
  x1: number
  y1: number
  x2: number
  y2: number
  /** Negotiated speed of the link the edge stands for. */
  speed: LinkSpeed
  max: LinkSpeed
  health: LinkHealth
  /** Stroke width in layout pixels, monotonic in capacity. */
  width: number
  bps: number
  /** 0..1 share of the link's nominal rate in use right now. */
  utilization: number
}

export interface GraphLayout {
  width: number
  height: number
  nodes: GraphNode[]
  edges: GraphEdge[]
}

export interface GraphOptions {
  isExpanded: (deviceId: string) => boolean
  throughput: ThroughputMap
}

const MIN_EDGE_WIDTH = 1.5
const EDGE_WIDTH_STEP = 1.2

/** Stroke width for a link speed: 1.5 px for USB 1 low, about 11 px for USB4 80 Gbps. */
export function edgeWidth(speed: LinkSpeed): number {
  const rank = linkRank(speed) - linkRank('low')
  if (rank < 0) return MIN_EDGE_WIDTH
  return MIN_EDGE_WIDTH + rank * EDGE_WIDTH_STEP
}

/** Traffic through a device's uplink: its own sample plus everything behind it. */
export function subtreeBps(device: Device, throughput: ThroughputMap): number {
  const own = sampleFor(throughput, device.id)
  let total = own ? own.read_bps + own.write_bps : 0
  for (const child of childEntries(device)) total += subtreeBps(child.device, throughput)
  return total
}

export function descendantCount(device: Device): number {
  let n = 0
  for (const child of childEntries(device)) n += 1 + descendantCount(child.device)
  return n
}

export function utilization(bps: number, speed: LinkSpeed): number {
  const capacity = linkBitrate(speed)
  if (capacity <= 0 || bps <= 0) return 0
  return Math.min(1, bps / capacity)
}

function columnX(depth: number): number {
  return PADDING + depth * (NODE_WIDTH + COLUMN_GAP)
}

interface PendingEdge {
  from: GraphNode
  to: GraphNode
  /** The parent socket the link leaves from; null for the USB4 router chain. */
  port: Port | null
  speed: LinkSpeed
  max: LinkSpeed
  health: LinkHealth
}

function edgeFrom(p: PendingEdge): GraphEdge {
  return {
    id: `${p.from.id}->${p.to.id}`,
    from: p.from.id,
    to: p.to.id,
    x1: p.from.x + p.from.width,
    y1: exitY(p.from, p.port),
    x2: p.to.x,
    y2: p.to.y + p.to.height / 2,
    speed: p.speed,
    max: p.max,
    health: p.health,
    width: edgeWidth(p.speed),
    bps: p.to.bps,
    utilization: utilization(p.to.bps, p.speed),
  }
}

function blankNode(id: string, kind: GraphNodeKind, depth: number): GraphNode {
  return { id, kind, x: columnX(depth), y: 0, width: NODE_WIDTH, height: NODE_HEIGHT, depth, sockets: [], hiddenCount: 0, bps: 0 }
}

/** Height a node needs to fit its socket strip, never less than a plain node. */
export function socketStripHeight(count: number): number {
  return Math.max(NODE_HEIGHT, SOCKET_PAD * 2 + count * SOCKET_PITCH)
}

/**
 * Give a hub-like node its socket strip: one slot per physical socket, so a
 * device's edge can leave from the socket it is actually plugged into.
 */
function attachSockets(node: GraphNode, topology: Topology | null, hubs: HubPathIndex): void {
  if (!node.device?.hub) return
  node.sockets = visiblePorts(node.device, topology, hubs).map((port, i) => ({
    port,
    y: SOCKET_PAD + i * SOCKET_PITCH + SOCKET_PITCH / 2,
    occupied: port.device !== undefined,
  }))
  node.height = socketStripHeight(node.sockets.length)
}

/** Where a child's edge leaves its parent: the child's socket, or the parent's middle. */
function exitY(parent: GraphNode, port: Port | null): number {
  if (port) {
    const slot = parent.sockets.find((s) => s.port === port) ?? parent.sockets.find((s) => s.port.number === port.number)
    if (slot) return parent.y + slot.y
  }
  return parent.y + parent.height / 2
}

export function layoutGraph(topology: Topology | null, opts: GraphOptions): GraphLayout {
  const nodes: GraphNode[] = []
  const pending: PendingEdge[] = []
  const hubs = indexHubPaths(topology)
  let cursor = PADDING
  let maxDepth = 0
  let firstGroup = true

  const startGroup = (): void => {
    if (!firstGroup) cursor += GROUP_GAP - ROW_GAP
    firstGroup = false
  }

  const centreOver = (node: GraphNode, children: GraphNode[]): void => {
    const first = children[0]
    const last = children[children.length - 1]
    node.y = (first.y + last.y + last.height - node.height) / 2
  }

  /**
   * Place a node that has already been pushed, together with everything its
   * recursion pushed after it. A parent taller than its children's span would
   * otherwise be centred above the row it started on and collide with the
   * group before it, so the whole DFS subtree (nodes from `start` on) slides
   * down instead, and the cursor clears the parent's own bottom edge.
   */
  const settle = (node: GraphNode, start: number, top: number, children: GraphNode[]): void => {
    if (children.length > 0) centreOver(node, children)
    else node.y = cursor
    if (node.y < top) {
      const shift = top - node.y
      for (let i = start; i < nodes.length; i++) nodes[i].y += shift
      cursor += shift
    }
    cursor = Math.max(cursor, node.y + node.height + ROW_GAP)
  }

  const placeDevice = (device: Device, port: Port, parent: GraphNode, depth: number): GraphNode => {
    maxDepth = Math.max(maxDepth, depth)
    const children = childEntries(device)
    const open = device.hub === undefined || opts.isExpanded(device.id)
    const node = blankNode(normalizeId(device.id), 'device', depth)
    node.device = device
    node.port = port
    node.hiddenCount = open ? 0 : descendantCount(device)
    node.bps = subtreeBps(device, opts.throughput)
    attachSockets(node, topology, hubs)
    const start = nodes.length
    const top = cursor
    nodes.push(node)
    const placed = open && children.length > 0
      ? children.map((c) => placeDevice(c.device, c.port, node, depth + 1))
      : []
    settle(node, start, top, placed)
    pending.push({ from: parent, to: node, port, speed: port.negotiated_link, max: port.max_link, health: linkHealth(port, device) })
    return node
  }

  for (const controller of topology?.controllers ?? []) {
    startGroup()
    const root = controller.root_hub
    const node = blankNode(normalizeId(controller.id), 'controller', 0)
    node.controller = controller
    node.device = root ?? undefined
    node.port = null
    node.bps = root ? subtreeBps(root, opts.throughput) : 0
    attachSockets(node, topology, hubs)
    const start = nodes.length
    const top = cursor
    nodes.push(node)
    const children = root ? childEntries(root) : []
    settle(node, start, top, children.map((c) => placeDevice(c.device, c.port, node, 1)))
  }

  const routers = topology?.usb4 ?? []
  if (routers.length > 0) {
    startGroup()
    const ids = new Set(routers.map((r) => normalizeId(r.id)))
    const childrenOf = new Map<string, USB4Router[]>()
    const roots: USB4Router[] = []
    for (const r of routers) {
      const parent = r.parent_id ? normalizeId(r.parent_id) : ''
      if (parent && ids.has(parent) && parent !== normalizeId(r.id)) {
        childrenOf.set(parent, [...(childrenOf.get(parent) ?? []), r])
      } else {
        roots.push(r)
      }
    }

    const placeRouter = (router: USB4Router, parent: GraphNode | null, depth: number): GraphNode => {
      maxDepth = Math.max(maxDepth, depth)
      const node = blankNode(normalizeId(router.id), 'router', depth)
      node.router = router
      const start = nodes.length
      const top = cursor
      nodes.push(node)
      const placed: GraphNode[] = []
      for (const child of childrenOf.get(node.id) ?? []) placed.push(placeRouter(child, node, depth + 1))
      const carriedLabels = router.children ?? []
      for (let i = 0; i < carriedLabels.length; i++) {
        maxDepth = Math.max(maxDepth, depth + 1)
        const carried = blankNode(`${node.id}#carried-${i}`, 'carried', depth + 1)
        carried.label = carriedLabels[i]
        nodes.push(carried)
        settle(carried, nodes.length - 1, cursor, [])
        placed.push(carried)
        pending.push({ from: node, to: carried, port: null, speed: 'unknown', max: 'unknown', health: 'idle' })
      }
      settle(node, start, top, placed)
      if (parent) pending.push({ from: parent, to: node, port: null, speed: USB4_ROUTER_SPEED, max: USB4_ROUTER_SPEED, health: 'idle' })
      return node
    }

    for (const root of roots) placeRouter(root, null, 0)
  }

  const width = nodes.length > 0 ? columnX(maxDepth) + NODE_WIDTH + PADDING : 0
  const height = nodes.length > 0 ? cursor - ROW_GAP + PADDING : 0
  return { width, height, nodes, edges: pending.map(edgeFrom) }
}
