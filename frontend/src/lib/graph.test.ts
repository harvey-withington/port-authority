import { describe, expect, it } from 'vitest'
import {
  COLUMN_GAP, GROUP_GAP, NODE_HEIGHT, NODE_WIDTH, PADDING, ROW_GAP, SOCKET_PAD, SOCKET_PITCH,
  descendantCount, edgeWidth, layoutGraph, socketStripHeight, subtreeBps, utilization,
} from './graph'
import { LINK_ORDER } from './link'
import { applySample, emptyThroughput } from './throughput'
import { controller, device, hub, port, sampleTopology, topology } from './fixtures.test-helpers'

const SSD = 'USB\\VID_1B1C&PID_1A20\\SSD'
const DOCK = 'USB\\VID_2188&PID_5500\\DOCK'
const MOUSE = 'USB\\VID_046D&PID_C52B\\MOUSE'
const allOpen = (): boolean => true

/** No two nodes in the same column may share vertical space. */
function expectNoOverlap(layout: ReturnType<typeof layoutGraph>): void {
  const byColumn = new Map<number, { y: number; height: number }[]>()
  for (const n of layout.nodes) byColumn.set(n.depth, [...(byColumn.get(n.depth) ?? []), n])
  for (const column of byColumn.values()) {
    const sorted = [...column].sort((a, b) => a.y - b.y)
    for (let i = 1; i < sorted.length; i++) {
      expect(sorted[i].y).toBeGreaterThanOrEqual(sorted[i - 1].y + sorted[i - 1].height)
    }
  }
}

function rootOf(): NonNullable<ReturnType<typeof sampleTopology>['controllers'][0]['root_hub']> {
  const root = sampleTopology().controllers[0].root_hub
  if (!root) throw new Error('fixture has no root hub')
  return root
}

describe('edgeWidth', () => {
  it('grows with link speed and floors unknown links', () => {
    const known = LINK_ORDER.filter((s) => s !== 'unknown' && s !== 'none')
    for (let i = 1; i < known.length; i++) expect(edgeWidth(known[i])).toBeGreaterThan(edgeWidth(known[i - 1]))
    expect(edgeWidth('unknown')).toBe(edgeWidth('low'))
    expect(edgeWidth('none')).toBe(edgeWidth('low'))
  })
})

describe('subtreeBps and utilization', () => {
  it('sums the samples of every device behind a hub, case-insensitively', () => {
    let map = emptyThroughput()
    map = applySample(map, { device_id: SSD, at: '', read_bps: 4_000_000_000, write_bps: 1_000_000_000 }, 0)
    map = applySample(map, { device_id: MOUSE.toLowerCase(), at: '', read_bps: 1_000, write_bps: 0 }, 0)
    expect(subtreeBps(rootOf(), map)).toBe(5_000_001_000)
    expect(descendantCount(rootOf())).toBe(3)
  })

  it('clamps utilization to the link and treats unknown links as empty', () => {
    expect(utilization(5_000_000_000, 'ss10')).toBe(0.5)
    expect(utilization(50_000_000_000, 'ss10')).toBe(1)
    expect(utilization(1, 'unknown')).toBe(0)
    expect(utilization(0, 'ss10')).toBe(0)
  })
})

