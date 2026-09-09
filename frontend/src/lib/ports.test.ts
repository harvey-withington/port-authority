import { describe, expect, it } from 'vitest'
import type { Connector, Port, Topology } from './api/types'
import { companionPort, indexHubPaths, isUsb4, normalizeHubPath, socketKind, visiblePorts } from './ports'
import { controller, device, hub, port, sampleTopology, topology } from './fixtures.test-helpers'

const GUID = '{f18a0e88-c30c-11d0-8815-00a0c906bed8}'
const PATH_A = `USB#ROOT_HUB30#4&34c97ea0&0&0#${GUID}`
const PATH_B = `USB#ROOT_HUB30#4&5e9e03d&0&0#${GUID}`

function connector(overrides: Partial<Connector> = {}): Connector {
  return { type_c: false, user_connectable: true, multiple_companions: false, ...overrides }
}

/** A port on the other half of a physical socket. */
function twin(path: string, number: number, extra: Partial<Connector> = {}): Connector {
  return connector({ type_c: true, multiple_companions: true, companion_hub_path: path, companion_port: number, ...extra })
}

describe('socketKind', () => {
  it('falls back to unknown when the provider reports no connector', () => {
    expect(socketKind(port(1, undefined))).toBe('unknown')
  })

  it('calls a port that nobody can reach internal, whatever its shape', () => {
    expect(socketKind(port(1, undefined, { connector: connector({ user_connectable: false }) }))).toBe('internal')
    expect(socketKind(port(1, undefined, { connector: connector({ user_connectable: false, type_c: true }) }))).toBe('internal')
  })

  it('separates USB-C from USB-A on user-facing ports', () => {
    expect(socketKind(port(1, undefined, { connector: connector({ type_c: true }) }))).toBe('usb-c')
    expect(socketKind(port(1, undefined, { connector: connector({ type_c: false }) }))).toBe('usb-a')
  })
})

describe('isUsb4', () => {
  it('is true only for the USB4 port maximums', () => {
    for (const max of ['usb4_20', 'usb4_40', 'usb4_80'] as const) {
      expect(isUsb4(port(1, undefined, { max_link: max }))).toBe(true)
    }
    for (const max of ['unknown', 'none', 'high', 'ss10', 'ss20'] as const) {
      expect(isUsb4(port(1, undefined, { max_link: max }))).toBe(false)
    }
  })
})

describe('normalizeHubPath', () => {
  it('strips a device-namespace prefix and compares case-insensitively', () => {
    expect(normalizeHubPath(`\\\\?\\${PATH_A}`)).toBe(normalizeHubPath(PATH_A.toUpperCase()))
    expect(normalizeHubPath(`\\\\.\\${PATH_A}`)).toBe(normalizeHubPath(` ${PATH_A} `))
  })
})

/**
 * A USB 3 root hub and a USB 2 root hub that expose the same three chassis
 * sockets: A1/B5 and A2/B6 are empty pairs, A3/B7 has the device on the
 * slower half.
 */
function twoHubTopology() {
  const flash = device({ id: 'USB\\FLASH', class: 'storage' })
  const hubA = device({
    id: 'USB\\ROOT_A', class: 'hub',
    hub: hub([
      port(1, undefined, { max_link: 'ss10', connector: twin(PATH_B, 5) }),
      port(2, flash, { max_link: 'ss10', connector: twin(PATH_B, 6) }),
      port(3, undefined, { max_link: 'ss10', connector: twin(PATH_B, 7) }),
      port(4, undefined, { max_link: 'ss10', connector: twin('USB#GONE#0', 1) }),
    ], { kind: 'root', device_path: PATH_A }),
  })
  const keyboard = device({ id: 'USB\\KEYBOARD', class: 'hid' })
  const hubB = device({
    id: 'USB\\ROOT_B', class: 'hub',
    hub: hub([
      port(5, undefined, { max_link: 'high', connector: twin(PATH_A.toUpperCase(), 1) }),
      port(6, undefined, { max_link: 'high', connector: twin(PATH_A, 2) }),
      port(7, keyboard, { max_link: 'high', negotiated_link: 'high', connector: twin(PATH_A, 3) }),
    ], { kind: 'root', device_path: `\\\\?\\${PATH_B}` }),
  })
  return {
    hubA, hubB,
    topology: topology([
      controller(hubA, { id: 'PCI\\CTRL_A' }),
      controller(hubB, { id: 'PCI\\CTRL_B' }),
    ]),
  }
}

