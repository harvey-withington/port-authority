// Smoke tests: each shared component renders real data without runtime
// errors and shows the text a user relies on.
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { tick } from 'svelte'
import { fireEvent, render, screen } from '@testing-library/svelte'
import TopologyTree from './TopologyTree.svelte'
import TopologyGraph from './TopologyGraph.svelte'
import ViewSwitch from './ViewSwitch.svelte'
import ZoomControl from './ZoomControl.svelte'
import InsightList from './InsightList.svelte'
import Timeline from './Timeline.svelte'
import LinkBadge from './LinkBadge.svelte'
import ThroughputMeter from './ThroughputMeter.svelte'
import Legend from './Legend.svelte'
import ConnectionBanner from './ConnectionBanner.svelte'
import Header from './Header.svelte'
import { indexTopology } from '../lib/topology'
import { visiblePorts } from '../lib/ports'
import { applySample, emptyThroughput } from '../lib/throughput'
import { flaggedDevices } from '../lib/insights'
import { zoom } from '../lib/zoom.svelte'
import { insight, sampleTopology } from '../lib/fixtures.test-helpers'
import type { TreeContext } from '../lib/tree'

const topology = sampleTopology()
const index = indexTopology(topology)
const finding = insight()
const root = topology.controllers[0].root_hub
if (!root) throw new Error('fixture has no root hub')
const dock = root.hub?.ports?.[0].device
if (!dock) throw new Error('fixture has no dock')

describe('TopologyTree', () => {
  it('renders controllers, devices, link badges, meters and warnings', () => {
    const ctx: TreeContext = {
      flagged: flaggedDevices([finding]),
      throughput: applySample(emptyThroughput(), { device_id: 'USB\\VID_1B1C&PID_1A20\\SSD', at: '', read_bps: 800_000_000, write_bps: 0 }, 0),
      showMeter: true,
    }
    const { container } = render(TopologyTree, { topology: { ...topology, warnings: ['hub 3 would not open'] }, ctx, loading: false })
    expect(screen.getByText('Test xHCI')).toBeInTheDocument()
    expect(screen.getByText('Corsair EX400U')).toBeInTheDocument()
    expect(screen.getByText('USB Input Device')).toBeInTheDocument()
    expect(screen.getByText('hub 3 would not open')).toBeInTheDocument()
    expect(screen.getByText('100.0 MB/s')).toBeInTheDocument()
    expect(screen.getAllByTitle('Mentioned in a finding').length).toBeGreaterThan(0)
    // Every device row shows the socket it is plugged into next to its port number.
    expect(container.querySelectorAll('.socket')).toHaveLength(3)
    expect(container.querySelectorAll('.socket.empty')).toHaveLength(0)
  })

  it('collapses and expands a hub', async () => {
    const ctx: TreeContext = { flagged: new Map(), throughput: emptyThroughput(), showMeter: false }
    render(TopologyTree, { topology, ctx, loading: false })
    const toggle = screen.getByLabelText('Collapse CalDigit TS4 USB3.2 Gen2 HUB')
    await fireEvent.click(toggle)
    expect(screen.queryByText('Corsair EX400U')).not.toBeInTheDocument()
    await fireEvent.click(screen.getByLabelText('Expand CalDigit TS4 USB3.2 Gen2 HUB'))
    expect(screen.getByText('Corsair EX400U')).toBeInTheDocument()
  })

  it('shows the loading and empty states', () => {
    const ctx: TreeContext = { flagged: new Map(), throughput: emptyThroughput(), showMeter: false }
    const { unmount } = render(TopologyTree, { topology: null, ctx, loading: true })
    expect(screen.getByText('Reading the USB tree...')).toBeInTheDocument()
    unmount()
    render(TopologyTree, { topology: { ...topology, controllers: [] }, ctx, loading: false })
    expect(screen.getByText('No USB controllers were found.')).toBeInTheDocument()
  })
})

