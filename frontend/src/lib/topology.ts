// Pure helpers over a Topology snapshot: naming and an id-keyed index.
import type { Controller, Device, Port, Topology, USB4Router } from './api/types'
import { normalizeId } from './ids'
import { t } from './i18n.svelte'
import { hex4 } from './format'

export interface DeviceRef {
  /** The device, for anything in a controller's hub tree. */
  device?: Device
  /**
   * The USB4 router, for an entry that stands for one. Routers are on a
   * separate bus with no hub port above them, so they are indexed for
   * naming and focus only and carry no port, parent or controller.
   */
  router?: USB4Router
  /** The hub port the device hangs off; null for a root hub or a router. */
  port: Port | null
  /** Normalised id of the parent hub; null for a root hub or a router. */
  parentId: string | null
  controller?: Controller
  depth: number
}

/** Normalised device id -> where it lives in the tree. */
export type DeviceIndex = ReadonlyMap<string, DeviceRef>

export function indexTopology(topology: Topology | null): DeviceIndex {
  const index = new Map<string, DeviceRef>()
  if (!topology) return index
  const visit = (controller: Controller, device: Device, port: Port | null, parentId: string | null, depth: number): void => {
    index.set(normalizeId(device.id), { device, port, parentId, controller, depth })
    for (const p of device.hub?.ports ?? []) {
      if (p.device) visit(controller, p.device, p, normalizeId(device.id), depth + 1)
    }
  }
  for (const c of topology.controllers) {
    if (c.root_hub) visit(c, c.root_hub, null, null, 0)
  }
  // Routers too: an insight can name one, and a USB4 device that tunnels
  // PCIe is only ever a router, so this is the only place it can be found.
  for (const router of topology.usb4 ?? []) {
    index.set(normalizeId(router.id), { router, port: null, parentId: null, depth: router.depth })
  }
  return index
}

/** Normalised ids of every hub between the device and its controller, nearest first. */
export function ancestorIds(index: DeviceIndex, id: string): string[] {
  const out: string[] = []
  let current = index.get(normalizeId(id))?.parentId ?? null
  while (current) {
    out.push(current)
    current = index.get(current)?.parentId ?? null
  }
  return out
}

function firstNonEmpty(...values: Array<string | undefined>): string {
  for (const v of values) if (v && v.trim()) return v.trim()
  return ''
}

/**
 * Friendliest name: knowledge-base vendor + product, then string
 * descriptors, then what the OS calls it.
 */
export function deviceName(d: Device): string {
  const product = firstNonEmpty(d.product_name, d.product, d.friendly_name, d.description)
  const vendor = firstNonEmpty(d.vendor_name, d.manufacturer)
  if (!product && !vendor) return t('device.unknownName', { vid: hex4(d.vendor_id), pid: hex4(d.product_id) })
  if (!vendor) return product
  if (!product) return t('device.vendorOnly', { vendor })
  if (product.toLowerCase().startsWith(vendor.toLowerCase())) return product
  return `${vendor} ${product}`
}

/**
 * Friendliest name for a USB4 router.
 *
 * A router announces the bridge silicon it is built on, so a USB4 SSD calls
 * itself its controller chip. The knowledge base maps that back to the
 * product, and the OS name is the fallback.
 */
export function routerName(router: USB4Router): string {
  return router.product_name?.trim() || router.name
}

/** Name for whatever an index entry stands for. */
export function refName(ref: DeviceRef): string {
  if (ref.device) return deviceName(ref.device)
  return ref.router ? routerName(ref.router) : ''
}

/** Name for an id, or null when the id is not in the snapshot. */
export function nameForId(index: DeviceIndex, id: string): string | null {
  const ref = index.get(normalizeId(id))
  return ref ? refName(ref) : null
}

export function countDevices(topology: Topology | null): number {
  let n = 0
  for (const ref of indexTopology(topology).values()) if (ref.device) n++
  return n
}

export interface ChildEntry {
  port: Port
  device: Device
}

/** Ports with a device attached, in port order, with the device unwrapped. */
export function childEntries(device: Device): ChildEntry[] {
  const out: ChildEntry[] = []
  for (const port of device.hub?.ports ?? []) {
    if (port.device) out.push({ port, device: port.device })
  }
  return out
}
