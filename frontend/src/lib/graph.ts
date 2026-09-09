// Layout for the topology diagram: one left-to-right tree per group, then
// the USB4 router chain. Pure and unit-tested; the components only paint
// what comes out.
//
// Two readings of the same snapshot share one placement engine:
//   layoutGraph    the logical view — every controller, hub and device the
//                  OS reports, root hub folded into the controller node.
//   layoutPhysical the physical view — the boxes on the desk, with a dock's
//                  internal hub chain and the computer's controllers each
//                  folded into one node. See physical.ts.
//
// Encoding, per the handoff spec: edge thickness is link capacity, edge
// colour (with a dash pattern, so colour is never alone) is link health,
// and the flow overlay is live utilisation of that link.
import type { Controller, Device, Enclosure, LinkSpeed, Port, Topology, USB4Router } from './api/types'
import { normalizeId } from './ids'
import { linkBitrate, linkHealth, linkRank, type LinkHealth } from './link'
import { sampleFor, type ThroughputMap } from './throughput'
import { childEntries } from './topology'
import { indexHubPaths, visiblePorts, type HubPathIndex } from './ports'
import { buildPhysical, type PhysicalNode } from './physical'
import { USB4_ROUTER_SPEED } from './colors'

export const NODE_WIDTH = 272
/**
 * A box carries more on its face than a device does — a name, what it is,
 * how many hubs it folded in, how many of its sockets are in use — so it
 * gets a wider card. Still narrower than a column, so the next column
 * clears it.
 */
export const BOX_WIDTH = 336
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

export type GraphNodeKind = 'controller' | 'device' | 'router' | 'carried' | 'box'

/** One physical socket on a hub-like node, drawn at the node's right edge. */
export interface SocketSlot {
  /** Unique within the node: a box's member hubs can share a port number. */
  key: string
  port: Port
  /** Centre of the socket, in pixels down from the node's top edge. */
  y: number
  /** Whether something is plugged into it. */
  occupied: boolean
  /**
   * Ports folded into this one because they are the same physical socket
   * seen twice — a dock reached over two controllers. Physical view only.
   */
  also?: Port[]
}

