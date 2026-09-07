import { describe, expect, it } from 'vitest'
import { THROUGHPUT_IDLE_MS, applySample, applySamples, emptyThroughput, pruneStale, sampleFor } from './throughput'
import type { ThroughputSample } from './api/types'

const sample = (device_id: string, read_bps = 1000, write_bps = 0): ThroughputSample => ({ device_id, at: '2026-09-07T00:00:00Z', read_bps, write_bps })

describe('throughput map', () => {
  it('keys by normalised id so upper-cased stream ids match the topology', () => {
    const map = applySample(emptyThroughput(), sample('USB\\VID_1B1C&PID_1A20\\MSFT30SCRUBBED-06'), 1000)
    expect(sampleFor(map, 'usb\\vid_1b1c&pid_1a20\\msft30scrubbed-06')?.read_bps).toBe(1000)
    expect(sampleFor(map, 'other')).toBeNull()
  })

  it('replaces the previous sample for the same device without mutating the old map', () => {
    const first = applySample(emptyThroughput(), sample('a', 1), 0)
    const second = applySample(first, sample('A', 2), 10)
    expect(sampleFor(first, 'a')?.read_bps).toBe(1)
    expect(sampleFor(second, 'a')?.read_bps).toBe(2)
    expect(second.size).toBe(1)
  })

  it('applies a batch and returns the same map for an empty batch', () => {
    const base = emptyThroughput()
    expect(applySamples(base, [], 0)).toBe(base)
    expect(applySamples(base, [sample('a'), sample('b')], 0).size).toBe(2)
  })

  it('decays to idle after the idle window and keeps identity when nothing changed', () => {
    let map = applySample(emptyThroughput(), sample('a'), 0)
    map = applySample(map, sample('b'), 2000)
    expect(pruneStale(map, 2500)).toBe(map)
    const later = pruneStale(map, THROUGHPUT_IDLE_MS + 1)
    expect(later).not.toBe(map)
    expect(sampleFor(later, 'a')).toBeNull()
    expect(sampleFor(later, 'b')).not.toBeNull()
    expect(pruneStale(later, 2000 + THROUGHPUT_IDLE_MS + 1).size).toBe(0)
  })
})
