// Which rendering of "what is connected" the user last chose. The
// diagram is the hero view; the tree stays one click away.

export type ConnectedView = 'graph' | 'tree'

const STORAGE_KEY = 'pa-connected-view'
const DEFAULT_VIEW: ConnectedView = 'graph'

function isView(v: unknown): v is ConnectedView {
  return v === 'graph' || v === 'tree'
}

function load(): ConnectedView {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    return isView(raw) ? raw : DEFAULT_VIEW
  } catch {
    return DEFAULT_VIEW
  }
}

let current = $state<ConnectedView>(load())

export const view = {
  get current(): ConnectedView {
    return current
  },
  set(next: ConnectedView): void {
    current = next
    try {
      localStorage.setItem(STORAGE_KEY, next)
    } catch {
      // Storage unavailable: the choice still holds for this session.
    }
  },
}
