import { describe, expect, it, vi } from 'vitest'
import { ApiError, createClient, normalizeBase, streamUrlFor } from './client'

const json = (body: unknown, status = 200): Response =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })

describe('urls', () => {
  it('normalises the base and derives the stream url', () => {
    expect(normalizeBase('http://127.0.0.1:7911///')).toBe('http://127.0.0.1:7911')
    expect(streamUrlFor('http://127.0.0.1:7911')).toBe('ws://127.0.0.1:7911/api/v1/stream')
    expect(streamUrlFor('https://host/prefix/')).toBe('wss://host/prefix/api/v1/stream')
  })

  it('url-encodes device ids', async () => {
    const fetchFn = vi.fn(async (url: string) => json({ schema_version: 1, captured_at: '', device: { id: url }, insights: [] }))
    const client = createClient('http://x', fetchFn)
    await client.device('USB\\VID_1B1C&PID_1A20\\MSFT30SCRUBBED-06')
    expect(fetchFn.mock.calls[0][0]).toBe('http://x/api/v1/devices/USB%5CVID_1B1C%26PID_1A20%5CMSFT30SCRUBBED-06')
  })
})

describe('knowledge base writes', () => {
  it('posts a dock as JSON and deletes by encoded id', async () => {
    const fetchFn = vi.fn(async (url: string, init?: RequestInit) => {
      if (init?.method === 'POST') return json({ schema_version: 1, dock: { id: 'local:acme', name: 'Acme', hubs: ['1234:0001'], source: 'local' } }, 201)
      return json({ schema_version: 1, ok: true })
    })
    const client = createClient('http://x', fetchFn)
    const created = await client.createDock({ name: 'Acme', hubs: ['1234:0001'] })
    expect(created.dock.id).toBe('local:acme')
    const [url, init] = fetchFn.mock.calls[0]
    expect(url).toBe('http://x/api/v1/kb/docks')
    expect(init?.method).toBe('POST')
    expect(init?.body).toBe(JSON.stringify({ name: 'Acme', hubs: ['1234:0001'] }))
    expect((init?.headers as Record<string, string>)['Content-Type']).toBe('application/json')
    await client.deleteDock('local:acme')
    expect(fetchFn.mock.calls[1][0]).toBe('http://x/api/v1/kb/docks/local%3Aacme')
    expect(fetchFn.mock.calls[1][1]?.method).toBe('DELETE')
  })
})

describe('errors', () => {
  it('surfaces the service error message with its status', async () => {
    const client = createClient('http://x', async () => json({ error: 'snapshot failed: boom' }, 502))
    await expect(client.topology()).rejects.toMatchObject({ name: 'ApiError', status: 502, message: 'snapshot failed: boom' })
  })

  it('wraps network failures as status 0', async () => {
    const client = createClient('http://x', async () => {
      throw new TypeError('Failed to fetch')
    })
    const err = await client.health().catch((e: unknown) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect((err as ApiError).status).toBe(0)
    expect((err as ApiError).url).toBe('http://x/api/v1/health')
  })

  it('copes with a non-JSON error body', async () => {
    const client = createClient('http://x', async () => new Response('gateway', { status: 504, statusText: 'Gateway Timeout' }))
    await expect(client.insights()).rejects.toMatchObject({ status: 504, message: '504 Gateway Timeout' })
  })
})
