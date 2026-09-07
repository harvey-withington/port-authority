// Latest throughput sample per device. Keys are normalised device ids
// because the ETW stream upper-cases them while the topology does not.
import type { ThroughputSample } from './api/types'
import { normalizeId } from './ids'

/** A device with no sample for this long is shown as idle. */
export const THROUGHPUT_IDLE_MS = 3_000

export interface ThroughputEntry {
  sample: ThroughputSample
  /** Local clock (ms) when the sample arrived; decay is measured against this. */
  seenAt: number
}

export type ThroughputMap = ReadonlyMap<string, ThroughputEntry>

export function emptyThroughput(): ThroughputMap {
  return new Map()
}

/** Returns a new map with the sample recorded under its normalised id. */
export function applySample(map: ThroughputMap, sample: ThroughputSample, now: number): ThroughputMap {
  const next = new Map(map)
  next.set(normalizeId(sample.device_id), { sample, seenAt: now })
  return next
}

export function applySamples(map: ThroughputMap, samples: readonly ThroughputSample[], now: number): ThroughputMap {
  if (samples.length === 0) return map
  const next = new Map(map)
  for (const s of samples) next.set(normalizeId(s.device_id), { sample: s, seenAt: now })
  return next
}

/**
 * Drops entries older than maxAge. Returns the same map instance when
 * nothing changed so reactive consumers do not re-render needlessly.
 */
export function pruneStale(map: ThroughputMap, now: number, maxAge = THROUGHPUT_IDLE_MS): ThroughputMap {
  let changed = false
  const next = new Map<string, ThroughputEntry>()
  for (const [id, entry] of map) {
    if (now - entry.seenAt > maxAge) changed = true
    else next.set(id, entry)
  }
  return changed ? next : map
}

export function sampleFor(map: ThroughputMap, deviceId: string): ThroughputSample | null {
  return map.get(normalizeId(deviceId))?.sample ?? null
}
