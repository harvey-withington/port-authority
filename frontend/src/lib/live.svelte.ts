// The live picture of the machine: connection state, the current
// snapshot, insights, throughput and the "what changed" timeline.
//
// Transport: a WebSocket on /api/v1/stream with exponential-backoff
// reconnect; while it is down, /topology is polled every few seconds.
// Everything time- or network-related is injectable so the store can be
// unit-tested with fakes.
import type {
  Insight, ProviderCaps, StreamEvent, Topology, TopologyChangedData, HelloData, ThroughputSample,
} from './api/types'
import { isStreamEvent } from './api/types'
import { createClient, type ApiClient, type FetchLike } from './api/client'
import type { ConnectionState } from './connection'
import { addInsight, removeInsight, sortInsights } from './insights'
import { indexTopology, nameForId, type DeviceIndex } from './topology'
import {
  createIdSource, entriesFromTopologyChanged, entryFromConnection, entryFromInsight,
  prependEntries, type TimelineEntry,
} from './timeline'
import { applySample, applySamples, emptyThroughput, pruneStale, type ThroughputMap } from './throughput'

export interface WebSocketLike {
  onopen: ((ev: Event) => void) | null
  onmessage: ((ev: MessageEvent) => void) | null
  onclose: ((ev: CloseEvent) => void) | null
  onerror: ((ev: Event) => void) | null
  close(code?: number, reason?: string): void
}

export type WebSocketFactory = (url: string) => WebSocketLike

export interface LiveDeps {
  fetch?: FetchLike
  createSocket?: WebSocketFactory
  now?: () => number
  pollIntervalMs?: number
  reconnectBaseMs?: number
  reconnectMaxMs?: number
  decayTickMs?: number
}

export interface LiveStore {
  readonly state: ConnectionState
  readonly topology: Topology | null
  readonly index: DeviceIndex
  readonly insights: Insight[]
  readonly capabilities: ProviderCaps | null
  readonly throughput: ThroughputMap
  readonly timeline: TimelineEntry[]
  /** Last service error the user should know about, or null. */
  readonly error: string | null
  readonly capturedAt: string | null
  readonly apiBase: string
  start(): void
  stop(): void
  /** Reconnects immediately, resetting the backoff. */
  retry(): void
}

export const POLL_INTERVAL_MS = 5_000
export const RECONNECT_BASE_MS = 1_000
export const RECONNECT_MAX_MS = 30_000
const DECAY_TICK_MS = 1_000

function defaultSocket(url: string): WebSocketLike {
  return new WebSocket(url)
}

