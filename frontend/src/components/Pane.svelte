<script lang="ts">
  // One of the panels under the diagram. Its body scrolls on its own, so a
  // long timeline never pushes the diagram off the screen or makes the app
  // shell scroll.
  //
  // There is deliberately no per-panel collapse: these sit side by side, so
  // folding one reclaims no height, the row being as tall as the other.
  // Hiding them is a single control on the divider above.
  import type { Snippet } from 'svelte'

  interface Props {
    id: string
    title: string
    children: Snippet
  }

  let { id, title, children }: Props = $props()
</script>

<section class="pane" aria-labelledby="pane-{id}">
  <h2 id="pane-{id}">{title}</h2>
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
  h2 {
    flex: 0 0 auto;
    font-size: var(--font-size-xs);
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--text-muted);
    margin: 0 var(--space-2) var(--space-2);
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
