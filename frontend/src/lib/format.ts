// Human formatting. Every string goes through t() so locales can adjust
// units and phrasing.
import { t } from './i18n.svelte'
import type { LinkSpeed } from './api/types'

const KILO = 1_000
const MEGA = 1_000_000
const GIGA = 1_000_000_000

/** Three significant digits with trailing zeros trimmed, like Go's %.3g. */
function sig3(n: number): string {
  return Number(n.toPrecision(3)).toString()
}

/** Bandwidth in bits per second: "10 Gbps", "480 Mbps", "1.5 Mbps". */
export function formatBitrate(bps: number): string {
  if (bps >= GIGA) return t('format.gbps', { n: sig3(bps / GIGA) })
  if (bps >= MEGA) return t('format.mbps', { n: sig3(bps / MEGA) })
  if (bps >= KILO) return t('format.kbps', { n: sig3(bps / KILO) })
  return t('format.bps', { n: Math.round(bps) })
}

/** Throughput given in bits per second, shown in bytes: "134.2 MB/s", "820 KB/s", "idle". */
export function formatThroughput(bps: number): string {
  if (!Number.isFinite(bps) || bps <= 0) return t('format.idle')
  const bytes = bps / 8
  if (bytes >= GIGA) return t('format.gbPerSec', { n: (bytes / GIGA).toFixed(2) })
  if (bytes >= MEGA) return t('format.mbPerSec', { n: (bytes / MEGA).toFixed(1) })
  if (bytes >= KILO) return t('format.kbPerSec', { n: Math.round(bytes / KILO) })
  return t('format.bPerSec', { n: Math.max(1, Math.round(bytes)) })
}

/** Short badge text for a link: "10 Gbps", "USB 2". */
export function linkLabel(speed: LinkSpeed): string {
  return t(`link.short.${speed}`)
}

/** Long description mirroring core/insight/rules.go speedWord. */
export function linkDescription(speed: LinkSpeed): string {
  return t(`link.long.${speed}`)
}

export function formatPower(milliamps: number): string {
  return t('format.milliamps', { n: milliamps })
}

/** 0..1 confidence as a percentage: "90%". */
export function formatConfidence(confidence: number): string {
  const pct = Math.round(Math.min(1, Math.max(0, confidence)) * 100)
  return t('format.percent', { n: pct })
}

/** Relative time for a timestamp: "just now", "12 s ago", "3 min ago". */
export function relativeTime(iso: string, now: number = Date.now()): string {
  const then = Date.parse(iso)
  if (Number.isNaN(then)) return ''
  const seconds = Math.max(0, Math.round((now - then) / 1000))
  if (seconds < 5) return t('time.justNow')
  if (seconds < 60) return t('time.secondsAgo', { n: seconds })
  const minutes = Math.round(seconds / 60)
  if (minutes < 60) return t('time.minutesAgo', { n: minutes })
  const hours = Math.round(minutes / 60)
  if (hours < 24) return t('time.hoursAgo', { n: hours })
  return t('time.daysAgo', { n: Math.round(hours / 24) })
}

/** Local wall-clock time for a timestamp, e.g. "14:05:09". */
export function clockTime(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

export function hex4(n: number): string {
  return n.toString(16).padStart(4, '0')
}
