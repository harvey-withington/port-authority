// Mirrors core/model/model.go, core/insight/insight.go and core/api/*.go.
// Field names are the Go JSON names, verbatim.

export type LinkSpeed =
  | 'unknown' | 'none' | 'low' | 'full' | 'high'
  | 'ss5' | 'ss10' | 'ss20' | 'usb4_20' | 'usb4_40' | 'usb4_80'

export type ControllerKind = 'unknown' | 'ehci' | 'xhci' | 'usb4-router' | 'thunderbolt3'

export type HubKind = 'unknown' | 'root' | 'usb2' | 'usb3'

export type DeviceClass =
  | 'unknown' | 'hub' | 'storage' | 'video' | 'audio' | 'hid' | 'network' | 'printer'
  | 'serial' | 'wireless' | 'billboard' | 'imaging' | 'smartcard' | 'vendor' | 'composite'

export type ConnectionStatus =
  | 'unknown' | 'none' | 'connected' | 'failed_enumeration' | 'general_failure'
  | 'overcurrent' | 'not_enough_power' | 'not_enough_bandwidth' | 'nested_too_deeply'
  | 'legacy_hub' | 'enumerating'

export interface Interface {
  number: number
  class: number
  subclass: number
  protocol: number
  kind: DeviceClass
  endpoints: number
}

export interface Connector {
  type_c: boolean
  user_connectable: boolean
  multiple_companions: boolean
  companion_hub_path?: string
  companion_port?: number
}

export interface AltModeInfo {
  displayport_lanes: number
  dsc: boolean
  confidence: number
  source: string
}

export interface TruthReport {
  claimed_speed: LinkSpeed
  measured_read_bps: number
  measured_write_bps: number
  capacity_verdict: string
  confidence: number
  tested_at: string
}

export interface Port {
  number: number
  status: ConnectionStatus
  negotiated_link: LinkSpeed
  max_link: LinkSpeed
  /** Printed dock/chassis label for the socket, e.g. "USB-C Data". */
  label?: string
  /** Where the socket sits on the chassis, e.g. "front" / "rear". */
  position?: string
  connector?: Connector
  alt_mode?: AltModeInfo
  device?: Device
}

export type EnclosureKind = 'host' | 'dock' | 'hub'

/**
 * The physical box a hub lives in. Windows reports a dock as a chain of
 * hubs across two controllers and the computer as several controllers with
 * a root hub each; hubs sharing an enclosure id are one object on the desk.
 */
export interface Enclosure {
  id: string
  kind: EnclosureKind
  /** Set when the knowledge base names the box, e.g. "CalDigit TS4". */
  name?: string
  dock_id?: string
}

export interface Hub {
  kind: HubKind
  depth: number
  port_count: number
  bus_powered: boolean
  device_path?: string
  ports: Port[]
}

export interface Device {
  id: string
  port_path: string
  vendor_id: number
  product_id: number
  bcd_device: number
  bcd_usb: number
  vendor_name?: string
  product_name?: string
  class: DeviceClass
  usb_class: number
  usb_subclass: number
  usb_protocol: number
  interfaces?: Interface[]
  manufacturer?: string
  product?: string
  serial_number?: string
  description?: string
  friendly_name?: string
  instance_id?: string
  driver_key?: string
  location?: string
  claimed_speed: LinkSpeed
  iso_reserved: number
  power_draw_ma: number
  children?: string[]
  truth_report?: TruthReport
  hub?: Hub
  /** Only hubs carry one; a leaf device belongs to the box its port is on. */
  enclosure?: Enclosure
}

export interface Controller {
  id: string
  name: string
  kind: ControllerKind
  max_bandwidth: number
  device_path?: string
  instance_id?: string
  driver_key?: string
  root_hub: Device | null
}

export type USB4RouterKind = 'host' | 'device'

export interface USB4Router {
  id: string
  instance_id: string
  name: string
  vendor_id: number
  product_id: number
  /** Resolved from the knowledge base: the product, not the bridge silicon. */
  vendor_name?: string
  product_name?: string
  kind: USB4RouterKind
  parent_id?: string
  depth: number
  children?: string[]
}

export interface Topology {
  schema_version: number
  captured_at: string
  platform: string
  controllers: Controller[]
  usb4?: USB4Router[]
  warnings?: string[]
}

export interface ProviderCaps {
  platform: string
  topology: boolean
  hotplug: boolean
  throughput: boolean
  alt_mode: boolean
  power_draw: boolean
  iso_reservation: boolean
  connector_info: boolean
  string_descriptors: boolean
}

export type TopologyEventKind = 'device_added' | 'device_removed' | 'link_changed' | 'resnapshot'

export interface TopologyEvent {
  kind: TopologyEventKind
  at: string
  device_id?: string
  detail?: string
}

export interface ThroughputSample {
  device_id: string
  at: string
  read_bps: number
  write_bps: number
}

export type Severity = 'info' | 'warning' | 'critical'

export interface Insight {
  rule_id: string
  severity: Severity
  title: string
  explanation: string
  suggestion: string
  evidence?: string[]
  confidence: number
  device_ids?: string[]
}

// ---- REST responses (core/api/handler.go, stream.go) ----

export interface HealthResponse {
  ok: boolean
  schema_version: number
}

export interface CapabilitiesResponse {
  schema_version: number
  capabilities: ProviderCaps
}

export interface TopologyResponse {
  schema_version: number
  captured_at: string
  topology: Topology
  insights: Insight[]
}

export interface InsightsResponse {
  schema_version: number
  captured_at: string
  insights: Insight[]
}

export interface DeviceResponse {
  schema_version: number
  captured_at: string
  device: Device
  insights: Insight[]
}

export interface ThroughputResponse {
  schema_version: number
  window_seconds: number
  samples: ThroughputSample[]
}

export interface ErrorResponse {
  error: string
}

// ---- WebSocket stream (core/api/events.go) ----

export interface HelloData {
  schema_version: number
  capabilities: ProviderCaps
  types?: string[]
  captured_at: string
  insights: Insight[]
  error?: string
}

export interface TopologyChangedData {
  events: TopologyEvent[]
  captured_at: string
  fetched_at: string
  controllers: number
  devices: number
}

export type StreamEvent =
  | { type: 'hello'; at: string; data: HelloData }
  | { type: 'topology_changed'; at: string; data: TopologyChangedData }
  | { type: 'insight_added'; at: string; data: Insight }
  | { type: 'insight_resolved'; at: string; data: Insight }
  | { type: 'throughput_sample'; at: string; data: ThroughputSample }

export type StreamEventType = StreamEvent['type']

export const STREAM_EVENT_TYPES: readonly StreamEventType[] = [
  'hello', 'topology_changed', 'insight_added', 'insight_resolved', 'throughput_sample',
]

export function isStreamEvent(value: unknown): value is StreamEvent {
  if (typeof value !== 'object' || value === null) return false
  const v = value as { type?: unknown; at?: unknown; data?: unknown }
  return typeof v.type === 'string'
    && (STREAM_EVENT_TYPES as readonly string[]).includes(v.type)
    && typeof v.at === 'string'
    && typeof v.data === 'object' && v.data !== null
}
