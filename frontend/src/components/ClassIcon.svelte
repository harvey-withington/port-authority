<script lang="ts">
  import {
    AudioLines, Box, Bluetooth, Cable, HardDrive, Keyboard, Layers, Monitor, Network,
    Package, Printer, ScanLine, Usb, Video,
  } from 'lucide-svelte'
  import type { DeviceClass } from '../lib/api/types'
  import { classToken, type ClassToken } from '../lib/colors'
  import { t } from '../lib/i18n.svelte'

  interface Props {
    cls: DeviceClass
    size?: number
  }

  let { cls, size = 15 }: Props = $props()

  const ICONS: Record<ClassToken, typeof HardDrive> = {
    storage: HardDrive,
    video: Video,
    audio: AudioLines,
    hid: Keyboard,
    network: Network,
    hub: Usb,
    display: Monitor,
    wireless: Bluetooth,
    printer: Printer,
    serial: Cable,
    imaging: ScanLine,
    composite: Layers,
    vendor: Package,
    unknown: Box,
  }

  const token = $derived(classToken(cls))
  const Icon = $derived(ICONS[token])
  const label = $derived(t(`class.${cls}`))
</script>

<span class="class-icon" style:color={`var(--class-${token})`} title={label} role="img" aria-label={label}>
  <Icon {size} strokeWidth={2} aria-hidden="true" />
</span>

<style>
  .class-icon {
    display: inline-flex;
    align-items: center;
    flex-shrink: 0;
    line-height: 0;
  }
</style>
