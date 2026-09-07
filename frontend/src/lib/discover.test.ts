import { describe, expect, it, vi } from 'vitest'
import { browserApiBase, browserDiscovery, isWailsHost, pollStatus, type AppStatusLike } from './discover'

const status = (over: Partial<AppStatusLike> = {}): AppStatusLike => ({
  ready: false, api_base: '', stream_url: '', app_name: 'Port Authority', version: '0.1.0', platform: 'windows', ...over,
})

describe('browser discovery', () => {
  it('defaults to loopback and honours ?api=', () => {
    expect(browserApiBase('')).toBe('http://127.0.0.1:7911')
    expect(browserApiBase('?api=http://localhost:9000/')).toBe('http://localhost:9000')
    expect(browserDiscovery('?api=http://localhost:9000').streamUrl).toBe('ws://localhost:9000/api/v1/stream')
  })

  it('detects the Wails host by window.go', () => {
    expect(isWailsHost({ go: undefined })).toBe(false)
    expect(isWailsHost({ go: {} })).toBe(true)
  })
})

describe('pollStatus', () => {
  it('polls until ready, reporting each status', async () => {
    const seen: boolean[] = []
    const statuses = [status(), status({ error: 'warming up' }), status({ ready: true, api_base: 'http://127.0.0.1:7911/', stream_url: '' })]
    const fn = vi.fn(async () => statuses.shift() ?? status({ ready: true, api_base: 'http://x' }))
    const found = await pollStatus(fn, { intervalMs: 1, sleep: async () => {}, onUpdate: (s) => seen.push(s.ready) })
    expect(fn).toHaveBeenCalledTimes(3)
    expect(seen).toEqual([false, false, true])
    expect(found).toEqual({
      host: 'wails', apiBase: 'http://127.0.0.1:7911', streamUrl: 'ws://127.0.0.1:7911/api/v1/stream', appName: 'Port Authority', version: '0.1.0', platform: 'windows',
    })
  })

  it('prefers the stream url the host gives', async () => {
    const found = await pollStatus(async () => status({ ready: true, api_base: 'http://a', stream_url: 'ws://custom/stream' }), { sleep: async () => {} })
    expect(found.streamUrl).toBe('ws://custom/stream')
  })

  it('keeps polling through thrown errors and stops on abort', async () => {
    const failures: string[] = []
    const ac = new AbortController()
    let calls = 0
    const fn = async (): Promise<AppStatusLike> => {
      calls++
      if (calls === 3) ac.abort()
      throw new Error('bridge not ready')
    }
    await expect(pollStatus(fn, { sleep: async () => {}, signal: ac.signal, onFailure: (e) => failures.push(e.message) })).rejects.toThrow('aborted')
    expect(failures).toEqual(['bridge not ready', 'bridge not ready', 'bridge not ready'])
  })
})
