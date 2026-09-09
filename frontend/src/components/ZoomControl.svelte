<script lang="ts">
  // Zoom for the diagram, in the pane header next to the view switches
  // rather than on a row of its own above the canvas.
  import { Maximize2, Minus, Plus } from 'lucide-svelte'
  import { MAX_ZOOM, MIN_ZOOM, ZOOM_STEP, zoom } from '../lib/zoom.svelte'
  import { t } from '../lib/i18n.svelte'

  const label = $derived(t('format.percent', { n: Math.round(zoom.value * 100) }))
</script>

<div class="zoom" role="group" aria-label={t('graph.zoom')}>
  <button class="tool" onclick={() => zoom.by(-ZOOM_STEP)} disabled={zoom.value <= MIN_ZOOM} aria-label={t('graph.zoomOut')} title={t('graph.zoomOut')}>
    <Minus size={13} aria-hidden="true" />
  </button>
  <span class="level mono" aria-live="polite">{label}</span>
  <button class="tool" onclick={() => zoom.by(ZOOM_STEP)} disabled={zoom.value >= MAX_ZOOM} aria-label={t('graph.zoomIn')} title={t('graph.zoomIn')}>
    <Plus size={13} aria-hidden="true" />
  </button>
  <button class="tool" onclick={() => zoom.fitToWidth()} aria-pressed={zoom.isFit} aria-label={t('graph.zoomFit')} title={t('graph.zoomFit')}>
    <Maximize2 size={13} aria-hidden="true" />
  </button>
</div>

<style>
  .zoom {
    display: inline-flex;
    align-items: center;
    gap: 1px;
    padding: 2px;
    border: 1px solid var(--border);
    border-radius: 999px;
    background: var(--bg-surface);
  }
  .tool {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 20px;
    height: 20px;
    padding: 0;
    border: 0;
    border-radius: 999px;
    background: transparent;
    color: var(--text-secondary);
  }
  .tool:hover:not(:disabled),
  .tool[aria-pressed='true'] {
    background: var(--bg-elevated);
    color: var(--text-strong);
  }
  .tool:disabled {
    opacity: 0.4;
    cursor: default;
  }
  .level {
    min-width: 34px;
    text-align: center;
    font-size: var(--font-size-xs);
    color: var(--text-muted);
    font-variant-numeric: tabular-nums;
  }
</style>
