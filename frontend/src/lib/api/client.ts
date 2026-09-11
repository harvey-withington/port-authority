import type {
  CapabilitiesResponse, DeviceResponse, DockCreatedResponse, DockEntry, DocksResponse, ErrorResponse, HealthResponse,
  InsightsResponse, ThroughputResponse, TopologyResponse,
} from './types'

export const DEFAULT_API_BASE = 'http://127.0.0.1:7911'

/** Failure talking to the service. `status` is 0 when no response arrived. */
export class ApiError extends Error {
  readonly status: number
  readonly url: string

  constructor(message: string, status: number, url: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.url = url
  }
}

export type FetchLike = (input: string, init?: RequestInit) => Promise<Response>

export interface ApiClient {
  readonly base: string
  health(signal?: AbortSignal): Promise<HealthResponse>
  capabilities(signal?: AbortSignal): Promise<CapabilitiesResponse>
  topology(signal?: AbortSignal): Promise<TopologyResponse>
  insights(signal?: AbortSignal): Promise<InsightsResponse>
  throughput(signal?: AbortSignal): Promise<ThroughputResponse>
  device(id: string, signal?: AbortSignal): Promise<DeviceResponse>
  /** Every dock the service knows, across the knowledge base layers. */
  docks(signal?: AbortSignal): Promise<DocksResponse>
  /** Adds a dock to the user's own layer; the service re-reads the machine. */
  createDock(entry: DockEntry, signal?: AbortSignal): Promise<DockCreatedResponse>
  /** Forgets one of the user's own docks. */
  deleteDock(id: string, signal?: AbortSignal): Promise<void>
}

/** Strips trailing slashes so paths can be appended verbatim. */
export function normalizeBase(base: string): string {
  return base.trim().replace(/\/+$/, '')
}

/** ws(s)://host/api/v1/stream for the given http(s) base. */
export function streamUrlFor(base: string): string {
  const url = new URL(normalizeBase(base))
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  url.pathname = `${url.pathname.replace(/\/+$/, '')}/api/v1/stream`
  url.search = ''
  url.hash = ''
  return url.toString()
}

function isErrorResponse(value: unknown): value is ErrorResponse {
  return typeof value === 'object' && value !== null && typeof (value as ErrorResponse).error === 'string'
}

function isAbort(e: unknown): boolean {
  return e instanceof Error && e.name === 'AbortError'
}

async function getJson<T>(fetchFn: FetchLike, url: string, signal?: AbortSignal): Promise<T> {
  return requestJson<T>(fetchFn, 'GET', url, undefined, signal)
}

async function requestJson<T>(fetchFn: FetchLike, method: string, url: string, payload: unknown, signal?: AbortSignal): Promise<T> {
  let res: Response
  try {
    // no-store: every endpoint reports the machine as it is right now, and
    // a body served from the browser cache is a picture of a machine that
    // has since changed.
    const headers: Record<string, string> = { Accept: 'application/json' }
    if (payload !== undefined) headers['Content-Type'] = 'application/json'
    res = await fetchFn(url, {
      method,
      signal,
      cache: 'no-store',
      headers,
      body: payload === undefined ? undefined : JSON.stringify(payload),
    })
  } catch (e) {
    if (isAbort(e)) throw e
    throw new ApiError(e instanceof Error ? e.message : String(e), 0, url)
  }
  let body: unknown = null
  try {
    body = await res.json()
  } catch {
    body = null
  }
  if (!res.ok) {
    const msg = isErrorResponse(body) ? body.error : `${res.status} ${res.statusText}`
    throw new ApiError(msg, res.status, url)
  }
  return body as T
}

export function createClient(base: string, fetchFn: FetchLike = (i, init) => fetch(i, init)): ApiClient {
  const root = `${normalizeBase(base)}/api/v1`
  return {
    base: normalizeBase(base),
    health: (signal) => getJson<HealthResponse>(fetchFn, `${root}/health`, signal),
    capabilities: (signal) => getJson<CapabilitiesResponse>(fetchFn, `${root}/capabilities`, signal),
    topology: (signal) => getJson<TopologyResponse>(fetchFn, `${root}/topology`, signal),
    insights: (signal) => getJson<InsightsResponse>(fetchFn, `${root}/insights`, signal),
    throughput: (signal) => getJson<ThroughputResponse>(fetchFn, `${root}/throughput`, signal),
    device: (id, signal) => getJson<DeviceResponse>(fetchFn, `${root}/devices/${encodeURIComponent(id)}`, signal),
    docks: (signal) => getJson<DocksResponse>(fetchFn, `${root}/kb/docks`, signal),
    createDock: (entry, signal) => requestJson<DockCreatedResponse>(fetchFn, 'POST', `${root}/kb/docks`, entry, signal),
    deleteDock: async (id, signal) => {
      await requestJson<unknown>(fetchFn, 'DELETE', `${root}/kb/docks/${encodeURIComponent(id)}`, undefined, signal)
    },
  }
}