describe('layoutGraph', () => {
  it('returns nothing for an empty snapshot', () => {
    expect(layoutGraph(null, { isExpanded: allOpen, throughput: emptyThroughput() })).toEqual({ width: 0, height: 0, nodes: [], edges: [] })
  })

  it('folds the root hub into the controller and places children one column right', () => {
    const layout = layoutGraph(sampleTopology(), { isExpanded: allOpen, throughput: emptyThroughput() })
    expect(layout.nodes.map((n) => n.kind)).toEqual(['controller', 'device', 'device', 'device'])
    const ctrl = layout.nodes[0]
    const dock = layout.nodes.find((n) => n.id === DOCK)
    const ssd = layout.nodes.find((n) => n.id === SSD)
    expect(ctrl.x).toBe(PADDING)
    expect(ctrl.device?.id).toBe('USB\\ROOT_HUB30\\ROOT')
    expect(dock?.x).toBe(PADDING + NODE_WIDTH + COLUMN_GAP)
    expect(ssd?.depth).toBe(2)
    expect(layout.width).toBe(PADDING + 3 * NODE_WIDTH + 2 * COLUMN_GAP + PADDING)
  })

  it('centres a parent over its children and never overlaps rows', () => {
    const layout = layoutGraph(sampleTopology(), { isExpanded: allOpen, throughput: emptyThroughput() })
    const ctrl = layout.nodes[0]
    const dock = layout.nodes.find((n) => n.id === DOCK)
    const mouse = layout.nodes.find((n) => n.id === MOUSE)
    if (!dock || !mouse) throw new Error('missing nodes')
    // Nodes have different heights now, so the centres line up, not the tops.
    expect(ctrl.y + ctrl.height / 2).toBe((dock.y + mouse.y + mouse.height) / 2)
    expectNoOverlap(layout)
    expect(layout.height).toBeGreaterThanOrEqual(Math.max(...layout.nodes.map((n) => n.y + n.height)))
  })

  it('draws one edge per link with health, width and utilisation', () => {
    let map = emptyThroughput()
    map = applySample(map, { device_id: SSD, at: '', read_bps: 5_000_000_000, write_bps: 0 }, 0)
    const layout = layoutGraph(sampleTopology(), { isExpanded: allOpen, throughput: map })
    expect(layout.edges).toHaveLength(3)
    const toSsd = layout.edges.find((e) => e.to === SSD)
    expect(toSsd?.speed).toBe('ss10')
    expect(toSsd?.health).toBe('good')
    expect(toSsd?.utilization).toBe(0.5)
    expect(toSsd?.width).toBe(edgeWidth('ss10'))
    const toDock = layout.edges.find((e) => e.to === DOCK)
    expect(toDock?.bps).toBe(5_000_000_000)
    const toMouse = layout.edges.find((e) => e.to === MOUSE)
    expect(toMouse?.speed).toBe('full')
    expect(toMouse?.health).toBe('good')
    const dock = layout.nodes.find((n) => n.id === DOCK)
    if (!dock) throw new Error('missing dock')
    expect(toDock?.x1).toBe(PADDING + NODE_WIDTH)
    expect(toDock?.y2).toBe(dock.y + dock.height / 2)
  })

  it('hides the subtree of a collapsed hub and counts it', () => {
    const layout = layoutGraph(sampleTopology(), { isExpanded: (id) => id.toUpperCase() !== DOCK, throughput: emptyThroughput() })
    expect(layout.nodes.find((n) => n.id === SSD)).toBeUndefined()
    expect(layout.nodes.find((n) => n.id === DOCK)?.hiddenCount).toBe(1)
    expect(layout.edges).toHaveLength(2)
  })

  it('separates controllers with a gap and chains USB4 routers', () => {
    const cam = device({ id: 'CAM', class: 'video' })
    const second = controller(
      device({ id: 'ROOT2', class: 'hub', hub: hub([port(1, cam)], { kind: 'root' }) }),
      { id: 'PCI\\CTRL2', name: 'Second' },
    )
    const t = topology([...sampleTopology().controllers, second], {
      usb4: [
        { id: 'R0', instance_id: 'R0', name: 'Host', vendor_id: 0, product_id: 0, kind: 'host', depth: 0 },
        { id: 'R1', instance_id: 'R1', name: 'TS4', vendor_id: 0, product_id: 0, kind: 'device', depth: 1, parent_id: 'r0', children: ['NVMe (PCI\\X)'] },
      ],
    })
    const layout = layoutGraph(t, { isExpanded: allOpen, throughput: emptyThroughput() })
    const ctrls = layout.nodes.filter((n) => n.kind === 'controller')
    expect(ctrls).toHaveLength(2)
    expect(ctrls[1].y).toBeGreaterThan(ctrls[0].y + NODE_HEIGHT)
    const routers = layout.nodes.filter((n) => n.kind === 'router')
    expect(routers.map((n) => n.depth)).toEqual([0, 1])
    const carried = layout.nodes.find((n) => n.kind === 'carried')
    expect(carried?.label).toBe('NVMe (PCI\\X)')
    expect(carried?.depth).toBe(2)
    expect(layout.edges.find((e) => e.from === 'R0' && e.to === 'R1')?.speed).toBe('usb4_40')
    expect(layout.edges.find((e) => e.to === carried?.id)?.health).toBe('idle')
    expect(layout.height).toBeGreaterThan(routers[1].y)
  })
})

