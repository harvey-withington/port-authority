// Pure helpers over the insight list. Identity follows core/api/run.go
// insightKey: rule id plus the sorted device ids.
import type { Insight, Severity } from './api/types'
import { idSetKey, normalizeId } from './ids'

const SEVERITY_RANK: Record<Severity, number> = { critical: 0, warning: 1, info: 2 }

export function severityRank(s: Severity): number {
  return SEVERITY_RANK[s] ?? 3
}

export function insightKey(i: Insight): string {
  return `${i.rule_id}#${idSetKey(i.device_ids)}`
}

/** Severity, then rule id, then title: the order the service uses. */
export function sortInsights(list: readonly Insight[]): Insight[] {
  return [...list].sort((a, b) =>
    severityRank(a.severity) - severityRank(b.severity)
    || a.rule_id.localeCompare(b.rule_id)
    || a.title.localeCompare(b.title))
}

/** Adds the insight unless one with the same key is present. */
export function addInsight(list: readonly Insight[], insight: Insight): Insight[] {
  const key = insightKey(insight)
  if (list.some((i) => insightKey(i) === key)) return [...list]
  return sortInsights([...list, insight])
}

export function removeInsight(list: readonly Insight[], insight: Insight): Insight[] {
  const key = insightKey(insight)
  return list.filter((i) => insightKey(i) !== key)
}

/** Worst severity per normalised device id across all insights. */
export function flaggedDevices(list: readonly Insight[]): ReadonlyMap<string, Severity> {
  const out = new Map<string, Severity>()
  for (const insight of list) {
    for (const id of insight.device_ids ?? []) {
      const key = normalizeId(id)
      const existing = out.get(key)
      if (!existing || severityRank(insight.severity) < severityRank(existing)) out.set(key, insight.severity)
    }
  }
  return out
}

/** The insights that name any of the given devices, in their existing order. */
export function insightsMentioning(list: readonly Insight[], ids: readonly string[]): Insight[] {
  const wanted = new Set(ids.map(normalizeId))
  return list.filter((i) => (i.device_ids ?? []).some((id) => wanted.has(normalizeId(id))))
}
