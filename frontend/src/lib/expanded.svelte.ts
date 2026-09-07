// Which hubs the user collapsed, remembered per device id. Hubs default
// to expanded, so only the collapsed set is stored.
import { normalizeId } from './ids'

const STORAGE_KEY = 'pa-collapsed-hubs'

function load(): Record<string, true> {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return {}
    const parsed: unknown = JSON.parse(raw)
    if (!Array.isArray(parsed)) return {}
    const out: Record<string, true> = {}
    for (const id of parsed) if (typeof id === 'string') out[normalizeId(id)] = true
    return out
  } catch {
    return {}
  }
}

function persist(collapsed: Record<string, true>): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(Object.keys(collapsed)))
  } catch {
    // Storage unavailable: the choice still holds for this session.
  }
}

let collapsed = $state<Record<string, true>>(load())

export const expanded = {
  isExpanded(id: string): boolean {
    return !collapsed[normalizeId(id)]
  },
  toggle(id: string): void {
    const key = normalizeId(id)
    const next = { ...collapsed }
    if (next[key]) delete next[key]
    else next[key] = true
    collapsed = next
    persist(next)
  },
  /** Ensures every id is expanded (used to reveal a focused device). */
  expand(ids: readonly string[]): void {
    const next = { ...collapsed }
    let changed = false
    for (const id of ids) {
      const key = normalizeId(id)
      if (next[key]) {
        delete next[key]
        changed = true
      }
    }
    if (!changed) return
    collapsed = next
    persist(next)
  },
}