describe('TopologyGraph', () => {
  const busy: TreeContext = {
    flagged: flaggedDevices([finding]),
    throughput: applySample(emptyThroughput(), { device_id: 'USB\\VID_1B1C&PID_1A20\\SSD', at: '', read_bps: 800_000_000, write_bps: 0 }, 0),
    showMeter: true,
  }
  const quiet: TreeContext = { flagged: new Map(), throughput: emptyThroughput(), showMeter: false }

  it('draws a node per device, an edge per link, flow on busy links and pulses flagged devices', async () => {
    const { container } = render(TopologyGraph, { topology: { ...topology, warnings: ['hub 3 would not open'] }, ctx: busy, loading: false, detail: 'logical' })
    await tick()
    expect(screen.getByText('Test xHCI')).toBeInTheDocument()
    expect(screen.getByText('Corsair EX400U')).toBeInTheDocument()
    expect(screen.getByText('USB Input Device')).toBeInTheDocument()
    expect(screen.getByText('hub 3 would not open')).toBeInTheDocument()
    expect(container.querySelectorAll('.node')).toHaveLength(4)
    expect(container.querySelectorAll('.edge')).toHaveLength(3)
    expect(container.querySelectorAll('.edge.health-good')).toHaveLength(3)
    // The SSD's traffic flows on its own link and on the dock's uplink, not on the mouse's.
    expect(container.querySelectorAll('.flow')).toHaveLength(2)
    expect(screen.getAllByRole('meter')).toHaveLength(3)
    expect(container.querySelector('.node.flagged.pulse')).not.toBeNull()
    expect(screen.getAllByTitle('Mentioned in a finding').length).toBeGreaterThan(0)
  })

  it('draws a socket per visible port, including the empty one', async () => {
    const { container } = render(TopologyGraph, { topology, ctx: quiet, loading: false, detail: 'logical' })
    await tick()
    // Root hub: USB-C (dock), USB-A (mouse), empty USB-A. Dock hub: three ports.
    const visible = visiblePorts(root, topology).length + visiblePorts(dock, topology).length
    expect(visible).toBe(6)
    expect(container.querySelectorAll('.socket')).toHaveLength(visible)
    expect(container.querySelectorAll('.socket.empty')).toHaveLength(3)
    expect(screen.getByLabelText('Port 3: USB-A, up to 10 Gbps, empty. USB-A Data (rear)')).toBeInTheDocument()
    expect(screen.getByLabelText('Port 1: USB-C, up to 10 Gbps, in use')).toBeInTheDocument()
  })

  it('collapses a hub, hides its subtree and says how many devices are hidden', async () => {
    const { container } = render(TopologyGraph, { topology, ctx: quiet, loading: false, detail: 'logical' })
    await fireEvent.click(screen.getByLabelText('Collapse CalDigit TS4 USB3.2 Gen2 HUB'))
    expect(screen.queryByText('Corsair EX400U')).not.toBeInTheDocument()
    expect(screen.getByText('1 hidden')).toBeInTheDocument()
    expect(container.querySelectorAll('.edge')).toHaveLength(2)
    await fireEvent.click(screen.getByLabelText('Expand CalDigit TS4 USB3.2 Gen2 HUB'))
    expect(screen.getByText('Corsair EX400U')).toBeInTheDocument()
  })

  it('shows the loading and empty states', () => {
    const { unmount } = render(TopologyGraph, { topology: null, ctx: quiet, loading: true, detail: 'logical' })
    expect(screen.getByText('Reading the USB tree...')).toBeInTheDocument()
    unmount()
    render(TopologyGraph, { topology: { ...topology, controllers: [] }, ctx: quiet, loading: false, detail: 'logical' })
    expect(screen.getByText('No USB controllers were found.')).toBeInTheDocument()
  })
})

describe('ZoomControl', () => {
  // The control lives in the pane header while the diagram it scales is in
  // the pane body, so the level is shared module state; reset it so the
  // test does not depend on what ran before.
  beforeEach(() => {
    zoom.fitToWidth()
    zoom.setFit(1)
  })

  it('zooms in and out and returns to fit', async () => {
    render(ZoomControl)
    expect(screen.getByText('100%')).toBeInTheDocument()
    await fireEvent.click(screen.getByLabelText('Zoom in'))
    await fireEvent.click(screen.getByLabelText('Zoom in'))
    expect(screen.getByText('130%')).toBeInTheDocument()
    await fireEvent.click(screen.getByLabelText('Zoom out'))
    expect(screen.getByText('115%')).toBeInTheDocument()
    await fireEvent.click(screen.getByLabelText('Fit to width'))
    expect(screen.getByText('100%')).toBeInTheDocument()
  })

  it('stops at the ends of the range', async () => {
    render(ZoomControl)
    const out = screen.getByLabelText('Zoom out')
    for (let i = 0; i < 10; i++) await fireEvent.click(out)
    expect(screen.getByText('50%')).toBeInTheDocument()
    expect(out).toBeDisabled()
  })
})

describe('ViewSwitch', () => {
  it('marks the active view and reports a change', async () => {
    const onChange = vi.fn()
    render(ViewSwitch, { value: 'graph', onChange })
    expect(screen.getByRole('button', { name: 'Diagram' })).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByRole('button', { name: 'Tree' })).toHaveAttribute('aria-pressed', 'false')
    await fireEvent.click(screen.getByRole('button', { name: 'Tree' }))
    expect(onChange).toHaveBeenCalledWith('tree')
  })
})

