// Shared DOM behaviours as Svelte actions.
import type { Action } from 'svelte/action'

export interface FlashParams {
  /** Changes to a new non-null value to trigger a scroll + flash. */
  token: number | null
  /** CSS class applied for the flash; must define the animation. */
  className?: string
  durationMs?: number
  /** Whether to scroll the node into view first; true unless told otherwise. */
  scroll?: boolean
}

/**
 * Scrolls the node into view and applies a temporary class whenever the
 * token changes to a non-null value. Used to point the user at a device
 * an insight talks about, and at the findings a flag stands for.
 */
export const flash: Action<HTMLElement, FlashParams> = (node, params) => {
  let last: number | null = null
  let timer: ReturnType<typeof setTimeout> | null = null

  const run = (p: FlashParams): void => {
    if (p.token === null || p.token === last) return
    last = p.token
    const cls = p.className ?? 'flash'
    // jsdom has no scrollIntoView; the flash itself still runs there.
    if (p.scroll !== false && typeof node.scrollIntoView === 'function') node.scrollIntoView({ block: 'center', behavior: 'smooth' })
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
 * Reports the node's content width now and whenever it changes. Falls
 * back to a single measurement where ResizeObserver does not exist
 * (jsdom), so components never touch the constructor themselves.
 */
export const observeWidth: Action<HTMLElement, (width: number) => void> = (node, handler) => {
  let current = handler
  const report = (): void => current(node.clientWidth)
  report()
  const observer = typeof ResizeObserver === 'function' ? new ResizeObserver(report) : null
  observer?.observe(node)
  return {
    update: (next) => {
      current = next
      report()
    },
    destroy: () => observer?.disconnect(),
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

export interface DraggableWindowParams {
  /** localStorage key the position is remembered under. */
  key: string
  /** CSS selector for the grab area inside the node; the node itself if omitted. */
  handle?: string
  /** Distance from the viewport edge for the first appearance, in pixels. */
  margin?: number
  /**
   * Where the top edge sits the first time, in pixels. Defaults to the
   * margin, which on a page with a header would put the window over it.
   */
  initialTop?: number
}

interface WindowPosition {
  x: number
  y: number
}

/** Keeps a window fully on screen, whatever the viewport has done since. */
function clampToViewport(node: HTMLElement, pos: WindowPosition, margin: number): WindowPosition {
  const maxX = Math.max(margin, window.innerWidth - node.offsetWidth - margin)
  const maxY = Math.max(margin, window.innerHeight - node.offsetHeight - margin)
  return {
    x: Math.min(maxX, Math.max(margin, pos.x)),
    y: Math.min(maxY, Math.max(margin, pos.y)),
  }
}

/**
 * Makes a panel a floating window the user can move by its handle.
 *
 * The position is remembered, and re-clamped whenever the window changes
 * size, so a window parked against an edge cannot end up off screen after
 * a resize. Pointer events with capture, so a fast drag that outruns the
 * handle keeps tracking; arrow keys move it too, since a window that only
 * a mouse can reach is a window some people cannot move at all.
 */
export const draggableWindow: Action<HTMLElement, DraggableWindowParams> = (node, params) => {
  const margin = params.margin ?? 12
  let pos: WindowPosition = { x: 0, y: 0 }
  let dragging = false
  let startX = 0
  let startY = 0
  let origin: WindowPosition = { x: 0, y: 0 }

  const apply = (): void => {
    node.style.left = `${pos.x}px`
    node.style.top = `${pos.y}px`
  }

  const save = (): void => {
    try {
      localStorage.setItem(params.key, JSON.stringify(pos))
    } catch {
      // Storage unavailable: the position still holds for this session.
    }
  }

  const load = (): WindowPosition | null => {
    try {
      const raw = localStorage.getItem(params.key)
      if (!raw) return null
      const parsed: unknown = JSON.parse(raw)
      if (typeof parsed !== 'object' || parsed === null) return null
      const { x, y } = parsed as Partial<WindowPosition>
      return typeof x === 'number' && typeof y === 'number' ? { x, y } : null
    } catch {
      return null
    }
  }

  // First appearance: where it used to be pinned, below the header on the
  // right, so it does not cover the control that opened it.
  const stored = load()
  let placed = stored !== null

  const place = (): void => {
    if (!placed) {
      // Right-aligned, which needs a width. Stylesheets can still be
      // loading when an action first runs, and measuring then gives the
      // unstyled box and so the wrong corner.
      pos = { x: window.innerWidth - node.offsetWidth - margin, y: params.initialTop ?? margin }
    }
    pos = clampToViewport(node, pos, margin)
    apply()
  }

  pos = stored ?? { x: margin, y: params.initialTop ?? margin }
  place()

  // Re-place once the node's real size is known, and whenever it changes.
  const sizer = typeof ResizeObserver === 'function'
    ? new ResizeObserver(() => {
      place()
      // Only the first measurement decides the default corner; after that
      // the position is the user's.
      if (node.offsetWidth > 0) placed = true
    })
    : null
  sizer?.observe(node)

  const handle: HTMLElement = (params.handle ? node.querySelector<HTMLElement>(params.handle) : null) ?? node
  handle.style.cursor = 'move'
  handle.style.touchAction = 'none'

  const onPointerDown = (e: PointerEvent): void => {
    // Never start a drag from a control inside the handle, such as Close.
    if (e.target instanceof Element && e.target.closest('button, a, input, select, textarea')) return
    if (e.button !== 0 && e.pointerType === 'mouse') return
    dragging = true
    startX = e.clientX
    startY = e.clientY
    origin = { ...pos }
    handle.setPointerCapture(e.pointerId)
    e.preventDefault()
  }

  const onPointerMove = (e: PointerEvent): void => {
    if (!dragging) return
    pos = clampToViewport(node, { x: origin.x + (e.clientX - startX), y: origin.y + (e.clientY - startY) }, margin)
    apply()
  }

  const onPointerUp = (e: PointerEvent): void => {
    if (!dragging) return
    dragging = false
    placed = true
    handle.releasePointerCapture(e.pointerId)
    save()
  }

  const onKeyDown = (e: KeyboardEvent): void => {
    const step = e.shiftKey ? 24 : 8
    const dx = e.key === 'ArrowLeft' ? -step : e.key === 'ArrowRight' ? step : 0
    const dy = e.key === 'ArrowUp' ? -step : e.key === 'ArrowDown' ? step : 0
    if (dx === 0 && dy === 0) return
    e.preventDefault()
    placed = true
    pos = clampToViewport(node, { x: pos.x + dx, y: pos.y + dy }, margin)
    apply()
    save()
  }

  const onResize = (): void => {
    pos = clampToViewport(node, pos, margin)
    apply()
  }

  handle.addEventListener('pointerdown', onPointerDown)
  handle.addEventListener('pointermove', onPointerMove)
  handle.addEventListener('pointerup', onPointerUp)
  handle.addEventListener('pointercancel', onPointerUp)
  handle.addEventListener('keydown', onKeyDown)
  window.addEventListener('resize', onResize)

  return {
    destroy: () => {
      sizer?.disconnect()
      handle.removeEventListener('pointerdown', onPointerDown)
      handle.removeEventListener('pointermove', onPointerMove)
      handle.removeEventListener('pointerup', onPointerUp)
      handle.removeEventListener('pointercancel', onPointerUp)
      handle.removeEventListener('keydown', onKeyDown)
      window.removeEventListener('resize', onResize)
    },
  }
}
