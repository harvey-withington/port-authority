import { describe, expect, it } from 'vitest'
import type { Device, Enclosure, Topology } from './api/types'
import { buildPhysical, enclosureIdOf, memberIds, type PhysicalNode } from './physical'
import { layoutGraph, layoutPhysical } from './graph'
import { emptyThroughput } from './throughput'
import { connector, controller, device, hub, port, topology } from './fixtures.test-helpers'

const HOST: Enclosure = { id: 'host', kind: 'host' }
const DOCK: Enclosure = { id: 'dock:caldigit-ts4:1', kind: 'dock', name: 'CalDigit TS4', dock_id: 'caldigit-ts4' }

const allOpen = (): boolean => true

/** Find a node by id anywhere below the given roots. */
function findNode(roots: PhysicalNode[], id: string): PhysicalNode | null {
  for (const root of roots) {
    if (root.id === id) return root
    const found = findNode(root.children.map((c) => c.node), id)
    if (found) return found
  }
  return null
}

/**
 * The shape Windows reports for a laptop with a TS4 attached: the dock's
 * USB 2 half hangs off one controller and its USB 3 half off another, so
 * the same physical dock appears as two chains reached by two cables.
 */
function dockedTopology(): Topology {
  const keyboard = device({ id: 'USB\\KEYBOARD', class: 'hid', description: 'Keyboard' })
  const ssd = device({ id: 'USB\\SSD', class: 'storage', product_name: 'EX400U' })

  const usb2Inner = device({
    id: 'USB\\TS4_USB2_B', class: 'hub', enclosure: DOCK,
    hub: hub([port(1, keyboard), port(2, undefined)]),
  })
  const usb2Top = device({
    id: 'USB\\TS4_USB2_A', class: 'hub', enclosure: DOCK,
    hub: hub([port(1, usb2Inner)]),
  })
  const usb3Inner = device({
    id: 'USB\\TS4_USB3_B', class: 'hub', enclosure: DOCK,
    hub: hub([port(1, ssd), port(2, undefined)]),
  })
  const usb3Top = device({
    id: 'USB\\TS4_USB3_A', class: 'hub', enclosure: DOCK,
    hub: hub([port(1, usb3Inner)]),
  })

  const camera = device({ id: 'USB\\CAMERA', class: 'video', description: 'Integrated Camera' })
  const root1 = device({
    id: 'USB\\ROOT1', class: 'hub', enclosure: HOST,
    // The dock's USB 2 uplink: same hole in the chassis as root2 port 1,
    // but only 480 Mbps.
    hub: hub([port(1, usb2Top, { negotiated_link: 'high', max_link: 'high', connector: connector(true) })], { kind: 'root', depth: 0 }),
  })
  const root2 = device({
    id: 'USB\\ROOT2', class: 'hub', enclosure: HOST,
    hub: hub([
      port(1, usb3Top, { connector: connector(true) }),
      port(2, camera, { connector: { type_c: false, user_connectable: false, multiple_companions: false } }),
      port(3, undefined, { connector: connector(false) }),
    ], { kind: 'root', depth: 0 }),
  })

  return topology([
    controller(root1, { id: 'PCI\\CTRL1' }),
    controller(root2, { id: 'PCI\\CTRL2' }),
  ])
}

describe('enclosureIdOf', () => {
  it('uses the enclosure the backend stamped on the hub', () => {
    expect(enclosureIdOf(device({ id: 'h', class: 'hub', hub: hub([]), enclosure: DOCK }), false)).toBe(DOCK.id)
  })

  it('falls back to the computer for a root hub and to a box of its own otherwise', () => {
    expect(enclosureIdOf(device({ id: 'r', class: 'hub', hub: hub([]) }), true)).toBe('host')
    expect(enclosureIdOf(device({ id: 'h', class: 'hub', hub: hub([]) }), false)).toBe('hub:h')
  })

  it('gives a leaf device no enclosure of its own', () => {
    expect(enclosureIdOf(device({ id: 'mouse' }), false)).toBeNull()
  })
})