function errorMessage(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

export function createLive(apiBase: string, streamUrl: string, deps: LiveDeps = {}): LiveStore {
  const client: ApiClient = createClient(apiBase, deps.fetch)
  const createSocket = deps.createSocket ?? defaultSocket
  const now = deps.now ?? (() => Date.now())
  const pollInterval = deps.pollIntervalMs ?? POLL_INTERVAL_MS
  const reconnectBase = deps.reconnectBaseMs ?? RECONNECT_BASE_MS
  const reconnectMax = deps.reconnectMaxMs ?? RECONNECT_MAX_MS
  const decayTick = deps.decayTickMs ?? DECAY_TICK_MS
  const nextId = createIdSource()

  let state = $state.raw<ConnectionState>('starting')
  let topology = $state.raw<Topology | null>(null)
  let index = $state.raw<DeviceIndex>(new Map())
  let insights = $state.raw<Insight[]>([])
  let capabilities = $state.raw<ProviderCaps | null>(null)
  let throughput = $state.raw<ThroughputMap>(emptyThroughput())
  let timeline = $state.raw<TimelineEntry[]>([])
  let error = $state.raw<string | null>(null)
  let capturedAt = $state.raw<string | null>(null)

  let running = false
  /** Increments per /topology request, so a stale response can be ignored. */
  let topologyRequest = 0
  let socket: WebSocketLike | null = null
  let attempt = 0
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null
  let pollTimer: ReturnType<typeof setInterval> | null = null
  let decayTimer: ReturnType<typeof setInterval> | null = null

  const nowIso = (): string => new Date(now()).toISOString()

  const setState = (next: ConnectionState): void => {
    if (next === state) return
    state = next
    if (next === 'live' || next === 'polling' || next === 'offline') {
      timeline = prependEntries(timeline, [entryFromConnection(nowIso(), next, nextId)])
    }
  }

  const applyTopology = (t: Topology, list: Insight[], at: string): void => {
    topology = t
    index = indexTopology(t)
    insights = sortInsights(list)
    capturedAt = at
  }

  /** Fetches /topology; returns true on success. Errors are surfaced, never swallowed. */
  const refreshTopology = async (): Promise<boolean> => {
    // A burst of hotplug events starts several of these at once. Responses
    // can land out of order, and an older one must not overwrite a newer
    // snapshot: that would leave the UI on a half-connected dock.
    const request = ++topologyRequest
    try {
      const res = await client.topology()
      if (!running) return false
      if (request !== topologyRequest) return true
      applyTopology(res.topology, res.insights ?? [], res.captured_at)
      error = null
      return true
    } catch (e) {
      if (!running) return false
      error = errorMessage(e)
      return false
    }
  }

  const refreshCapabilities = async (): Promise<void> => {
    try {
      const res = await client.capabilities()
      if (running) capabilities = res.capabilities
    } catch (e) {
      if (running && !capabilities) error = errorMessage(e)
    }
  }

  const refreshThroughput = async (): Promise<void> => {
    if (!capabilities?.throughput) return
    try {
      const res = await client.throughput()
      if (running) throughput = applySamples(throughput, res.samples ?? [], now())
    } catch {
      // Throughput is decorative while polling; the topology poll reports connectivity.
    }
  }

  const poll = async (): Promise<void> => {
    const ok = await refreshTopology()
    if (!running || socket) return
    setState(ok ? 'polling' : 'offline')
    if (ok) await refreshThroughput()
  }

  const startPolling = (): void => {
    if (pollTimer) return
    pollTimer = setInterval(() => {
      void poll()
    }, pollInterval)
  }

  const stopPolling = (): void => {
    if (pollTimer) clearInterval(pollTimer)
    pollTimer = null
  }

  const clearReconnect = (): void => {
    if (reconnectTimer) clearTimeout(reconnectTimer)
    reconnectTimer = null
  }

  const scheduleReconnect = (): void => {
    clearReconnect()
    const delay = Math.min(reconnectMax, reconnectBase * 2 ** attempt)
    attempt += 1
    reconnectTimer = setTimeout(() => {
      reconnectTimer = null
      connectSocket()
    }, delay)
  }

  const onHello = (data: HelloData): void => {
    capabilities = data.capabilities
    insights = sortInsights(data.insights ?? [])
    if (data.error) error = data.error
  }

  const onTopologyChanged = async (at: string, data: TopologyChangedData): Promise<void> => {
    const previous = index
    await refreshTopology()
    if (!running) return
    const resolve = (id: string): string | null => nameForId(index, id) ?? nameForId(previous, id)
    timeline = prependEntries(timeline, entriesFromTopologyChanged(at, data, resolve, nextId))
  }

  const onSample = (sample: ThroughputSample): void => {
    throughput = applySample(throughput, sample, now())
  }

  const handleEvent = (ev: StreamEvent): void => {
    switch (ev.type) {
      case 'hello':
        onHello(ev.data)
        break
      case 'topology_changed':
        void onTopologyChanged(ev.at, ev.data)
        break
      case 'insight_added':
        insights = addInsight(insights, ev.data)
        timeline = prependEntries(timeline, [entryFromInsight('insight_added', ev.at, ev.data, nextId)])
        break
      case 'insight_resolved':
        insights = removeInsight(insights, ev.data)
        timeline = prependEntries(timeline, [entryFromInsight('insight_resolved', ev.at, ev.data, nextId)])
        break
      case 'throughput_sample':
        onSample(ev.data)
        break
    }
  }

  const onSocketDown = (ws: WebSocketLike): void => {
    if (socket !== ws) return
    socket = null
    if (!running) return
    setState(topology ? 'polling' : 'offline')
    startPolling()
    void poll()
    scheduleReconnect()
  }

  const connectSocket = (): void => {
    if (!running || socket) return
    let ws: WebSocketLike
    try {
      ws = createSocket(streamUrl)
    } catch (e) {
      error = errorMessage(e)
      setState(topology ? 'polling' : 'offline')
      startPolling()
      scheduleReconnect()
      return
    }
    socket = ws
    ws.onopen = () => {
      if (socket !== ws) return
      attempt = 0
      stopPolling()
      setState('live')
      if (!topology) void refreshTopology()
    }
    ws.onmessage = (msg) => {
      if (socket !== ws || typeof msg.data !== 'string') return
      let parsed: unknown
      try {
        parsed = JSON.parse(msg.data)
      } catch {
        return
      }
      if (isStreamEvent(parsed)) handleEvent(parsed)
    }
    ws.onclose = () => onSocketDown(ws)
    ws.onerror = () => onSocketDown(ws)
  }

  const start = (): void => {
    if (running) return
    running = true
    setState('connecting')
    void refreshCapabilities()
    void refreshTopology().then((ok) => {
      // If the socket has not opened by the time the first fetch returns,
      // reflect what we know so the UI is never stuck on "connecting".
      if (running && state === 'connecting') setState(ok ? 'polling' : 'offline')
    })
    connectSocket()
    decayTimer = setInterval(() => {
      throughput = pruneStale(throughput, now())
    }, decayTick)
  }

  const stop = (): void => {
    running = false
    clearReconnect()
    stopPolling()
    if (decayTimer) clearInterval(decayTimer)
    decayTimer = null
    const ws = socket
    socket = null
    ws?.close()
  }

  const retry = (): void => {
    if (!running) {
      start()
      return
    }
    attempt = 0
    clearReconnect()
    const ws = socket
    socket = null
    ws?.close()
    setState('connecting')
    void refreshTopology().then((ok) => {
      if (running && state === 'connecting') setState(ok ? 'polling' : 'offline')
    })
    void refreshCapabilities()
    connectSocket()
  }

  return {
    get state() { return state },
    get topology() { return topology },
    get index() { return index },
    get insights() { return insights },
    get capabilities() { return capabilities },
    get throughput() { return throughput },
    get timeline() { return timeline },
    get error() { return error },
    get capturedAt() { return capturedAt },
    apiBase: client.base,
    start,
    stop,
    retry,
  }
}
