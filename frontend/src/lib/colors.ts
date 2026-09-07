// Maps model values to the colour token families defined in app.css.
// Colour is a recognition aid only: every use pairs with an icon or a
// text label, never carrying meaning alone.
import type { DeviceClass, LinkSpeed } from './api/types'

export type ClassToken =
  | 'storage' | 'video' | 'audio' | 'hid' | 'network' | 'hub' | 'display' | 'wireless'
  | 'printer' | 'serial' | 'imaging' | 'composite' | 'vendor' | 'unknown'

export const CLASS_TOKENS: readonly ClassToken[] = [
  'storage', 'video', 'audio', 'hid', 'network', 'display', 'wireless', 'printer',
  'serial', 'imaging', 'vendor', 'hub', 'composite', 'unknown',
]

const CLASS_TO_TOKEN: Record<DeviceClass, ClassToken> = {
  unknown: 'unknown',
  hub: 'hub',
  storage: 'storage',
  video: 'video',
  audio: 'audio',
  hid: 'hid',
  network: 'network',
  printer: 'printer',
  serial: 'serial',
  wireless: 'wireless',
  // A billboard device is the USB identity of an alt-mode display or dock.
  billboard: 'display',
  imaging: 'imaging',
  smartcard: 'vendor',
  vendor: 'vendor',
  composite: 'composite',
}

/**
 * Token for a device class. `kbKind` is the knowledge-base kind when the
 * caller knows it (e.g. "display" for DisplayLink adapters the model only
 * sees as vendor-specific).
 */
export function classToken(cls: DeviceClass, kbKind?: string): ClassToken {
  if (kbKind === 'display') return 'display'
  return CLASS_TO_TOKEN[cls] ?? 'unknown'
}

export function classColorVar(cls: DeviceClass, kbKind?: string): string {
  return `var(--class-${classToken(cls, kbKind)})`
}

export type SpeedToken =
  | 'unknown' | 'none' | 'low' | 'full' | 'high' | 'ss5' | 'ss10' | 'ss20'
  | 'usb4-20' | 'usb4-40' | 'usb4-80'

/** Speed tokens from slowest to fastest, for the legend ramp. */
export const SPEED_RAMP: readonly LinkSpeed[] = [
  'low', 'full', 'high', 'ss5', 'ss10', 'ss20', 'usb4_20', 'usb4_40', 'usb4_80',
]

export function speedToken(speed: LinkSpeed): SpeedToken {
  return speed.replaceAll('_', '-') as SpeedToken
}

export function speedColorVar(speed: LinkSpeed): string {
  return `var(--speed-${speedToken(speed)})`
}

/** Text colour that reads on a solid fill of the speed colour. */
export function speedInkVar(speed: LinkSpeed): string {
  return `var(--speed-${speedToken(speed)}-ink)`
}

/** USB4 / Thunderbolt routers wear the 40 Gbps step. */
export const USB4_ROUTER_SPEED: LinkSpeed = 'usb4_40'
