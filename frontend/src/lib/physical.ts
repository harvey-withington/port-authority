// The physical reading of a snapshot: the boxes on the desk and the cables
// between them, rather than the controllers and hubs the OS reports.
//
// Windows describes a dock as a chain of four or five hubs, split across
// two controllers because its USB 2 and USB 3 halves enumerate separately,
// and describes the computer as several controllers with a root hub each.
// A person sees a laptop and a dock joined by one cable. This module folds
// the first description into the second, using the enclosure ids that
// enrichment stamped on every hub, and never guessing beyond them.
//
// Pure and unit-tested; graph.ts lays the result out and the components
// only paint what comes out.
import type { Device, Enclosure, EnclosureKind, Port, Topology } from './api/types'
import { normalizeId } from './ids'
import { linkRank } from './link'
import { indexHubPaths, socketKind, visiblePorts, type HubPathIndex } from './ports'
import { indexTopology, type DeviceIndex } from './topology'

export type PhysicalKind = EnclosureKind | 'device'

/** One physical socket on a box, after folding away duplicates. */
export interface PhysicalSocket {
  /** Unique within the box: two member hubs can both have a port 1. */
  key: string
  port: Port
  /** The member hub the port actually lives on. */
  hub: Device
  /**
   * Ports folded into this one because they proved to be the same socket:
   * a dock reached over two controllers is plugged in with one cable, so
   * the two uplink ports the OS reports are one hole in the chassis.
   */
  also: Port[]
}

/** A box on the desk, or a single device plugged into one. */
export interface PhysicalNode {
  /** Enclosure id for a box, normalised device id for a leaf. */
  id: string
  kind: PhysicalKind
  /**
   * True when any hub in this box could not be read to the end, so the
   * sockets shown are only the ones we know about.
   */
  incomplete: boolean
  /** The hubs folded into this box, in walk order; empty for a leaf. */
  members: Device[]
  enclosure: Enclosure | null
  /** The device itself, for a leaf node. */
  device: Device | null
  sockets: PhysicalSocket[]
  children: PhysicalChild[]
}

export interface PhysicalChild {
  node: PhysicalNode
  /** The socket on the parent box this hangs off. */
  socket: PhysicalSocket
}

/**
 * Which box a hub belongs to.
 *
 * Enrichment fills this in, but a snapshot from a provider that skipped it
 * still has to render, so a root hub falls back to the computer and any
 * other hub to a box of its own — never to a guess about nesting.
 */
export function enclosureIdOf(device: Device, isRoot: boolean): string | null {
  if (!device.hub) return null
  if (device.enclosure) return device.enclosure.id
  return isRoot ? 'host' : `hub:${device.id}`
}

/** Position groups in the order a person reads a chassis, unknown last. */
const POSITION_ORDER: readonly string[] = ['front', 'top', 'left', 'right', 'rear', 'back']

function positionRank(position: string | undefined): number {
  if (!position) return POSITION_ORDER.length
  const i = POSITION_ORDER.indexOf(position.trim().toLowerCase())
  return i < 0 ? POSITION_ORDER.length : i
}

/** The faster of two ports for one socket, ties going to the lower number. */
function betterPort(a: Port, b: Port): Port {
  const ra = linkRank(a.max_link)
  const rb = linkRank(b.max_link)
  if (ra !== rb) return ra > rb ? a : b
  return a.number <= b.number ? a : b
}

interface Build {
  topology: Topology | null
  hubPaths: HubPathIndex
  /** Enclosure id -> the hubs in that box, in walk order. */
  boxes: Map<string, Device[]>
  /** Normalised device id -> the enclosure it is a member of. */
  boxOf: Map<string, string>
  /** Boxes already turned into nodes, so a shared child is built once. */
  built: Map<string, PhysicalNode>
}

/** Group every hub in the snapshot by the box it belongs to. */
function collectBoxes(index: DeviceIndex): Pick<Build, 'boxes' | 'boxOf'> {
  const boxes = new Map<string, Device[]>()
  const boxOf = new Map<string, string>()
  for (const [id, ref] of index) {
    // Routers are indexed for naming only; they are not in the hub tree.
    if (!ref.device) continue
    const enclosureId = enclosureIdOf(ref.device, ref.parentId === null)
    if (!enclosureId) continue
    boxOf.set(id, enclosureId)
    const members = boxes.get(enclosureId)
    if (members) members.push(ref.device)
    else boxes.set(enclosureId, [ref.device])
  }
  return { boxes, boxOf }
}

