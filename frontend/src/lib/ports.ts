// The physical sockets behind a hub's port list.
//
// Windows reports one hub port per logical port, so a single USB-C or USB-A
// socket on the chassis shows up twice: once on the USB 3 side and once on
// the USB 2 companion. The connector descriptor links the pair
// (companion_hub_path + companion_port), and this module folds them back
// into the one socket a person can actually see and plug into.
//
// Pure and free of i18n: the components turn a SocketKind into words.
import type { Device, Port, Topology } from './api/types'
import { linkRank } from './link'

export type SocketKind = 'usb-c' | 'usb-a' | 'internal' | 'unknown'

/** The link speeds that mean a USB4 / Thunderbolt socket. */
const USB4_LINKS: ReadonlySet<string> = new Set(['usb4_20', 'usb4_40', 'usb4_80'])

/**
 * What the socket looks like on the outside. Without connector data from the
 * provider we cannot tell, so the glyph stays deliberately generic.
 */
export function socketKind(port: Port): SocketKind {
  const connector = port.connector
  if (!connector) return 'unknown'
  if (!connector.user_connectable) return 'internal'
  return connector.type_c ? 'usb-c' : 'usb-a'
}

/** True when the port itself can carry a USB4 / Thunderbolt link. */
export function isUsb4(port: Port): boolean {
  return USB4_LINKS.has(port.max_link)
}

/** Normalised hub device path -> the hub device that owns it. */
export type HubPathIndex = ReadonlyMap<string, Device>

/**
 * Device paths are compared case-insensitively after dropping a "\\?\" or
 * "\\.\" namespace prefix, which some sources carry and others do not.
 */
export function normalizeHubPath(path: string): string {
  const trimmed = path.trim()
  const prefixed = trimmed.startsWith('\\\\?\\') || trimmed.startsWith('\\\\.\\')
  return (prefixed ? trimmed.slice(4) : trimmed).toUpperCase()
}

/** Index every hub in the snapshot by its device path, so companions resolve in O(1). */
export function indexHubPaths(topology: Topology | null): HubPathIndex {
  const index = new Map<string, Device>()
  if (!topology) return index
  const visit = (device: Device): void => {
    const hub = device.hub
    if (!hub) return
    if (hub.device_path) index.set(normalizeHubPath(hub.device_path), device)
    // A hub the collector could not open has no port list at all, which
    // arrives as null rather than an empty array. It is still worth
    // indexing: something else may name it as its companion.
    for (const port of hub.ports ?? []) {
      if (port.device) visit(port.device)
    }
  }
  for (const controller of topology.controllers) {
    if (controller.root_hub) visit(controller.root_hub)
  }
  return index
}

/** The port on the other side of the same physical socket, when we can find it. */
export function companionPort(port: Port, index: HubPathIndex): Port | null {
  const connector = port.connector
  if (!connector?.companion_hub_path || connector.companion_port === undefined) return null
  const hub = index.get(normalizeHubPath(connector.companion_hub_path))?.hub
  if (!hub) return null
  return (hub.ports ?? []).find((p) => p.number === connector.companion_port) ?? null
}

/**
 * The hub's ports in number order, one entry per physical socket.
 *
 * A port is dropped only when it is empty and its companion resolves to the
 * better half of the pair: a faster port maximum, or the same maximum and a
 * lower port number as the tie-break. A port with a device is never dropped,
 * and an unresolvable companion keeps both halves rather than hiding a socket
 * that exists.
 */
export function visiblePorts(hub: Device, topology: Topology | null, index?: HubPathIndex): Port[] {
  const ports = hub.hub?.ports
  if (!ports || ports.length === 0) return []
  const paths = index ?? indexHubPaths(topology)
  const ordered = [...ports].sort((a, b) => a.number - b.number)
  return ordered.filter((port) => {
    if (port.device) return true
    const companion = companionPort(port, paths)
    if (!companion) return true
    const here = linkRank(port.max_link)
    const there = linkRank(companion.max_link)
    if (there > here) return false
    return !(there === here && companion.number < port.number)
  })
}
