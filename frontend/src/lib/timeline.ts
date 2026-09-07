// "What changed" timeline: a bounded, newest-first list of things worth
// telling the user about. Pure functions; the live store owns the state.
import type { Insight, Severity, TopologyChangedData } from './api/types'
import type { ConnectionState } from './connection'

export type TimelineKind =
  | 'device_added' | 'device_removed' | 'link_changed' | 'resnapshot'
  | 'insight_added' | 'insight_resolved' | 'connection'

export interface TimelineEntry {
  /** Unique, stable id for keyed rendering. */
  id: string
  /** RFC 3339 timestamp of the event. */
  at: string
  kind: TimelineKind
  /** Device name, insight title or connection state, depending on kind. */
  subject: string
  deviceId?: string
  severity?: Severity
  connection?: ConnectionState
  detail?: string
}

export const TIMELINE_MAX = 100

export type IdSource = () => string

/** Monotonic ids so keyed lists never collide, even for same-instant events. */
export function createIdSource(prefix = 'ev'): IdSource {
  let n = 0
  return () => `${prefix}-${++n}`
}

/** Newest first, capped at `max`. Returns a new array. */
export function prependEntries(list: readonly TimelineEntry[], entries: readonly TimelineEntry[], max = TIMELINE_MAX): TimelineEntry[] {
  if (entries.length === 0) return [...list]
  const newest = [...entries].reverse()
  return [...newest, ...list].slice(0, max)
}

/**
 * One entry per hotplug notification in a topology_changed burst. Names
 * are resolved by the caller (the removed device is only known to the
 * previous snapshot). A burst with no events becomes a single resnapshot.
 */
export function entriesFromTopologyChanged(
  at: string,
  data: TopologyChangedData,
  resolveName: (deviceId: string) => string | null,
  nextId: IdSource,
): TimelineEntry[] {
  const events = data.events ?? []
  if (events.length === 0) {
    return [{ id: nextId(), at, kind: 'resnapshot', subject: '', detail: String(data.devices) }]
  }
  return events.map((ev) => {
    const deviceId = ev.device_id
    const name = deviceId ? resolveName(deviceId) : null
    return {
      id: nextId(),
      at: ev.at || at,
      kind: ev.kind,
      subject: name ?? deviceId ?? '',
      deviceId,
      detail: ev.detail,
    }
  })
}

export function entryFromInsight(kind: 'insight_added' | 'insight_resolved', at: string, insight: Insight, nextId: IdSource): TimelineEntry {
  return {
    id: nextId(),
    at,
    kind,
    subject: insight.title,
    severity: insight.severity,
    deviceId: insight.device_ids?.[0],
  }
}

export function entryFromConnection(at: string, state: ConnectionState, nextId: IdSource, detail?: string): TimelineEntry {
  return { id: nextId(), at, kind: 'connection', subject: state, connection: state, detail }
}
