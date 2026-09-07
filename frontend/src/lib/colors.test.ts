import { describe, expect, it } from 'vitest'
import { CLASS_TOKENS, SPEED_RAMP, classColorVar, classToken, speedColorVar, speedInkVar, speedToken } from './colors'
import type { DeviceClass, LinkSpeed } from './api/types'
import { LINK_ORDER } from './link'

const ALL_CLASSES: DeviceClass[] = [
  'unknown', 'hub', 'storage', 'video', 'audio', 'hid', 'network', 'printer', 'serial',
  'wireless', 'billboard', 'imaging', 'smartcard', 'vendor', 'composite',
]

describe('classToken', () => {
  it('maps every model class to a defined token', () => {
    for (const cls of ALL_CLASSES) expect(CLASS_TOKENS).toContain(classToken(cls))
  })

  it('keeps real device classes on their own token', () => {
    expect(classToken('storage')).toBe('storage')
    expect(classToken('hid')).toBe('hid')
    expect(classToken('wireless')).toBe('wireless')
  })

  it('folds billboard into display and smartcard into vendor', () => {
    expect(classToken('billboard')).toBe('display')
    expect(classToken('smartcard')).toBe('vendor')
  })

  it('lets the knowledge base promote a vendor device to display', () => {
    expect(classToken('vendor', 'display')).toBe('display')
    expect(classToken('vendor', 'storage')).toBe('vendor')
  })

  it('produces a CSS custom property reference', () => {
    expect(classColorVar('audio')).toBe('var(--class-audio)')
  })
})

describe('speedToken', () => {
  it('has a token for every link speed', () => {
    for (const speed of LINK_ORDER) expect(speedToken(speed)).toMatch(/^[a-z0-9-]+$/)
  })

  it('turns underscores into hyphens for CSS', () => {
    expect(speedToken('usb4_40')).toBe('usb4-40')
    expect(speedColorVar('usb4_40')).toBe('var(--speed-usb4-40)')
    expect(speedInkVar('ss10')).toBe('var(--speed-ss10-ink)')
  })

  it('orders the legend ramp slow to fast', () => {
    const ranks = SPEED_RAMP.map((s: LinkSpeed) => LINK_ORDER.indexOf(s))
    for (let i = 1; i < ranks.length; i++) expect(ranks[i]).toBeGreaterThan(ranks[i - 1])
  })
})
