// Shared read-only context passed down the topology tree so the recursive
// DeviceNode does not need a prop per concern.
import type { DockView, Severity } from './api/types'
import type { ThroughputMap } from './throughput'

export interface TreeContext {
  /** Normalised device id -> worst insight severity naming it. */
  flagged: ReadonlyMap<string, Severity>
  throughput: ThroughputMap
  /** Whether the provider streams throughput at all (meters show "idle" when true, nothing when false). */
  showMeter: boolean
  /** Dock id -> knowledge base entry, so a dock's cable can be judged against what the dock can do. */
  docks?: ReadonlyMap<string, DockView>
}
