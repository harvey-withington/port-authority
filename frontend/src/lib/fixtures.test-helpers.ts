// Builders for test topologies. Not a test file itself.
import type { Connector, Controller, Device, Hub, Insight, Port, Topology } from './api/types'

export function device(overrides: Partial<Device> & { id: string }): Device {
  return {
    port_path: '1',
    vendor_id: 0,
    product_id: 0,
    bcd_device: 0,
    bcd_usb: 0,
    class: 'unknown',
    usb_class: 0,
    usb_subclass: 0,
    usb_protocol: 0,
    claimed_speed: 'unknown',
    iso_reserved: 0,
    power_draw_ma: 0,
    ...overrides,
  }
}

export function hub(ports: Port[], overrides: Partial<Hub> = {}): Hub {
  return { kind: 'usb3', depth: 1, port_count: ports.length, bus_powered: false, ports, ...overrides }
}

export function port(number: number, dev: Device | undefined, overrides: Partial<Port> = {}): Port {
  return { number, status: dev ? 'connected' : 'none', negotiated_link: dev ? 'ss10' : 'none', max_link: 'ss10', device: dev, ...overrides }
}

export function controller(root: Device, overrides: Partial<Controller> = {}): Controller {
  return { id: 'PCI\\CTRL', name: 'Test xHCI', kind: 'xhci', max_bandwidth: 10_000_000_000, root_hub: root, ...overrides }
}

export function topology(controllers: Controller[], overrides: Partial<Topology> = {}): Topology {
  return { schema_version: 1, captured_at: '2026-09-07T00:00:00Z', platform: 'test', controllers, ...overrides }
}

export function insight(overrides: Partial<Insight> = {}): Insight {
  return {
    rule_id: 'faster-port-available',
    severity: 'warning',
    title: 'SSD could be faster',
    explanation: 'It is on a slow port.',
    suggestion: 'Move it.',
    confidence: 0.9,
    device_ids: ['USB\\VID_1B1C&PID_1A20\\SSD'],
    ...overrides,
  }
}

/** A user-facing socket descriptor; `internal` ports are not user connectable. */
export function connector(type_c: boolean, overrides: Partial<Connector> = {}): Connector {
  return { type_c, user_connectable: true, multiple_companions: false, ...overrides }
}

/**
 * Controller -> root hub -> [hub(port 1) -> ssd(port 3)], mouse(port 2),
 * plus an empty USB-A socket on root port 3 so an unused socket is drawn.
 */
export function sampleTopology(): Topology {
  const ssd = device({
    id: 'USB\\VID_1B1C&PID_1A20\\SSD', port_path: '1/1/3', class: 'storage', vendor_name: 'Corsair', product_name: 'EX400U', claimed_speed: 'usb4_40',
  })
  const dockHub = device({
    id: 'USB\\VID_2188&PID_5500\\DOCK', port_path: '1/1', class: 'hub', vendor_name: 'CalDigit', product: 'TS4 USB3.2 Gen2 HUB', claimed_speed: 'ss10',
    hub: hub([port(1, undefined), port(2, undefined), port(3, ssd)]),
  })
  const mouse = device({ id: 'USB\\VID_046D&PID_C52B\\MOUSE', port_path: '1/2', class: 'hid', description: 'USB Input Device', claimed_speed: 'full' })
  const root = device({
    id: 'USB\\ROOT_HUB30\\ROOT', port_path: '1', class: 'hub', description: 'USB Root Hub (USB 3.0)',
    hub: hub([
      port(1, dockHub, { connector: connector(true) }),
      port(2, mouse, { negotiated_link: 'full', max_link: 'high', connector: connector(false) }),
      port(3, undefined, { connector: connector(false), label: 'USB-A Data', position: 'rear' }),
    ], { kind: 'root', depth: 0, port_count: 4, device_path: 'USB#ROOT_HUB30#4&TEST&0&0#{f18a0e88-c30c-11d0-8815-00a0c906bed8}' }),
  })
  return topology([controller(root)])
}
