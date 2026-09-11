<script lang="ts">
  // One glyph per severity, so a finding and the flag on the device it
  // names wear the same sign. Colour is never the only signal.
  import { Info, OctagonAlert, TriangleAlert } from 'lucide-svelte'
  import type { Severity } from '../lib/api/types'

  interface Props {
    severity: Severity
    size?: number
  }

  let { severity, size = 16 }: Props = $props()

  const Icon = $derived(severity === 'critical' ? OctagonAlert : severity === 'warning' ? TriangleAlert : Info)
</script>

<Icon {size} aria-hidden="true" />
