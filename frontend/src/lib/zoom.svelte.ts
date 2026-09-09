// The diagram's zoom, shared because the control sits in the pane header
// while the thing it scales lives in the pane body.
//
// Two values: the fit-to-width the diagram works out for itself from its
// own size, and the level the user picked. The user's choice wins until
// they ask for fit again, so a diagram that redraws does not yank the zoom
// out from under them.

export const MIN_ZOOM = 0.5
export const MAX_ZOOM = 1.5
export const ZOOM_STEP = 0.15

export function clampZoom(z: number): number {
  return Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, z))
}

let manual = $state<number | null>(null)
let fit = $state(1)

export const zoom = {
  /** What the diagram should be drawn at. */
  get value(): number {
    return manual ?? fit
  },
  /** True while following the diagram's own fit-to-width. */
  get isFit(): boolean {
    return manual === null
  },
  /** Reported by the diagram, which is the only thing that knows its size. */
  setFit(next: number): void {
    fit = next
  },
  by(delta: number): void {
    manual = clampZoom(this.value + delta)
  },
  fitToWidth(): void {
    manual = null
  },
}
