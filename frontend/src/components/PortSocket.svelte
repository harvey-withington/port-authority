<script lang="ts">
  // The physical socket, drawn in the proportions of the real thing: a USB-A
  // rectangle with its tongue in the upper half, the flatter USB-C pill, a
  // dotted circle for something soldered inside, or a plain rectangle when the
  // provider cannot tell. The inner shape carries the port's maximum speed
  // colour and is filled only when something is plugged in, so an occupied
  // socket reads as "lit" and an empty one as an outline. The bolt on the left
  // marks a USB4 / Thunderbolt socket, so shape and colour never work alone.
  //
  // The 40x16 viewBox is in graph pixels at size 40: a USB-A shell is 28x12,
  // the bolt 8 tall. The socket is centred on the viewBox so it centres in the
  // strip, and the bolt sits in the reserved left margin. The tree passes the
  // smaller default.
  import type { Port } from '../lib/api/types'
  import { isUsb4, socketKind } from '../lib/ports'
  import { speedColorVar } from '../lib/colors'
  import { linkDescription } from '../lib/format'
  import { t } from '../lib/i18n.svelte'

  interface Props {
    port: Port
    /** Whether a device is plugged into this socket. */
    occupied: boolean
    /** Glyph width in pixels; the height follows the 40x16 viewBox. */
    size?: number
  }

  let { port, occupied, size = 14 }: Props = $props()

  const VIEW_W = 40
  const VIEW_H = 16

  const kind = $derived(socketKind(port))
  const usb4 = $derived(isUsb4(port))
  const speed = $derived(speedColorVar(port.max_link))
  const height = $derived((size * VIEW_H) / VIEW_W)
  /** Occupied sockets fill their inner shape; empty ones only outline it. */
  const innerFill = $derived(occupied ? speed : 'none')

  /** "USB-A Data (rear)", or whichever half the provider gave us. */
  const printed = $derived(
    port.label && port.position ? t('socket.labelled', { label: port.label, position: port.position })
      : (port.label ?? port.position ?? ''),
  )
  const title = $derived([
    t('socket.title', {
      n: port.number,
      kind: t(`socket.kind.${kind}`),
      max: linkDescription(port.max_link),
      state: occupied ? t('socket.state.occupied') : t('socket.state.empty'),
    }),
    ...(usb4 ? [t('socket.usb4')] : []),
    ...(printed ? [printed] : []),
  ].join('. '))
</script>

<svg
  class="socket kind-{kind}"
  class:empty={!occupied}
  width={size}
  height={height}
  viewBox="0 0 {VIEW_W} {VIEW_H}"
  role="img"
  aria-label={title}
  data-port={port.number}
>
  <title>{title}</title>
  {#if usb4}
    <polyline class="bolt" points="5.4,3 1.6,8.6 4.4,8.6 2.6,13" style:stroke={speedColorVar('usb4_40')} />
  {/if}
  {#if kind === 'usb-a'}
    <rect class="shell" x="6" y="2" width="28" height="12" rx="2" />
    <rect class="inner" x="10" y="3.5" width="20" height="4" style:fill={innerFill} style:stroke={speed} />
  {:else if kind === 'usb-c'}
    <rect class="shell" x="7" y="3" width="26" height="10" rx="5" />
    <rect class="inner" x="11" y="6.5" width="18" height="3" rx="1.5" style:fill={innerFill} style:stroke={speed} />
  {:else if kind === 'internal'}
    <circle class="shell dotted" cx="20" cy="8" r="6" />
    <circle class="inner mark" cx="20" cy="8" r="2" />
  {:else}
    <rect class="shell" x="8" y="3" width="24" height="10" rx="2" />
  {/if}
</svg>

<style>
  .socket {
    display: block;
    flex-shrink: 0;
    overflow: visible;
  }
  .shell,
  .bolt {
    fill: none;
    stroke: currentColor;
    stroke-width: 1.2;
    stroke-linejoin: round;
    stroke-linecap: round;
  }
  .dotted {
    stroke-dasharray: 2 2;
  }
  .inner {
    stroke-width: 1;
  }
  /* The "built in" mark takes the node's ink, not a speed colour. */
  .mark {
    fill: currentColor;
    stroke: currentColor;
  }
  /* An unused socket stays visible but recedes behind the ones in use. */
  .socket.empty {
    opacity: 0.45;
  }
  .socket.empty .mark {
    fill: none;
  }
</style>