describe('buildPhysical', () => {
  it('folds every controller into one computer', () => {
    const roots = buildPhysical(dockedTopology())
    expect(roots).toHaveLength(1)
    expect(roots[0].id).toBe('host')
    expect(roots[0].kind).toBe('host')
    expect(memberIds(roots[0]).sort()).toEqual(['USB\\ROOT1', 'USB\\ROOT2'])
  })

  it('folds a dock reached over two controllers into one box behind one cable', () => {
    const roots = buildPhysical(dockedTopology())
    const host = roots[0]
    const dockChildren = host.children.filter((c) => c.node.id === DOCK.id)
    expect(dockChildren).toHaveLength(1)

    const dock = dockChildren[0].node
    expect(dock.kind).toBe('dock')
    expect(dock.enclosure?.name).toBe('CalDigit TS4')
    expect(memberIds(dock).sort()).toEqual(['USB\\TS4_USB2_A', 'USB\\TS4_USB2_B', 'USB\\TS4_USB3_A', 'USB\\TS4_USB3_B'])
  })

  it('keeps the faster half of a socket seen twice and remembers the other', () => {
    const host = buildPhysical(dockedTopology())[0]
    const uplink = host.children.find((c) => c.node.id === DOCK.id)?.socket
    expect(uplink).toBeDefined()
    // Root 2's 10 Gbps side wins over root 1's 480 Mbps companion.
    expect(uplink?.port.max_link).toBe('ss10')
    expect(uplink?.also.map((p) => p.max_link)).toEqual(['high'])
    // And the slower half is not drawn a second time.
    expect(host.sockets.filter((s) => s.port.device !== undefined)).toHaveLength(2)
  })

  it('hides the wiring between hubs in one box and keeps the sockets facing out', () => {
    const dock = findNode(buildPhysical(dockedTopology()), DOCK.id)
    expect(dock).not.toBeNull()
    // Four member hubs, but the two ports that only join them are internal
    // to the box: what is left is the keyboard, the SSD and two empties.
    expect(dock?.sockets).toHaveLength(4)
    expect(dock?.children.map((c) => c.node.id).sort()).toEqual(['USB\\KEYBOARD', 'USB\\SSD'])
  })

  it('puts the sockets a person can reach before the soldered-in ones', () => {
    const host = buildPhysical(dockedTopology())[0]
    const internalAt = host.sockets.findIndex((s) => s.port.connector?.user_connectable === false)
    expect(internalAt).toBe(host.sockets.length - 1)
  })

  it('marks a box whose hub could not be read to the end', () => {
    // What the Windows collector produces when a hub will not open: the
    // hub is in the tree, its ports are not. Drawing that as an empty box
    // would claim nothing is plugged in, which is not what was measured.
    const half = device({
      id: 'USB\HALF', class: 'hub', enclosure: { id: 'hub:half', kind: 'hub' },
      hub: hub([], { incomplete: true }),
    })
    const root = device({ id: 'USB\ROOT', class: 'hub', enclosure: HOST, hub: hub([port(1, half)], { kind: 'root', depth: 0 }) })
    const roots = buildPhysical(topology([controller(root)]))

    expect(findNode(roots, 'hub:half')?.incomplete).toBe(true)
    expect(roots[0].incomplete).toBe(false)
  })

  it('returns nothing for an empty snapshot', () => {
    expect(buildPhysical(null)).toEqual([])
  })

  it('gives each unannotated hub a box of its own, so nothing merges by guess', () => {
    const inner = device({ id: 'USB\\INNER', class: 'hub', hub: hub([port(1, device({ id: 'USB\\LEAF' }))]) })
    const outer = device({ id: 'USB\\OUTER', class: 'hub', hub: hub([port(1, inner)]) })
    const root = device({ id: 'USB\\ROOT', class: 'hub', hub: hub([port(1, outer)], { kind: 'root', depth: 0 }) })
    const roots = buildPhysical(topology([controller(root)]))

    expect(roots.map((r) => r.id)).toEqual(['host'])
    expect(findNode(roots, 'hub:USB\\OUTER')).not.toBeNull()
    expect(findNode(roots, 'hub:USB\\INNER')).not.toBeNull()
  })
})