describe('socket strips', () => {
  it('gives a hub-like node one slot per visible port and grows it to fit', () => {
    const layout = layoutGraph(sampleTopology(), { isExpanded: allOpen, throughput: emptyThroughput() })
    const ctrl = layout.nodes[0]
    expect(ctrl.sockets.map((s) => s.port.number)).toEqual([1, 2, 3])
    expect(ctrl.sockets.map((s) => s.y)).toEqual([22, 46, 70])
    expect(ctrl.sockets.map((s) => s.occupied)).toEqual([true, true, false])
    expect(ctrl.height).toBe(socketStripHeight(3))
    expect(ctrl.height).toBe(SOCKET_PAD * 2 + 3 * SOCKET_PITCH)
  })

  it('leaves a leaf device at the plain node height with no sockets', () => {
    const layout = layoutGraph(sampleTopology(), { isExpanded: allOpen, throughput: emptyThroughput() })
    const mouse = layout.nodes.find((n) => n.id === MOUSE)
    expect(mouse?.sockets).toEqual([])
    expect(mouse?.height).toBe(NODE_HEIGHT)
    expect(socketStripHeight(1)).toBe(NODE_HEIGHT)
  })

  it('starts each edge at the socket the child is plugged into', () => {
    const layout = layoutGraph(sampleTopology(), { isExpanded: allOpen, throughput: emptyThroughput() })
    const ctrl = layout.nodes[0]
    const dock = layout.nodes.find((n) => n.id === DOCK)
    if (!dock) throw new Error('missing dock')
    // The dock is on root port 1 and the mouse on port 2: two different sockets.
    expect(layout.edges.find((e) => e.to === DOCK)?.y1).toBe(ctrl.y + ctrl.sockets[0].y)
    expect(layout.edges.find((e) => e.to === MOUSE)?.y1).toBe(ctrl.y + ctrl.sockets[1].y)
    // The SSD hangs off the dock's third socket.
    expect(layout.edges.find((e) => e.to === SSD)?.y1).toBe(dock.y + dock.sockets[2].y)
  })

  it('shifts a tall hub subtree down instead of letting it ride over the group above', () => {
    const a = device({ id: 'BIG_A', class: 'storage' })
    const b = device({ id: 'BIG_B', class: 'hid' })
    const ports = Array.from({ length: 16 }, (_, i) => port(i + 1, i === 3 ? a : i === 8 ? b : undefined))
    const big = controller(
      device({ id: 'BIG_ROOT', class: 'hub', hub: hub(ports, { kind: 'root', depth: 0, port_count: 16 }) }),
      { id: 'PCI\BIG', name: 'Big' },
    )
    const next = controller(
      device({ id: 'NEXT_ROOT', class: 'hub', hub: hub([port(1, device({ id: 'CAM', class: 'video' }))], { kind: 'root' }) }),
      { id: 'PCI\NEXT', name: 'Next' },
    )
    const layout = layoutGraph(topology([big, next]), { isExpanded: allOpen, throughput: emptyThroughput() })
    const bigNode = layout.nodes[0]
    const nextNode = layout.nodes.find((n) => n.id === 'PCI\NEXT')
    const devA = layout.nodes.find((n) => n.id === 'BIG_A')
    if (!nextNode || !devA) throw new Error('missing nodes')
    expect(bigNode.height).toBe(SOCKET_PAD * 2 + 16 * SOCKET_PITCH)
    // The hub is taller than its two children, so it stays on the row it
    // started and the whole subtree slides down under it.
    expect(bigNode.y).toBe(PADDING)
    expect(devA.y).toBeGreaterThan(PADDING)
    expect(nextNode.y).toBeGreaterThanOrEqual(bigNode.y + bigNode.height + GROUP_GAP - ROW_GAP)
    expectNoOverlap(layout)
    expect(layout.height).toBeGreaterThanOrEqual(Math.max(...layout.nodes.map((n) => n.y + n.height)))
  })
})
