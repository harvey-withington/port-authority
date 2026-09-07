<script lang="ts">
  // The device's physical edge: an inset panel down the right side of a hub
  // node, with one socket per slot. Each glyph's centre sits at the exact y
  // `lib/graph.ts` gave the slot, so the edge leaving the node starts on the
  // socket the device is plugged into.
  import { SOCKET_STRIP_WIDTH, type SocketSlot } from '../lib/graph'
  import PortSocket from './PortSocket.svelte'

  interface Props {
    sockets: SocketSlot[]
  }

  let { sockets }: Props = $props()

  /** Glyph width in graph pixels: the 40x16 viewBox drawn 1:1. */
  const GLYPH_WIDTH = 40
</script>

<div class="strip" style:width={`${SOCKET_STRIP_WIDTH}px`}>
  {#each sockets as slot (slot.port.number)}
    <span class="slot" style:top={`${slot.y}px`}>
      <PortSocket port={slot.port} occupied={slot.occupied} size={GLYPH_WIDTH} />
    </span>
  {/each}
</div>

<style>
  .strip {
    position: absolute;
    /* -1px top and bottom cancel the node's borders, so slot.y is measured
       from the node's outer edge exactly as the layout computed it. */
    top: -1px;
    bottom: -1px;
    right: 0;
    background: var(--bg-subtle);
    border-left: 1px solid var(--border-muted);
    color: var(--text-secondary);
    pointer-events: none;
  }
  .slot {
    position: absolute;
    left: 50%;
    display: block;
    transform: translate(-50%, -50%);
    pointer-events: auto;
  }
</style>