/**
 * The sockets a box exposes: every visible port of every member hub, minus
 * the ports that only wire one member to another. Those are the inside of
 * the box, which is exactly what this view exists to hide.
 */
function socketsFor(build: Build, boxId: string, members: Device[]): PhysicalSocket[] {
  const raw: PhysicalSocket[] = []
  for (const hub of members) {
    for (const port of visiblePorts(hub, build.topology, build.hubPaths)) {
      const child = port.device
      if (child && build.boxOf.get(normalizeId(child.id)) === boxId) continue
      raw.push({ key: `${normalizeId(hub.id)}:${port.number}`, port, hub, also: [] })
    }
  }

  // Two sockets leading to the same box are one socket: a dock's USB 2 and
  // USB 3 uplinks are a single cable in a single hole. Keep the faster port
  // and remember the other, so the tooltip can still account for it.
  const byChild = new Map<string, PhysicalSocket>()
  const out: PhysicalSocket[] = []
  for (const socket of raw) {
    const child = socket.port.device
    const childBox = child ? build.boxOf.get(normalizeId(child.id)) : undefined
    if (!childBox) {
      out.push(socket)
      continue
    }
    const seen = byChild.get(childBox)
    if (!seen) {
      byChild.set(childBox, socket)
      out.push(socket)
      continue
    }
    const winner = betterPort(seen.port, socket.port)
    seen.also.push(winner === seen.port ? socket.port : seen.port)
    if (winner !== seen.port) {
      seen.port = winner
      seen.hub = socket.hub
      seen.key = `${normalizeId(socket.hub.id)}:${winner.number}`
    }
  }

  // Grouped by where the socket is on the chassis, with the soldered-in
  // internals last, so the ones a person can reach come first.
  return out
    .map((socket, i) => ({ socket, i }))
    .sort((a, b) => {
      const ai = socketKind(a.socket.port) === 'internal' ? 1 : 0
      const bi = socketKind(b.socket.port) === 'internal' ? 1 : 0
      return ai - bi || positionRank(a.socket.port.position) - positionRank(b.socket.port.position) || a.i - b.i
    })
    .map((entry) => entry.socket)
}

function leafNode(device: Device): PhysicalNode {
  return { id: normalizeId(device.id), kind: 'device', members: [], enclosure: null, device, sockets: [], children: [], incomplete: false }
}

function boxNode(build: Build, boxId: string): PhysicalNode {
  const existing = build.built.get(boxId)
  if (existing) return existing

  const members = build.boxes.get(boxId) ?? []
  const enclosure = members.find((m) => m.enclosure)?.enclosure ?? null
  const node: PhysicalNode = {
    id: boxId,
    kind: enclosure?.kind ?? (boxId === 'host' ? 'host' : 'hub'),
    members,
    enclosure,
    device: null,
    sockets: [],
    children: [],
    incomplete: members.some((m) => m.hub?.incomplete === true),
  }
  // Register before recursing: a box reached from two places must not be
  // rebuilt, and registering first also stops a cycle in malformed data.
  build.built.set(boxId, node)

  node.sockets = socketsFor(build, boxId, members)
  for (const socket of node.sockets) {
    const child = socket.port.device
    if (!child) continue
    const childBox = build.boxOf.get(normalizeId(child.id))
    node.children.push({ socket, node: childBox ? boxNode(build, childBox) : leafNode(child) })
  }
  return node
}

/**
 * The boxes in a snapshot, as roots to draw. Normally that is one node,
 * the computer; a box the computer cannot reach is returned alongside it
 * rather than dropped.
 */
export function buildPhysical(topology: Topology | null): PhysicalNode[] {
  if (!topology) return []
  const { boxes, boxOf } = collectBoxes(indexTopology(topology))
  const build: Build = { topology, hubPaths: indexHubPaths(topology), boxes, boxOf, built: new Map() }

  const roots: PhysicalNode[] = []
  if (boxes.has('host')) roots.push(boxNode(build, 'host'))
  // Anything the walk from the computer never reached: unusual, but a box
  // that exists gets drawn rather than silently lost.
  for (const boxId of boxes.keys()) {
    if (!build.built.has(boxId)) roots.push(boxNode(build, boxId))
  }
  return roots
}

/** Normalised ids of every device this node stands for. */
export function memberIds(node: PhysicalNode): string[] {
  if (node.device) return [node.id]
  return node.members.map((m) => normalizeId(m.id))
}
