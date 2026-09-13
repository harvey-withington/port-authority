import { describe, expect, it, vi } from 'vitest'
import type { DockView } from './api/types'
import { COMMUNITY_REPO, openExternal, shareDockUrl, shareableEntry } from './community'

const dock: DockView = {
  id: 'local:acme-dock-9',
  source: 'local',
  name: 'Acme Dock 9',
  hubs: ['1234:0001'],
  usb4: { vendor: 'Acme', model: 'Dock 9' },
  uplink: { kind: 'usb4', max_link: 'usb4_40' },
  verified: 'user, 2026-09-12',
}

describe('sharing a dock', () => {
  it('drops the local id and stamp and keeps the rest', () => {
    expect(shareableEntry(dock)).toEqual({
      name: 'Acme Dock 9',
      hubs: ['1234:0001'],
      usb4: { vendor: 'Acme', model: 'Dock 9' },
      uplink: { kind: 'usb4', max_link: 'usb4_40' },
    })
  })

  it('builds the new-issue URL with the form fields filled in', () => {
    const url = new URL(shareDockUrl(dock))
    expect(url.origin + url.pathname).toBe(`${COMMUNITY_REPO}/issues/new`)
    expect(url.searchParams.get('template')).toBe('dock.yml')
    expect(url.searchParams.get('title')).toBe('Dock: Acme Dock 9')
    expect(JSON.parse(url.searchParams.get('entry') ?? '')).toEqual(shareableEntry(dock))
  })

  it('opens through the Wails runtime when there is one, else a new tab', () => {
    const viaRuntime = vi.fn()
    openExternal('https://x', { runtime: { BrowserOpenURL: viaRuntime }, open: vi.fn() } as unknown as Window)
    expect(viaRuntime).toHaveBeenCalledWith('https://x')
    const open = vi.fn()
    openExternal('https://x', { open } as unknown as Window)
    expect(open).toHaveBeenCalledWith('https://x', '_blank', 'noopener')
  })
})
