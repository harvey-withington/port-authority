// How the panes are arranged: how much height the diagram gets, and
// whether the panels under it are showing at all.
//
// The diagram is a left-to-right tree, so it wants width; the two lists
// below it are text and read fine side by side at half width each. They
// show and hide together, as one region, because that is what the divider
// between them and the diagram divides.

/** Share of the pane area given to the diagram, as a fraction. */
const DIAGRAM_KEY = 'pa-diagram-share'
const PANELS_KEY = 'pa-panels-open'

export const MIN_DIAGRAM_SHARE = 0.25
export const MAX_DIAGRAM_SHARE = 0.85
const DEFAULT_DIAGRAM_SHARE = 0.62

function clampShare(v: number): number {
  if (!Number.isFinite(v)) return DEFAULT_DIAGRAM_SHARE
  return Math.min(MAX_DIAGRAM_SHARE, Math.max(MIN_DIAGRAM_SHARE, v))
}

function loadShare(): number {
  try {
    const raw = localStorage.getItem(DIAGRAM_KEY)
    return raw === null ? DEFAULT_DIAGRAM_SHARE : clampShare(Number.parseFloat(raw))
  } catch {
    return DEFAULT_DIAGRAM_SHARE
  }
}

function loadPanelsOpen(): boolean {
  try {
    return localStorage.getItem(PANELS_KEY) !== 'false'
  } catch {
    return true
  }
}

function save(key: string, value: string): void {
  try {
    localStorage.setItem(key, value)
  } catch {
    // Storage unavailable: the choice still holds for this session.
  }
}

let share = $state<number>(loadShare())
let panelsOpen = $state<boolean>(loadPanelsOpen())

export const layout = {
  /** Fraction of the pane area the diagram gets, 0..1. */
  get diagramShare(): number {
    return share
  },
  setDiagramShare(next: number): void {
    share = clampShare(next)
    save(DIAGRAM_KEY, String(share))
  },
  /** Whether the panels under the diagram are showing. */
  get panelsOpen(): boolean {
    return panelsOpen
  },
  togglePanels(): void {
    panelsOpen = !panelsOpen
    save(PANELS_KEY, String(panelsOpen))
  },
  /** Shows the panels if they are hidden: something below wants to be read. */
  openPanels(): void {
    if (panelsOpen) return
    panelsOpen = true
    save(PANELS_KEY, 'true')
  },
}
