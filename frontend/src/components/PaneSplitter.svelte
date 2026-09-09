<script lang="ts">
  // The divider between the diagram and the panels under it: a grab bar to
  // set the split, with the control that shows and hides those panels
  // sitting on it. The button is a sibling of the separator rather than a
  // child, so a press on it can never start a drag.
  //
  // Pointer events rather than mouse events, so a pen or touch works, with
  // capture so a fast drag that leaves the bar keeps tracking. Arrow keys
  // do the same job for anyone not using a pointer, which is why the bar is
  // a separator with a value rather than a bare div.
  import { ChevronDown, ChevronUp } from 'lucide-svelte'
  import { t } from '../lib/i18n.svelte'

  interface Props {
    /** Current share, 0..1. */
    value: number
    min: number
    max: number
    onChange: (next: number) => void
    /** Height of the area the share is measured against, in pixels. */
    track: number
    /** Whether the panels below are showing. */
    open: boolean
    onToggle: () => void
  }

  let { value, min, max, onChange, track, open, onToggle }: Props = $props()

  /** One arrow press moves this fraction of the track. */
  const STEP = 0.02

  let dragging = $state(false)
  let startY = 0
  let startValue = 0

  const clamp = (v: number): number => Math.min(max, Math.max(min, v))
  const percent = $derived(Math.round(value * 100))
  const label = $derived(open ? t('layout.hidePanels') : t('layout.showPanels'))

  function onPointerDown(e: PointerEvent): void {
    if (e.button !== 0 && e.pointerType === 'mouse') return
    dragging = true
    startY = e.clientY
    startValue = value
    ;(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId)
    e.preventDefault()
  }

  function onPointerMove(e: PointerEvent): void {
    if (!dragging || track <= 0) return
    onChange(clamp(startValue + (e.clientY - startY) / track))
  }

  function onPointerUp(e: PointerEvent): void {
    if (!dragging) return
    dragging = false
    ;(e.currentTarget as HTMLElement).releasePointerCapture(e.pointerId)
  }

  function onKeyDown(e: KeyboardEvent): void {
    const by = e.key === 'ArrowUp' ? -STEP : e.key === 'ArrowDown' ? STEP : 0
    if (by === 0) return
    e.preventDefault()
    onChange(clamp(value + by))
  }
</script>

<div class="row" class:dragging>
  {#if open}
    <!--
      A focusable separator carrying a value is the ARIA window-splitter
      pattern, which is what makes the arrow keys a real alternative to
      dragging. The linter only knows the decorative, non-focusable kind.
    -->
    <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <div
      class="bar"
      role="separator"
      aria-orientation="horizontal"
      aria-label={t('layout.resize')}
      aria-valuenow={percent}
      aria-valuemin={Math.round(min * 100)}
      aria-valuemax={Math.round(max * 100)}
      tabindex="0"
      onpointerdown={onPointerDown}
      onpointermove={onPointerMove}
      onpointerup={onPointerUp}
      onpointercancel={onPointerUp}
      onkeydown={onKeyDown}
    >
      <span class="grip" aria-hidden="true"></span>
    </div>
  {/if}
  <button class="fold" aria-expanded={open} aria-controls="pane-panels" aria-label={label} title={label} onclick={onToggle}>
    {#if open}<ChevronDown size={13} aria-hidden="true" />{:else}<ChevronUp size={13} aria-hidden="true" />{/if}
  </button>
</div>

<style>
  .row {
    position: relative;
    flex: 0 0 auto;
    height: 16px;
  }
  .bar {
    position: absolute;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: row-resize;
    border-radius: var(--radius-sm);
    /* Touch scrolling must not steal the drag. */
    touch-action: none;
  }
  .grip {
    display: block;
    width: 100%;
    height: 3px;
    border-radius: 999px;
    background: var(--border);
    /* Leaves the middle clear for the button sitting over it. */
    mask-image: linear-gradient(to right, #000 calc(50% - 22px), transparent calc(50% - 22px), transparent calc(50% + 22px), #000 calc(50% + 22px));
    transition: background var(--duration-fast);
  }
  .bar:hover .grip,
  .bar:focus-visible .grip,
  .row.dragging .grip {
    background: var(--text-muted);
  }
  .bar:focus-visible {
    outline: 2px solid var(--accent);
    outline-offset: 1px;
  }
  .fold {
    position: absolute;
    left: 50%;
    top: 50%;
    transform: translate(-50%, -50%);
    z-index: 1;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 34px;
    height: 16px;
    padding: 0;
    border: 1px solid var(--border);
    border-radius: 999px;
    background: var(--bg-elevated);
    color: var(--text-secondary);
  }
  .fold:hover {
    background: var(--bg-subtle-hover);
    color: var(--text-strong);
    border-color: var(--border-hover);
  }
  @media (prefers-reduced-motion: reduce) {
    .grip {
      transition: none;
    }
  }
</style>
