import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createLive, type LiveStore, type WebSocketLike } from './live.svelte'
import type { ProviderCaps, StreamEvent, TopologyResponse } from './api/types'
import { insight, sampleTopology } from './fixtures.test-helpers'

const caps: ProviderCaps = {
  platform: 'test', topology: true, hotplug: true, throughput: true, alt_mode: false,
  power_draw: true, iso_reservation: true, connector_info: true, string_descriptors: true,
}

class FakeSocket implements WebSocketLike {
  onopen: ((ev: Event) => void) | null = null
  onmessage: ((ev: MessageEvent) => void) | null = null
  onclose: ((ev: CloseEvent) => void) | null = null
  onerror: ((ev: Event) => void) | null = null
  closed = false
  constructor(readonly url: string) {}
  close(): void {
    this.closed = true
  }
  open(): void {
    this.onopen?.(new Event('open'))
  }
  send(event: StreamEvent): void {
    this.onmessage?.(new MessageEvent('message', { data: JSON.stringify(event) }))
  }
  drop(): void {
    this.onclose?.(new CloseEvent('close'))
  }
}

// A real Response's body stream does not settle under fake timers, so the
// mock answers with the subset of Response the client reads.
function reply(body: unknown, status = 200): Response {
  const partial: Pick<Response, 'ok' | 'status' | 'statusText' | 'json'> = {
    ok: status >= 200 && status < 300,
    status,
    statusText: status === 200 ? 'OK' : 'Error',
    json: () => Promise.resolve(body),
  }
  return partial as Response
}

interface Harness {
  store: LiveStore
  sockets: FakeSocket[]
  fetchMock: ReturnType<typeof vi.fn>
  topologyOk: { value: boolean }
  flush: () => Promise<void>
}

function harness(): Harness {
  const sockets: FakeSocket[] = []
  const topologyOk = { value: true }
  const fetchMock = vi.fn(async (url: string): Promise<Response> => {
    if (url.endsWith('/capabilities')) return reply({ schema_version: 1, capabilities: caps })
    if (url.endsWith('/topology')) {
      if (!topologyOk.value) return reply({ error: 'snapshot failed: provider down' }, 502)
      const body: TopologyResponse = { schema_version: 1, captured_at: '2026-09-07T00:00:00Z', topology: sampleTopology(), insights: [insight()] }
      return reply(body)
    }
    if (url.endsWith('/throughput')) return reply({ schema_version: 1, window_seconds: 5, samples: [] })
    return reply({ error: 'no such endpoint' }, 404)
  })
  const store = createLive('http://x', 'ws://x/api/v1/stream', {
    fetch: fetchMock,
    createSocket: (url) => {
      const s = new FakeSocket(url)
      sockets.push(s)
      return s
    },
    now: () => Date.now(),
    pollIntervalMs: 5000,
    reconnectBaseMs: 1000,
    reconnectMaxMs: 8000,
    decayTickMs: 1000,
  })
  const flush = async (): Promise<void> => {
    await vi.advanceTimersByTimeAsync(0)
  }
  return { store, sockets, fetchMock, topologyOk, flush }
}

const at = '2026-09-07T00:00:01Z'

