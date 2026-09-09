// Which rendering of "what is connected" the user last chose. The
// diagram is the hero view; the tree stays one click away.

export type ConnectedView = 'graph' | 'tree'

/**
 * How much of the wiring the diagram spells out.
 *
 * 'physical' draws the boxes on the desk: the computer as one machine, a
 * dock as one dock, one cable between them. 'logical' draws what the OS
 * reports: every controller, every hub in a dock's internal chain, every
 * device. Independent of ConnectedView so the choice survives a trip to
 * the tree and back.
 */
export type DetailLevel = 'physical' | 'logical'

const STORAGE_KEY = 'pa-connected-view'
const DETAIL_KEY = 'pa-detail-level'
const DEFAULT_VIEW: ConnectedView = 'graph'
const DEFAULT_DETAIL: DetailLevel = 'physical'

function isView(v: unknown): v is ConnectedView {
  return v === 'graph' || v === 'tree'
}

function isDetail(v: unknown): v is DetailLevel {
  return v === 'physical' || v === 'logical'
}

function load<T>(key: string, guard: (v: unknown) => v is T, fallback: T): T {
  try {
    const raw = localStorage.getItem(key)
    return guard(raw) ? raw : fallback
  } catch {
    return fallback
  }
}

function save(key: string, value: string): void {
  try {
    localStorage.setItem(key, value)
  } catch {
    // Storage unavailable: the choice still holds for this session.
  }
}

let current = $state<ConnectedView>(load(STORAGE_KEY, isView, DEFAULT_VIEW))
let detail = $state<DetailLevel>(load(DETAIL_KEY, isDetail, DEFAULT_DETAIL))

export const view = {
  get current(): ConnectedView {
    return current
  },
  set(next: ConnectedView): void {
    current = next
    save(STORAGE_KEY, next)
  },
  get detail(): DetailLevel {
    return detail
  },
  setDetail(next: DetailLevel): void {
    detail = next
    save(DETAIL_KEY, next)
  },
}
