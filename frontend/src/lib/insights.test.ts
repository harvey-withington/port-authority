import { describe, expect, it } from 'vitest'
import { addInsight, flaggedDevices, insightKey, insightsMentioning, removeInsight, sortInsights } from './insights'
import { insight } from './fixtures.test-helpers'

describe('insight identity', () => {
  it('keys by rule and sorted, normalised device ids', () => {
    expect(insightKey(insight({ device_ids: ['b', 'A'] }))).toBe(insightKey(insight({ device_ids: ['a', 'B'] })))
    expect(insightKey(insight({ rule_id: 'x' }))).not.toBe(insightKey(insight({ rule_id: 'y' })))
  })

  it('adds only when absent and removes by key', () => {
    const list = addInsight([], insight())
    expect(addInsight(list, insight({ device_ids: ['usb\\vid_1b1c&pid_1a20\\ssd'] }))).toHaveLength(1)
    expect(removeInsight(list, insight({ title: 'renamed but same key' }))).toHaveLength(0)
  })

  it('sorts by severity, rule, title', () => {
    const sorted = sortInsights([
      insight({ severity: 'info', rule_id: 'b', title: 'z' }),
      insight({ severity: 'critical', rule_id: 'z', title: 'a' }),
      insight({ severity: 'warning', rule_id: 'a', title: 'b' }),
      insight({ severity: 'warning', rule_id: 'a', title: 'a' }),
    ])
    expect(sorted.map((i) => `${i.severity}/${i.rule_id}/${i.title}`)).toEqual(['critical/z/a', 'warning/a/a', 'warning/a/b', 'info/b/z'])
  })

  it('flags devices with their worst severity', () => {
    const flagged = flaggedDevices([
      insight({ severity: 'info', device_ids: ['usb\\a', 'usb\\b'] }),
      insight({ severity: 'critical', rule_id: 'other', device_ids: ['USB\\B'] }),
    ])
    expect(flagged.get('USB\\A')).toBe('info')
    expect(flagged.get('USB\\B')).toBe('critical')
    expect(flagged.size).toBe(2)
  })

  it('finds the insights naming any of several devices, whatever the case', () => {
    const a = insight({ rule_id: 'a', device_ids: ['usb\\a'] })
    const b = insight({ rule_id: 'b', device_ids: ['USB\\B', 'USB\\C'] })
    const none = insight({ rule_id: 'n', device_ids: [] })
    expect(insightsMentioning([a, b, none], ['USB\\A'])).toEqual([a])
    expect(insightsMentioning([a, b, none], ['usb\\c', 'USB\\A'])).toEqual([a, b])
    expect(insightsMentioning([a, b, none], ['USB\\Z'])).toEqual([])
  })
})
