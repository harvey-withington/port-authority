<script lang="ts">
  // One of the panels under the diagram. Its body scrolls on its own, so a
  // long timeline never pushes the diagram off the screen or makes the app
  // shell scroll. Controls, when given, sit at the right of the title.
  //
  // There is deliberately no per-panel collapse: these sit side by side, so
  // folding one reclaims no height, the row being as tall as the other.
  // Hiding them is a single control on the divider above.
  import type { Snippet } from 'svelte'

  interface Props {
    id: string
    title: string
    /** Right-aligned in the header, next to the title. */
    controls?: Snippet
    children: Snippet
  }

  let { id, title, controls, children }: Props = $props()
</script>

<section class="pane" aria-labelledby="pane-{id}">
  <div class="head">
    <h2 id="pane-{id}">{title}</h2>
    {#if controls}
      <div class="controls">{@render controls()}</div>
    {/if}
  </div>
  <div class="body">
    {@render children()}
  </div>
</section>

<style>
  .pane {
    display: flex;
    flex-direction: column;
    min-height: 0;
    background: var(--bg-surface);
    border: 1px solid var(--border-muted);
    border-radius: var(--radius-lg);
    padding: var(--space-3);
    min-width: 0;
    overflow: hidden;
  }
  .head {
    flex: 0 0 auto;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
    margin: 0 var(--space-2) var(--space-2);
  }
  h2 {
    font-size: var(--font-size-xs);
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--text-muted);
    margin: 0;
  }
  .controls {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    min-width: 0;
  }
  /* The one scrolling region: the app shell itself never scrolls. */
  .body {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    overflow-x: hidden;
    padding-right: var(--space-1);
  }
</style>
