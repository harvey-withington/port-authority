import { describe, expect, it } from 'vitest'
import { LINK_ORDER, linkBitrate, linkHealth, linkRank } from './link'

describe('link ordering', () => {
  it('ranks faster links higher', () => {
    expect(linkRank('usb4_40')).toBeGreaterThan(linkRank('ss10'))
    expect(linkRank('ss10')).toBeGreaterThan(linkRank('high'))
    expect(LINK_ORDER[0]).toBe('unknown')
  })

  it('knows the nominal bitrate', () => {
    expect(linkBitrate('high')).toBe(480_000_000)
    expect(linkBitrate('ss10')).toBe(10_000_000_000)
    expect(linkBitrate('none')).toBe(0)
  })
})

describe('linkHealth', () => {
  it('is good when negotiated meets min(claimed, port max)', () => {
    expect(linkHealth({ negotiated_link: 'ss10', max_link: 'ss10' }, { claimed_speed: 'usb4_40' })).toBe('good')
    expect(linkHealth({ negotiated_link: 'ss5', max_link: 'ss10' }, { claimed_speed: 'ss5' })).toBe('good')
  })

  it('is slow when the link is below what both sides could do', () => {
    expect(linkHealth({ negotiated_link: 'high', max_link: 'ss10' }, { claimed_speed: 'ss5' })).toBe('slow')
  })

  it('falls back to the port maximum when the claim is unknown', () => {
    expect(linkHealth({ negotiated_link: 'high', max_link: 'high' }, { claimed_speed: 'unknown' })).toBe('good')
    expect(linkHealth({ negotiated_link: 'high', max_link: 'ss10' }, { claimed_speed: 'unknown' })).toBe('slow')
  })

  it('is idle when nothing is known', () => {
    expect(linkHealth({ negotiated_link: 'none', max_link: 'ss10' }, { claimed_speed: 'ss5' })).toBe('idle')
    expect(linkHealth({ negotiated_link: 'high', max_link: 'unknown' }, { claimed_speed: 'unknown' })).toBe('idle')
  })
})