describe('layoutPhysical', () => {
  it('draws one box per enclosure with the sockets on its edge', () => {
    const layout = layoutPhysical(dockedTopology(), { isExpanded: allOpen, throughput: emptyThroughput() })
    const boxes = layout.nodes.filter((n) => n.kind === 'box')
    expect(boxes.map((n) => n.id)).toEqual(['host', DOCK.id])

    const host = boxes[0]
    expect(host.members).toHaveLength(2)
    expect(host.sockets.length).toBe(3)
    // Socket keys stay unique even though both root hubs have a port 1.
    expect(new Set(host.sockets.map((s) => s.key)).size).toBe(host.sockets.length)
  })

  it('draws one cable to the dock, not one per controller', () => {
    const layout = layoutPhysical(dockedTopology(), { isExpanded: allOpen, throughput: emptyThroughput() })
    expect(layout.edges.filter((e) => e.to === DOCK.id)).toHaveLength(1)
    expect(layout.edges.find((e) => e.to === DOCK.id)?.speed).toBe('ss10')
  })

  it('starts a cable at the socket it leaves from', () => {
    const layout = layoutPhysical(dockedTopology(), { isExpanded: allOpen, throughput: emptyThroughput() })
    const host = layout.nodes.find((n) => n.id === 'host')
    const edge = layout.edges.find((e) => e.to === DOCK.id)
    const slot = host?.sockets.find((s) => s.port.device?.id === 'USB\\TS4_USB3_A')
    expect(slot).toBeDefined()
    expect(edge?.y1).toBe((host?.y ?? 0) + (slot?.y ?? 0))
  })

  it('collapses a box by its enclosure id and counts what it hides', () => {
    const collapsed = layoutPhysical(dockedTopology(), { isExpanded: (id) => id !== DOCK.id, throughput: emptyThroughput() })
    expect(collapsed.nodes.map((n) => n.id)).not.toContain('USB\\SSD')
    expect(collapsed.nodes.find((n) => n.id === DOCK.id)?.hiddenCount).toBe(2)
  })

  it('leaves the logical view alone: a snapshot with no enclosures still draws', () => {
    const layout = layoutPhysical(topology([controller(device({ id: 'USB\\ROOT', class: 'hub', hub: hub([], { kind: 'root', depth: 0 }) }))]), {
      isExpanded: allOpen,
      throughput: emptyThroughput(),
    })
    expect(layout.nodes.map((n) => n.id)).toEqual(['host'])
  })
})

describe('the USB4 fabric in the physical view', () => {
  const withRouters = (): Topology => {
    const root = device({ id: 'USB\ROOT', class: 'hub', enclosure: HOST, hub: hub([port(1, undefined)], { kind: 'root', depth: 0 }) })
    return topology([controller(root)], {
      usb4: [
        { id: 'USB4\HOST', instance_id: 'USB4\HOST', name: 'USB4 Root Router (1.0)', vendor_id: 0x8086, product_id: 0xe433, kind: 'host', depth: 0 },
        {
          id: 'USB4\SSD', instance_id: 'USB4\SSD', name: 'USB4 Router (2.0), Phison - PS2321',
          product_name: 'Corsair EX400U', vendor_id: 0x13fe, product_id: 0x6900, kind: 'device',
          parent_id: 'USB4\HOST', depth: 1, children: ['NVMe disk (D:)'],
        },
      ],
    })
  }

  it('hangs a plugged-in USB4 device off the computer, not off a group of its own', () => {
    const layout = layoutPhysical(withRouters(), { isExpanded: allOpen, throughput: emptyThroughput() })
    const edge = layout.edges.find((e) => e.to === 'USB4\SSD')
    expect(edge?.from).toBe('host')
    expect(edge?.speed).toBe('usb4_40')
    // Its PCIe-tunnelled disk still hangs off the drive.
    expect(layout.edges.some((e) => e.from === 'USB4\SSD' && e.to.includes('carried'))).toBe(true)
  })

  it('folds the computer\u2019s own host routers into it rather than drawing them', () => {
    const layout = layoutPhysical(withRouters(), { isExpanded: allOpen, throughput: emptyThroughput() })
    expect(layout.nodes.map((n) => n.id)).not.toContain('USB4\HOST')
    expect(layout.nodes.find((n) => n.id === 'host')?.hostRouters).toBe(1)
  })

  it('keeps the fabric as its own group in the logical view', () => {
    const layout = layoutGraph(withRouters(), { isExpanded: allOpen, throughput: emptyThroughput() })
    expect(layout.nodes.map((n) => n.id)).toContain('USB4\HOST')
    expect(layout.edges.find((e) => e.to === 'USB4\SSD')?.from).toBe('USB4\HOST')
  })
})