describe('InsightList', () => {
  it('shows the empty state', () => {
    render(InsightList, { insights: [], index, onFocusDevice: () => {} })
    expect(screen.getByText('Everything is connected at the speed it should be.')).toBeInTheDocument()
  })

  it('renders a card with details and resolves device names', async () => {
    const onFocus = vi.fn()
    render(InsightList, { insights: [{ ...finding, evidence: ['negotiated link ss10'] }], index, onFocusDevice: onFocus })
    expect(screen.getByText('SSD could be faster')).toBeInTheDocument()
    await fireEvent.click(screen.getByText('Corsair EX400U'))
    expect(onFocus).toHaveBeenCalledWith('USB\\VID_1B1C&PID_1A20\\SSD')
    await fireEvent.click(screen.getByText('Details'))
    expect(screen.getByText('negotiated link ss10')).toBeInTheDocument()
    expect(screen.getByText('Confidence 90%')).toBeInTheDocument()
  })
})

describe('Timeline', () => {
  it('localises kinds and links device entries', () => {
    render(Timeline, {
      entries: [
        { id: '1', at: new Date().toISOString(), kind: 'device_added', subject: 'Corsair EX400U', deviceId: 'USB\\VID_1B1C&PID_1A20\\SSD' },
        { id: '2', at: new Date().toISOString(), kind: 'connection', subject: 'live', connection: 'live' },
        { id: '3', at: new Date().toISOString(), kind: 'insight_added', subject: 'SSD could be faster', severity: 'warning' },
      ],
      index,
      onFocusDevice: () => {},
    })
    expect(screen.getByText('Live stream connected')).toBeInTheDocument()
    expect(screen.getByText('New finding: SSD could be faster')).toBeInTheDocument()
    expect(screen.getByText('Corsair EX400U')).toBeInTheDocument()
    expect(screen.getAllByText('just now')).toHaveLength(3)
  })
})

describe('small parts', () => {
  it('LinkBadge shows the negotiated speed, a port-max swatch and the health ring', () => {
    const { container } = render(LinkBadge, { negotiated: 'ss10', max: 'usb4_40', claimed: 'usb4_40' })
    expect(screen.getByText('10 Gbps')).toBeInTheDocument()
    expect(screen.getByLabelText('Port max 40 Gbps (USB4 / Thunderbolt)')).toBeInTheDocument()
    expect(container.querySelector('.health-slow')).not.toBeNull()
  })

  it('ThroughputMeter scales to the link and reports idle', () => {
    const { unmount } = render(ThroughputMeter, { sample: { device_id: 'x', at: '', read_bps: 5_000_000_000, write_bps: 0 }, speed: 'ss10', enabled: true })
    expect(screen.getByRole('meter')).toHaveAttribute('aria-valuenow', '50')
    unmount()
    render(ThroughputMeter, { sample: null, speed: 'ss10', enabled: true })
    expect(screen.getByText('idle')).toBeInTheDocument()
  })

  it('Legend lists classes, the speed ramp and the socket shapes', () => {
    const { container } = render(Legend, { onClose: () => {} })
    expect(screen.getByText('Storage')).toBeInTheDocument()
    expect(screen.getByText('80 Gbps USB4')).toBeInTheDocument()
    expect(screen.getByText('Sockets')).toBeInTheDocument()
    expect(screen.getByText('USB-C')).toBeInTheDocument()
    expect(screen.getByText('built in')).toBeInTheDocument()
    expect(container.querySelectorAll('.socket')).toHaveLength(4)
    expect(container.querySelectorAll('.socket .bolt')).toHaveLength(1)
  })

  it('ConnectionBanner offers a retry when degraded', async () => {
    const onRetry = vi.fn()
    render(ConnectionBanner, { state: 'offline', error: 'connection refused', apiBase: 'http://127.0.0.1:7911', onRetry })
    expect(screen.getByText('Cannot reach the Port Authority service')).toBeInTheDocument()
    expect(screen.getByText('Last error: connection refused')).toBeInTheDocument()
    await fireEvent.click(screen.getByText('Retry now'))
    expect(onRetry).toHaveBeenCalled()
  })

  it('Header shows the throughput hint when the capability is off', async () => {
    render(Header, {
      appName: 'Port Authority', version: '0.1.0', state: 'live', legendOpen: false, onToggleLegend: () => {},
      capabilities: { platform: 'windows', topology: true, hotplug: true, throughput: false, alt_mode: false, power_draw: true, iso_reservation: true, connector_info: true, string_descriptors: true },
    })
    expect(screen.getByText('v0.1.0')).toBeInTheDocument()
    expect(screen.getByText('Live')).toBeInTheDocument()
    await fireEvent.click(screen.getByText('No live throughput'))
    expect(screen.getByText(/Performance Log Users/)).toBeInTheDocument()
  })
})
