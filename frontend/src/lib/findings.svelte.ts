// Which findings the user has folded down to their headline.
//
// A finding is worth reading in full, so they start open and only the
// closed ones are stored, the same way collapsed hubs are. Keyed by
// insightKey, so a finding that goes away and comes back is still the one
// the user chose to fold.

const STORAGE_KEY = 'pa-folded-findings'

function load(): Record<string, true> {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return {}
    const parsed: unknown = JSON.parse(raw)
    if (!Array.isArray(parsed)) return {}
    const out: Record<string, true> = {}
    for (const key of parsed as unknown[]) if (typeof key === 'string') out[key] = true
    return out
  } catch {
    return {}
  }
}

function persist(folded: Record<string, true>): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(Object.keys(folded)))
  } catch {
    // Storage unavailable: the choice still holds for this session.
  }
}

let folded = $state<Record<string, true>>(load())

export const findings = {
  isOpen(key: string): boolean {
    return !folded[key]
  },
  toggle(key: string): void {
    const next = { ...folded }
    if (next[key]) delete next[key]
    else next[key] = true
    folded = next
    persist(next)
  },
}
