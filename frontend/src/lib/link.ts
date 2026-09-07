import type { LinkSpeed, Port, Device } from './api/types'

/** Link speeds in ascending order; the index doubles as a rank. */
export const LINK_ORDER: readonly LinkSpeed[] = [
  'unknown', 'none', 'low', 'full', 'high', 'ss5', 'ss10', 'ss20', 'usb4_20', 'usb4_40', 'usb4_80',
]

const BITRATE: Record<LinkSpeed, number> = {
  unknown: 0,
  none: 0,
  low: 1_500_000,
  full: 12_000_000,
  high: 480_000_000,
  ss5: 5_000_000_000,
  ss10: 10_000_000_000,
  ss20: 20_000_000_000,
  usb4_20: 20_000_000_000,
  usb4_40: 40_000_000_000,
  usb4_80: 80_000_000_000,
}

export function linkRank(speed: LinkSpeed): number {
  return LINK_ORDER.indexOf(speed)
}

/** Nominal signalling rate in bits per second (0 when unknown / none). */
export function linkBitrate(speed: LinkSpeed): number {
  return BITRATE[speed] ?? 0
}

export function isLinkKnown(speed: LinkSpeed): boolean {
  return speed !== 'unknown' && speed !== 'none'
}

export type LinkHealth = 'good' | 'slow' | 'idle'

/**
 * good  when the negotiated link is at least min(claimed, port max),
 * slow  when it is below that,
 * idle  when nothing usable is known about the link.
 * An unknown claim falls back to the port maximum alone.
 */
export function linkHealth(
  port: Pick<Port, 'negotiated_link' | 'max_link'>,
  device: Pick<Device, 'claimed_speed'>,
): LinkHealth {
  const negotiated = port.negotiated_link
  if (!isLinkKnown(negotiated)) return 'idle'
  const claimed = device.claimed_speed
  const max = port.max_link
  const claimedKnown = isLinkKnown(claimed)
  const maxKnown = isLinkKnown(max)
  if (!claimedKnown && !maxKnown) return 'idle'
  let target: LinkSpeed
  if (claimedKnown && maxKnown) target = linkRank(claimed) < linkRank(max) ? claimed : max
  else target = claimedKnown ? claimed : max
  return linkRank(negotiated) >= linkRank(target) ? 'good' : 'slow'
}
