// Shared DOM behaviours as Svelte actions.
import type { Action } from 'svelte/action'

export interface FlashParams {
  /** Changes to a new non-null value to trigger a scroll + flash. */
  token: number | null
  /** CSS class applied for the flash; must define the animation. */
  className?: string
  durationMs?: number
}

/**
 * Scrolls the node into view and applies a temporary class whenever the
 * token changes to a non-null value. Used to point the user at a device
 * an insight talks about.
 */
export const flash: Action<HTMLElement, FlashParams> = (node, params) => {
  let last: number | null = null
  let timer: ReturnType<typeof setTimeout> | null = null

  const run = (p: FlashParams): void => {
    if (p.token === null || p.token === last) return
    last = p.token
    const cls = p.className ?? 'flash'
    node.scrollIntoView({ block: 'center', behavior: 'smooth' })
    node.classList.remove(cls)
    // Force a reflow so re-adding the class restarts the animation.
    void node.offsetWidth
    node.classList.add(cls)
    if (timer) clearTimeout(timer)
    timer = setTimeout(() => node.classList.remove(cls), p.durationMs ?? 1600)
  }

  run(params)
  return {
    update: run,
    destroy: () => {
      if (timer) clearTimeout(timer)
    },
  }
}

/**
 * Calls the handler when a pointer press lands outside the node. Used by
 * popovers such as the capability hint.
 */
export const clickOutside: Action<HTMLElement, () => void> = (node, handler) => {
  let current = handler
  const onPointerDown = (e: PointerEvent): void => {
    if (e.target instanceof Node && !node.contains(e.target)) current()
  }
  document.addEventListener('pointerdown', onPointerDown, true)
  return {
    update: (next) => {
      current = next
    },
    destroy: () => document.removeEventListener('pointerdown', onPointerDown, true),
  }
}
