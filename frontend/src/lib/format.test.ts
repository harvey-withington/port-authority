import { describe, expect, it } from 'vitest'
import { formatBitrate, formatConfidence, formatPower, formatThroughput, linkDescription, linkLabel, relativeTime } from './format'

describe('formatBitrate', () => {
  it('picks the unit like Go Bitrate.String', () => {
    expect(formatBitrate(10_000_000_000)).toBe('10 Gbps')
    expect(formatBitrate(480_000_000)).toBe('480 Mbps')
    expect(formatBitrate(1_500_000)).toBe('1.5 Mbps')
    expect(formatBitrate(12_000_000)).toBe('12 Mbps')
    expect(formatBitrate(2_500)).toBe('2.5 Kbps')
    expect(formatBitrate(300)).toBe('300 bps')
  })
})

describe('formatThroughput', () => {
  it('shows bytes per second from bits per second', () => {
    expect(formatThroughput(134.2 * 8 * 1_000_000)).toBe('134.2 MB/s')
    expect(formatThroughput(820 * 8 * 1_000)).toBe('820 KB/s')
    expect(formatThroughput(1.25 * 8 * 1_000_000_000)).toBe('1.25 GB/s')
    expect(formatThroughput(400)).toBe('50 B/s')
  })

  it('is idle at zero or nonsense', () => {
    expect(formatThroughput(0)).toBe('idle')
    expect(formatThroughput(-5)).toBe('idle')
    expect(formatThroughput(Number.NaN)).toBe('idle')
  })
})

describe('link labels', () => {
  it('mirrors rules.go speedWord for the long form', () => {
    expect(linkDescription('high')).toBe('USB 2 speed (480 Mbps)')
    expect(linkDescription('ss10')).toBe('10 Gbps')
    expect(linkDescription('usb4_40')).toBe('40 Gbps (USB4 / Thunderbolt)')
    expect(linkDescription('usb4_80')).toBe('80 Gbps (USB4 v2)')
  })

  it('has a short badge form', () => {
    expect(linkLabel('high')).toBe('USB 2')
    expect(linkLabel('ss10')).toBe('10 Gbps')
    expect(linkLabel('none')).toBe('empty')
  })
})

describe('relativeTime', () => {
  const now = Date.parse('2026-09-07T10:00:00Z')

  it('buckets by magnitude', () => {
    expect(relativeTime('2026-09-07T09:59:58Z', now)).toBe('just now')
    expect(relativeTime('2026-09-07T09:59:30Z', now)).toBe('30 s ago')
    expect(relativeTime('2026-09-07T09:45:00Z', now)).toBe('15 min ago')
    expect(relativeTime('2026-09-07T07:00:00Z', now)).toBe('3 h ago')
    expect(relativeTime('2026-09-04T10:00:00Z', now)).toBe('3 d ago')
  })

  it('never goes negative and tolerates garbage', () => {
    expect(relativeTime('2026-09-07T10:00:05Z', now)).toBe('just now')
    expect(relativeTime('not a date', now)).toBe('')
  })
})

describe('small formatters', () => {
  it('formats power and confidence', () => {
    expect(formatPower(500)).toBe('500 mA')
    expect(formatConfidence(0.9)).toBe('90%')
    expect(formatConfidence(1.7)).toBe('100%')
  })
})
