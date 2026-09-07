import { describe, expect, it } from 'vitest'
import { idSetKey, normalizeId, sameId } from './ids'

describe('id normalisation', () => {
  const lower = 'USB\\VID_1b1c&PID_1a20\\msft30scrubbed-06'
  const upper = 'USB\\VID_1B1C&PID_1A20\\MSFT30SCRUBBED-06'

  it('upper-cases and trims', () => {
    expect(normalizeId(`  ${lower} `)).toBe(upper)
  })

  it('compares case-insensitively', () => {
    expect(sameId(lower, upper)).toBe(true)
    expect(sameId(lower, 'USB\\VID_0000&PID_0000\\1')).toBe(false)
    expect(sameId(null, upper)).toBe(false)
    expect(sameId('', '')).toBe(false)
  })

  it('builds an order-independent set key', () => {
    expect(idSetKey(['b', 'A'])).toBe(idSetKey(['a', 'B']))
    expect(idSetKey(undefined)).toBe('')
  })
})
