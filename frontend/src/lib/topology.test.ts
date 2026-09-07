import { describe, expect, it } from 'vitest'
import { ancestorIds, childEntries, deviceName, indexTopology, nameForId } from './topology'
import { device, sampleTopology } from './fixtures.test-helpers'

describe('deviceName', () => {
  it('prefers knowledge-base vendor and product names', () => {
    expect(deviceName(device({ id: 'a', vendor_name: 'Corsair', product_name: 'EX400U', product: 'EX400U SSD', description: 'USB Mass Storage' }))).toBe('Corsair EX400U')
  })

  it('falls back to descriptors, then OS names', () => {
    expect(deviceName(device({ id: 'a', manufacturer: 'CalDigit, Inc.', product: 'TS4 USB2.0 Hub' }))).toBe('CalDigit, Inc. TS4 USB2.0 Hub')
    expect(deviceName(device({ id: 'a', friendly_name: 'Stream Deck Neo', description: 'USB Input Device' }))).toBe('Stream Deck Neo')
    expect(deviceName(device({ id: 'a', description: 'USB Root Hub (USB 3.0)' }))).toBe('USB Root Hub (USB 3.0)')
  })

  it('does not repeat the vendor when the product already starts with it', () => {
    expect(deviceName(device({ id: 'a', vendor_name: 'Elgato', product: 'Elgato Prompter' }))).toBe('Elgato Prompter')
  })

  it('names vendor-only and anonymous devices', () => {
    expect(deviceName(device({ id: 'a', vendor_name: 'Chicony' }))).toBe('Chicony device')
    expect(deviceName(device({ id: 'a', vendor_id: 0x1b1c, product_id: 0x1a20 }))).toBe('Unknown device 1b1c:1a20')
  })
})

describe('indexTopology', () => {
  const t = sampleTopology()
  const index = indexTopology(t)

  it('indexes every device by normalised id with its port and parent', () => {
    expect(index.size).toBe(4)
    const ssd = index.get('USB\\VID_1B1C&PID_1A20\\SSD')
    expect(ssd?.port?.number).toBe(3)
    expect(ssd?.parentId).toBe('USB\\VID_2188&PID_5500\\DOCK')
    expect(ssd?.depth).toBe(2)
    expect(index.get('USB\\ROOT_HUB30\\ROOT')?.port).toBeNull()
  })

  it('looks up case-insensitively', () => {
    expect(nameForId(index, 'usb\\vid_1b1c&pid_1a20\\ssd')).toBe('Corsair EX400U')
    expect(nameForId(index, 'nope')).toBeNull()
  })

  it('walks ancestors nearest first', () => {
    expect(ancestorIds(index, 'usb\\vid_1b1c&pid_1a20\\ssd')).toEqual(['USB\\VID_2188&PID_5500\\DOCK', 'USB\\ROOT_HUB30\\ROOT'])
    expect(ancestorIds(index, 'USB\\ROOT_HUB30\\ROOT')).toEqual([])
  })

  it('handles a missing topology', () => {
    expect(indexTopology(null).size).toBe(0)
  })
})

describe('childEntries', () => {
  it('lists only occupied ports in order', () => {
    const t = sampleTopology()
    const root = t.controllers[0].root_hub
    expect(root).not.toBeNull()
    const entries = childEntries(root as NonNullable<typeof root>)
    expect(entries.map((e) => e.port.number)).toEqual([1, 2])
    expect(childEntries(device({ id: 'leaf' }))).toEqual([])
  })
})
