// Where is the API? Inside Wails the Go side tells us via Status(); in a
// plain browser we default to loopback, overridable with ?api= on the URL.
import { DEFAULT_API_BASE, normalizeBase, streamUrlFor } from './api/client'

/** Shape of wailsjs main.AppStatus, kept structural so tests need no Wails. */
export interface AppStatusLike {
  ready: boolean
  error?: string
  api_base: string
  stream_url: string
  app_name: string
  version: string
  platform: string
}

export type HostKind = 'wails' | 'browser'

export interface Discovered {
  host: HostKind
  apiBase: string
  streamUrl: string
  appName: string | null
  version: string | null
  platform: string | null
}

export function isWailsHost(win: Pick<Window, 'go'> = window): boolean {
  return typeof win.go === 'object' && win.go !== null
}

/** API base for browser development: ?api=http://host:port or the default. */
export function browserApiBase(search: string, fallback = DEFAULT_API_BASE): string {
  const override = new URLSearchParams(search).get('api')
  return normalizeBase(override && override.trim() ? override : fallback)
}

export function browserDiscovery(search: string): Discovered {
  const apiBase = browserApiBase(search)
  return { host: 'browser', apiBase, streamUrl: streamUrlFor(apiBase), appName: null, version: null, platform: null }
}

export interface PollOptions {
  intervalMs?: number
  /** Called after every poll so the UI can show progress and errors. */
  onUpdate?: (status: AppStatusLike) => void
  /** Called when Status() itself throws; polling continues. */
  onFailure?: (error: Error) => void
  signal?: AbortSignal
  sleep?: (ms: number) => Promise<void>
}

function defaultSleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

/** Polls Status() until it reports ready. Rejects only when aborted. */
export async function pollStatus(status: () => Promise<AppStatusLike>, opts: PollOptions = {}): Promise<Discovered> {
  const interval = opts.intervalMs ?? 500
  const sleep = opts.sleep ?? defaultSleep
  for (;;) {
    if (opts.signal?.aborted) throw new Error('aborted')
    try {
      const s = await status()
      opts.onUpdate?.(s)
      if (s.ready && s.api_base) {
        const apiBase = normalizeBase(s.api_base)
        return {
          host: 'wails',
          apiBase,
          streamUrl: s.stream_url ? s.stream_url : streamUrlFor(apiBase),
          appName: s.app_name || null,
          version: s.version || null,
          platform: s.platform || null,
        }
      }
    } catch (e) {
      opts.onFailure?.(e instanceof Error ? e : new Error(String(e)))
    }
    await sleep(interval)
  }
}