export interface GraphNode {
  /** Normalised device id, controller id, router id, enclosure id, or a synthetic id for carried devices. */
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
  /** The box this node stands for, in the physical view. */
  enclosure?: Enclosure | null
  /**
   * Normalised ids of every device this node stands for. A box node covers
   * all its member hubs, so an insight naming any of them flags the box.
   */
  members?: string[]
  /** The physical sockets this node exposes, top to bottom; empty for non-hubs. */
  sockets: SocketSlot[]
  /** Descendants not drawn because this hub is collapsed. */
  hiddenCount: number
  /** A hub here could not be read to the end, so its ports are unknown. */
  incomplete?: boolean
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

/**
 * Left edge of each column, given the widest card in each.
 *
 * Columns cannot be sized before placement because a box is wider than a
 * device and either can appear at any depth, so x is assigned afterwards.
 * With every card the same width this is the old fixed grid exactly.
 */
function columnXs(nodes: readonly GraphNode[]): number[] {
  const widths: number[] = []
  for (const node of nodes) widths[node.depth] = Math.max(widths[node.depth] ?? NODE_WIDTH, node.width)
  const xs: number[] = []
  let x = PADDING
  for (let depth = 0; depth < widths.length; depth++) {
    xs[depth] = x
    x += (widths[depth] ?? NODE_WIDTH) + COLUMN_GAP
  }
  return xs
}

/** Height a node needs to fit its socket strip, never less than a plain node. */
export function socketStripHeight(count: number): number {
  return Math.max(NODE_HEIGHT, SOCKET_PAD * 2 + count * SOCKET_PITCH)
}

function blankNode(id: string, kind: GraphNodeKind): GraphNode {
  return { id, kind, x: PADDING, y: 0, width: NODE_WIDTH, height: NODE_HEIGHT, depth: 0, sockets: [], hiddenCount: 0, bps: 0 }
}

/** Lay out a node's sockets top to bottom and grow it to fit them. */
function fitSockets(node: GraphNode, sockets: Array<Omit<SocketSlot, 'y'>>): void {
  node.sockets = sockets.map((slot, i) => ({ ...slot, y: SOCKET_PAD + i * SOCKET_PITCH + SOCKET_PITCH / 2 }))
  node.height = socketStripHeight(node.sockets.length)
}

// ---------------------------------------------------------------------------
// Placement
// ---------------------------------------------------------------------------

/** A node to place, with the nodes hanging off it. */
interface LayoutItem {
  node: GraphNode
  children: LayoutLink[]
}

/** One child of a LayoutItem, and the link that reaches it. */
interface LayoutLink {
  item: LayoutItem
  /** The parent socket the link leaves from; null to leave from the middle. */
  port: Port | null
  speed: LinkSpeed
  max: LinkSpeed
  health: LinkHealth
}

interface PendingEdge {
  from: GraphNode
  to: GraphNode
  port: Port | null
  speed: LinkSpeed
  max: LinkSpeed
  health: LinkHealth
}

/** Where a child's edge leaves its parent: the child's socket, or the middle. */
function exitY(parent: GraphNode, port: Port | null): number {
  if (port) {
    const slot = parent.sockets.find((s) => s.port === port) ?? parent.sockets.find((s) => s.port.number === port.number)
    if (slot) return parent.y + slot.y
  }
  return parent.y + parent.height / 2
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

/**
 * Place each group of trees left to right, one column per depth, groups
 * stacked down the canvas with a gap between them.
 */
function placeAll(groups: LayoutItem[][]): GraphLayout {
  const nodes: GraphNode[] = []
  const pending: PendingEdge[] = []
  let cursor = PADDING
  let firstGroup = true

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

  const place = (item: LayoutItem, depth: number): GraphNode => {
    const node = item.node
    node.depth = depth
    const start = nodes.length
    const top = cursor
    nodes.push(node)
    const placed: GraphNode[] = []
    for (const link of item.children) {
      const child = place(link.item, depth + 1)
      placed.push(child)
      pending.push({ from: node, to: child, port: link.port, speed: link.speed, max: link.max, health: link.health })
    }
    settle(node, start, top, placed)
    return node
  }

  for (const group of groups) {
    if (group.length === 0) continue
    if (!firstGroup) cursor += GROUP_GAP - ROW_GAP
    firstGroup = false
    for (const root of group) place(root, 0)
  }

  // x last: only now is it known how wide each column has to be.
  const xs = columnXs(nodes)
  let right = 0
  for (const node of nodes) {
    node.x = xs[node.depth] ?? PADDING
    right = Math.max(right, node.x + node.width)
  }
  const width = nodes.length > 0 ? right + PADDING : 0
  const height = nodes.length > 0 ? cursor - ROW_GAP + PADDING : 0
  return { width, height, nodes, edges: pending.map(edgeFrom) }
}

// ---------------------------------------------------------------------------
// The USB4 router chain, shared by both views
// ---------------------------------------------------------------------------

/**
 * The USB4 / Thunderbolt fabric, which the OS reports as a separate bus.
 * Its routers are the same physical boxes as the hubs above, but nothing in
 * a snapshot ties a router to the hub it shares a chassis with, so both
 * views draw the chain as its own group rather than inventing the link.
 */
function routerGroup(topology: Topology | null): LayoutItem[] {
  const routers = topology?.usb4 ?? []
  if (routers.length === 0) return []

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

  const build = (router: USB4Router): LayoutItem => {
    const node = blankNode(normalizeId(router.id), 'router')
    node.router = router
    const children: LayoutLink[] = []
    for (const child of childrenOf.get(node.id) ?? []) {
      children.push({ item: build(child), port: null, speed: USB4_ROUTER_SPEED, max: USB4_ROUTER_SPEED, health: 'idle' })
    }
    const carried = router.children ?? []
    for (let i = 0; i < carried.length; i++) {
      const leaf = blankNode(`${node.id}#carried-${i}`, 'carried')
      leaf.label = carried[i]
      children.push({ item: { node: leaf, children: [] }, port: null, speed: 'unknown', max: 'unknown', health: 'idle' })
    }
    return { node, children }
  }

  return roots.map(build)
}

// ---------------------------------------------------------------------------
// The logical view
// ---------------------------------------------------------------------------

/**
 * Give a hub-like node its socket strip: one slot per physical socket, so a
 * device's edge can leave from the socket it is actually plugged into.
 */
function attachSockets(node: GraphNode, topology: Topology | null, hubs: HubPathIndex): void {
  if (!node.device?.hub) return
  const device = node.device
  fitSockets(
    node,
    visiblePorts(device, topology, hubs).map((port) => ({
      key: `${normalizeId(device.id)}:${port.number}`,
      port,
      occupied: port.device !== undefined,
    })),
  )
}

export function layoutGraph(topology: Topology | null, opts: GraphOptions): GraphLayout {
  const hubs = indexHubPaths(topology)

  const buildDevice = (device: Device, port: Port): LayoutItem => {
    const open = device.hub === undefined || opts.isExpanded(device.id)
    const node = blankNode(normalizeId(device.id), 'device')
    node.device = device
    node.port = port
    node.hiddenCount = open ? 0 : descendantCount(device)
    node.incomplete = device.hub?.incomplete === true
    node.bps = subtreeBps(device, opts.throughput)
    attachSockets(node, topology, hubs)
    const children = open
      ? childEntries(device).map((c) => ({
        item: buildDevice(c.device, c.port),
        port: c.port,
        speed: c.port.negotiated_link,
        max: c.port.max_link,
        health: linkHealth(c.port, c.device),
      }))
      : []
    return { node, children }
  }

  const groups: LayoutItem[][] = []
  for (const controller of topology?.controllers ?? []) {
    const root = controller.root_hub
    const node = blankNode(normalizeId(controller.id), 'controller')
    node.controller = controller
    node.device = root ?? undefined
    node.port = null
    node.bps = root ? subtreeBps(root, opts.throughput) : 0
    node.incomplete = root?.hub?.incomplete === true
    attachSockets(node, topology, hubs)
    const children = (root ? childEntries(root) : []).map((c) => ({
      item: buildDevice(c.device, c.port),
      port: c.port,
      speed: c.port.negotiated_link,
      max: c.port.max_link,
      health: linkHealth(c.port, c.device),
    }))
    groups.push([{ node, children }])
  }
  groups.push(routerGroup(topology))

  return placeAll(groups)
}

// ---------------------------------------------------------------------------
// The physical view
// ---------------------------------------------------------------------------

/** Traffic through a box's cable: every member's own sample plus its subtree. */
function physicalBps(node: PhysicalNode, throughput: ThroughputMap): number {
  let total = 0
  if (node.device) total += subtreeBps(node.device, throughput)
  for (const member of node.members) {
    const own = sampleFor(throughput, member.id)
    if (own) total += own.read_bps + own.write_bps
  }
  for (const child of node.children) total += physicalBps(child.node, throughput)
  return total
}

/** Things drawn behind this node, which a collapsed node hides. */
function hiddenBehind(node: PhysicalNode): number {
  let n = 0
  for (const child of node.children) n += 1 + hiddenBehind(child.node)
  return n
}

export function layoutPhysical(topology: Topology | null, opts: GraphOptions): GraphLayout {
  const build = (physical: PhysicalNode, port: Port | null): LayoutItem => {
    const isBox = physical.device === null
    const node = blankNode(physical.id, isBox ? 'box' : 'device')
    if (isBox) node.width = BOX_WIDTH
    node.port = port
    node.bps = physicalBps(physical, opts.throughput)
    if (isBox) {
      node.enclosure = physical.enclosure
      node.incomplete = physical.incomplete
      node.members = physical.members.map((m) => normalizeId(m.id))
      // The first member is the box's uplink hub: the one whose name and
      // class stand in for the box when the knowledge base has no name.
      node.device = physical.members[0]
      fitSockets(
        node,
        physical.sockets.map((s) => ({ key: s.key, port: s.port, occupied: s.port.device !== undefined, also: s.also })),
      )
    } else {
      node.device = physical.device ?? undefined
    }

    const open = !isBox || opts.isExpanded(physical.id)
    node.hiddenCount = open ? 0 : hiddenBehind(physical)
    const children: LayoutLink[] = []
    if (open) {
      for (const child of physical.children) {
        // A child only exists because something is plugged into the socket.
        const plugged = child.socket.port.device
        if (!plugged) continue
        children.push({
          item: build(child.node, child.socket.port),
          port: child.socket.port,
          speed: child.socket.port.negotiated_link,
          max: child.socket.port.max_link,
          health: linkHealth(child.socket.port, plugged),
        })
      }
    }
    return { node, children }
  }

  const groups: LayoutItem[][] = buildPhysical(topology).map((root) => [build(root, null)])
  groups.push(routerGroup(topology))
  return placeAll(groups)
}