describe('visiblePorts across two hubs', () => {
  it('keeps the faster half of every companion pair', () => {
    const { hubA, topology: t } = twoHubTopology()
    expect(visiblePorts(hubA, t).map((p) => p.number)).toEqual([1, 2, 3, 4])
  })

  it('drops the empty slower half but never a port with a device', () => {
    const { hubB, topology: t } = twoHubTopology()
    expect(visiblePorts(hubB, t).map((p) => p.number)).toEqual([7])
  })

  it('keeps a port whose companion hub is not in the snapshot', () => {
    const { hubA, topology: t } = twoHubTopology()
    const index = indexHubPaths(t)
    const orphan = hubA.hub?.ports?.find((p) => p.number === 4) as Port
    expect(companionPort(orphan, index)).toBeNull()
    expect(visiblePorts(hubA, t, index)).toContain(orphan)
  })

  it('indexes hubs by path regardless of prefix or case', () => {
    const { hubB, topology: t } = twoHubTopology()
    expect(indexHubPaths(t).get(normalizeHubPath(PATH_B))).toBe(hubB)
    expect(indexHubPaths(null).size).toBe(0)
  })

  it('sorts by port number and accepts a prebuilt index', () => {
    const { hubA, topology: t } = twoHubTopology()
    const ports = hubA.hub?.ports
    if (!ports) throw new Error('fixture has no ports')
    ports.reverse()
    expect(visiblePorts(hubA, null, indexHubPaths(t)).map((p) => p.number)).toEqual([1, 2, 3, 4])
  })
})

describe('visiblePorts within one root hub', () => {
  it('merges the USB 2 and USB 3 halves of the same socket and breaks ties by port number', () => {
    const root = device({
      id: 'USB\\ROOT_ONE', class: 'hub',
      hub: hub([
        port(1, undefined, { max_link: 'high', connector: twin(PATH_A, 3) }),
        port(3, undefined, { max_link: 'ss10', connector: twin(PATH_A, 1) }),
        // Same maximum on both halves: the lower port number wins.
        port(5, undefined, { max_link: 'ss10', connector: twin(PATH_A, 6) }),
        port(6, undefined, { max_link: 'ss10', connector: twin(PATH_A, 5) }),
      ], { kind: 'root', device_path: PATH_A }),
    })
    const t = topology([controller(root)])
    expect(visiblePorts(root, t).map((p) => p.number)).toEqual([3, 5])
  })
})

describe('visiblePorts without connector data', () => {
  it('keeps every port of an old snapshot', () => {
    const t = sampleTopology()
    const dock = t.controllers[0].root_hub?.hub?.ports?.[0].device
    if (!dock) throw new Error('fixture has no dock')
    for (const p of dock.hub?.ports ?? []) expect(p.connector).toBeUndefined()
    expect(visiblePorts(dock, t).map((p) => p.number)).toEqual([1, 2, 3])
  })

  it('returns nothing for a device that is not a hub', () => {
    expect(visiblePorts(device({ id: 'USB\\MOUSE' }), null)).toEqual([])
  })
})

// A hub the collector could not open has a nil Ports slice, which Go
// marshals as `"ports": null`, not `[]`. That shape arrives whenever a
// dock is replugged while the app is running, so nothing that walks the
// tree may assume ports is an array.
describe('a hub whose ports could not be read', () => {
  const withNullPorts = (): Topology => {
    const halfRead = device({
      id: 'USB\VID_2109&PID_2822\HALF', class: 'hub',
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      hub: { kind: 'usb2', depth: 1, port_count: 0, bus_powered: false, incomplete: true, ports: null as any, device_path: 'USB#VID_2109&PID_2822#X' },
    })
    const root = device({
      id: 'USB\ROOT', class: 'hub',
      hub: hub([port(1, halfRead, { connector: connector({ type_c: true }) })], { kind: 'root', depth: 0, device_path: 'USB#ROOT_HUB30#X' }),
    })
    return topology([controller(root)])
  }

  it('can still be indexed by device path', () => {
    expect(() => indexHubPaths(withNullPorts())).not.toThrow()
  })

  it('reports no visible ports rather than failing', () => {
    const t = withNullPorts()
    const half = t.controllers[0].root_hub!.hub!.ports![0].device!
    expect(visiblePorts(half, t)).toEqual([])
  })

  it('resolves a companion that points at it without failing', () => {
    const t = withNullPorts()
    const index = indexHubPaths(t)
    const p = port(2, undefined, {
      connector: { type_c: true, user_connectable: true, multiple_companions: false, companion_hub_path: 'USB#VID_2109&PID_2822#X', companion_port: 1 },
    })
    expect(() => companionPort(p, index)).not.toThrow()
    expect(companionPort(p, index)).toBeNull()
  })
})
