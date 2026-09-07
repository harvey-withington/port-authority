import { describe, expect, it } from 'vitest'
import {
  TIMELINE_MAX, createIdSource, entriesFromTopologyChanged, entryFromConnection, entryFromInsight, prependEntries,
  type TimelineEntry,
} from './timeline'
import { insight } from './fixtures.test-helpers'

const entry = (id: string, at = '2026-09-07T00:00:00Z'): TimelineEntry => ({ id, at, kind: 'resnapshot', subject: '' })

describe('prependEntries', () => {
  it('puts the newest first and keeps burst order newest-first too', () => {
    const list = prependEntries([entry('old')], [entry('a'), entry('b')])
    expect(list.map((e) => e.id)).toEqual(['b', 'a', 'old'])
  })

  it('caps the list', () => {
    let list: TimelineEntry[] = []
    for (let i = 0; i < TIMELINE_MAX + 20; i++) list = prependEntries(list, [entry(`e${i}`)])
    expect(list).toHaveLength(TIMELINE_MAX)
    expect(list[0].id).toBe(`e${TIMELINE_MAX + 19}`)
  })

  it('returns a copy even with nothing to add', () => {
    const original = [entry('x')]
    const copy = prependEntries(original, [])
    expect(copy).toEqual(original)
    expect(copy).not.toBe(original)
  })
})

describe('entriesFromTopologyChanged', () => {
  const nextId = createIdSource('t')
  const at = '2026-09-07T01:00:00Z'

  it('makes one entry per hotplug event with resolved names', () => {
    const entries = entriesFromTopologyChanged(at, {
      events: [
        { kind: 'device_added', at: '2026-09-07T00:59:59Z', device_id: 'usb\\ssd' },
        { kind: 'device_removed', at: '', device_id: 'usb\\gone', detail: 'port 3' },
      ],
      captured_at: at, fetched_at: at, controllers: 1, devices: 4,
    }, (id) => (id === 'usb\\ssd' ? 'Corsair EX400U' : null), nextId)
    expect(entries).toHaveLength(2)
    expect(entries[0]).toMatchObject({ kind: 'device_added', subject: 'Corsair EX400U', deviceId: 'usb\\ssd', at: '2026-09-07T00:59:59Z' })
    expect(entries[1]).toMatchObject({ kind: 'device_removed', subject: 'usb\\gone', detail: 'port 3', at })
    expect(entries[0].id).not.toBe(entries[1].id)
  })

  it('collapses an empty burst into a resnapshot', () => {
    const entries = entriesFromTopologyChanged(at, { events: [], captured_at: at, fetched_at: at, controllers: 1, devices: 7 }, () => null, nextId)
    expect(entries).toEqual([{ id: expect.stringMatching(/^t-\d+$/), at, kind: 'resnapshot', subject: '', detail: '7' }])
  })
})

describe('insight and connection entries', () => {
  const nextId = createIdSource()

  it('carries severity, title and the first device id', () => {
    const e = entryFromInsight('insight_added', '2026-09-07T02:00:00Z', insight(), nextId)
    expect(e).toMatchObject({ kind: 'insight_added', subject: 'SSD could be faster', severity: 'warning', deviceId: 'USB\\VID_1B1C&PID_1A20\\SSD' })
  })

  it('records connection changes', () => {
    const e = entryFromConnection('2026-09-07T02:00:00Z', 'polling', nextId)
    expect(e).toMatchObject({ kind: 'connection', connection: 'polling', subject: 'polling' })
  })

  it('never repeats an id', () => {
    const ids = new Set(Array.from({ length: 50 }, () => nextId()))
    expect(ids.size).toBe(50)
  })
})