describe('live store', () => {
  let h: Harness

  beforeEach(() => {
    vi.useFakeTimers()
    h = harness()
  })

  afterEach(() => {
    h.store.stop()
    vi.useRealTimers()
  })

  it('starts connecting, loads the snapshot and goes live when the socket opens', async () => {
    expect(h.store.state).toBe('starting')
    h.store.start()
    expect(h.store.state).toBe('connecting')
    await h.flush()
    expect(h.store.topology?.controllers).toHaveLength(1)
    expect(h.store.insights).toHaveLength(1)
    expect(h.store.capabilities?.throughput).toBe(true)
    expect(h.store.index.size).toBe(4)
    h.sockets[0].open()
    expect(h.store.state).toBe('live')
    expect(h.store.timeline[0]).toMatchObject({ kind: 'connection', connection: 'live' })
  })

  it('falls back to polling with backoff when the socket drops, and recovers', async () => {
    h.store.start()
    await h.flush()
    h.sockets[0].open()
    h.fetchMock.mockClear()

    h.sockets[0].drop()
    await h.flush()
    expect(h.store.state).toBe('polling')
    expect(h.store.timeline[0]).toMatchObject({ kind: 'connection', connection: 'polling' })
    const topologyCalls = (): number => h.fetchMock.mock.calls.filter((c) => String(c[0]).endsWith('/topology')).length
    expect(topologyCalls()).toBe(1)

    await vi.advanceTimersByTimeAsync(5000)
    expect(topologyCalls()).toBe(2)

    // Reconnect attempts: 1 s, then 2 s.
    await vi.advanceTimersByTimeAsync(1000)
    expect(h.sockets).toHaveLength(2)
    h.sockets[1].drop()
    await vi.advanceTimersByTimeAsync(1999)
    expect(h.sockets).toHaveLength(2)
    await vi.advanceTimersByTimeAsync(1)
    expect(h.sockets).toHaveLength(3)

    h.sockets[2].open()
    expect(h.store.state).toBe('live')
    const before = topologyCalls()
    await vi.advanceTimersByTimeAsync(10_000)
    expect(topologyCalls()).toBe(before)
  })

  it('reports offline with the service error when polling fails too', async () => {
    h.topologyOk.value = false
    h.store.start()
    await h.flush()
    expect(h.store.state).toBe('offline')
    expect(h.store.error).toBe('snapshot failed: provider down')
    expect(h.store.topology).toBeNull()

    h.topologyOk.value = true
    h.store.retry()
    await h.flush()
    expect(h.store.error).toBeNull()
    expect(h.store.topology).not.toBeNull()
  })

  it('applies stream events: hello, insights, topology changes and samples', async () => {
    h.store.start()
    await h.flush()
    const ws = h.sockets[0]
    ws.open()

    ws.send({ type: 'hello', at, data: { schema_version: 1, capabilities: { ...caps, hotplug: false }, captured_at: at, insights: [] } })
    expect(h.store.capabilities?.hotplug).toBe(false)
    expect(h.store.insights).toHaveLength(0)

    const added = insight({ rule_id: 'usb2-on-dock', severity: 'critical', title: 'Dock dropped to USB 2' })
    ws.send({ type: 'insight_added', at, data: added })
    expect(h.store.insights.map((i) => i.rule_id)).toEqual(['usb2-on-dock'])
    expect(h.store.timeline[0]).toMatchObject({ kind: 'insight_added', severity: 'critical', subject: 'Dock dropped to USB 2' })

    ws.send({ type: 'insight_resolved', at, data: added })
    expect(h.store.insights).toHaveLength(0)
    expect(h.store.timeline[0].kind).toBe('insight_resolved')

    ws.send({
      type: 'topology_changed', at,
      data: { events: [{ kind: 'device_added', at, device_id: 'usb\\vid_1b1c&pid_1a20\\ssd' }], captured_at: at, fetched_at: at, controllers: 1, devices: 4 },
    })
    await h.flush()
    expect(h.store.timeline[0]).toMatchObject({ kind: 'device_added', subject: 'Corsair EX400U' })
    // The refetched snapshot carries the insight list again.
    expect(h.store.insights).toHaveLength(1)

    ws.send({ type: 'throughput_sample', at, data: { device_id: 'USB\\VID_1B1C&PID_1A20\\SSD', at, read_bps: 8_000_000, write_bps: 0 } })
    expect(h.store.throughput.get('USB\\VID_1B1C&PID_1A20\\SSD')?.sample.read_bps).toBe(8_000_000)
  })

  it('decays throughput to idle after three seconds without samples', async () => {
    h.store.start()
    await h.flush()
    const ws = h.sockets[0]
    ws.open()
    ws.send({ type: 'throughput_sample', at, data: { device_id: 'USB\\A', at, read_bps: 1, write_bps: 1 } })
    expect(h.store.throughput.size).toBe(1)
    await vi.advanceTimersByTimeAsync(2000)
    expect(h.store.throughput.size).toBe(1)
    await vi.advanceTimersByTimeAsync(2000)
    expect(h.store.throughput.size).toBe(0)
  })

  it('ignores malformed messages', async () => {
    h.store.start()
    await h.flush()
    const ws = h.sockets[0]
    ws.open()
    const before = h.store.timeline.length
    ws.onmessage?.(new MessageEvent('message', { data: 'not json' }))
    ws.onmessage?.(new MessageEvent('message', { data: JSON.stringify({ type: 'bogus', at, data: {} }) }))
    expect(h.store.state).toBe('live')
    expect(h.store.timeline).toHaveLength(before)
  })

  it('stops cleanly: closes the socket and no longer polls', async () => {
    h.store.start()
    await h.flush()
    h.sockets[0].open()
    h.store.stop()
    expect(h.sockets[0].closed).toBe(true)
    const calls = h.fetchMock.mock.calls.length
    await vi.advanceTimersByTimeAsync(20_000)
    expect(h.fetchMock.mock.calls.length).toBe(calls)
  })
})
