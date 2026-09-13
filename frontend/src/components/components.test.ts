// Smoke tests: each shared component renders real data without runtime
// errors and shows the text a user relies on.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { tick } from 'svelte'
import { fireEvent, render, screen } from '@testing-library/svelte'
import TopologyTree from './TopologyTree.svelte'
import TopologyGraph from './TopologyGraph.svelte'
import ViewSwitch from './ViewSwitch.svelte'
import ZoomControl from './ZoomControl.svelte'
import InsightList from './InsightList.svelte'
import FindingsFilter from './FindingsFilter.svelte'
import DockSetupDialog from './DockSetupDialog.svelte'
import ConfirmDialog from './ConfirmDialog.svelte'
import type { Topology } from '../lib/api/types'
import { dockEditor } from '../lib/dockEditor.svelte'
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
import { layout } from '../lib/layout.svelte'
import { findingFilter } from '../lib/findingFilter.svelte'
import { focus } from '../lib/focus.svelte'
import { findings } from '../lib/findings.svelte'
import { insightKey } from '../lib/insights'
import { dockedTopologyWithUsb4, insight, sampleTopology } from '../lib/fixtures.test-helpers'
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
    expect(screen.getAllByTitle('Named in a warning. Show the findings for Corsair EX400U').length).toBeGreaterThan(0)
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
    expect(screen.getAllByTitle('Named in a warning. Show the findings for Corsair EX400U').length).toBeGreaterThan(0)
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

  it('badges a dock on a USB4 cable with the measured cable speed beside its USB tunnel', async () => {
    const docks = new Map([['caldigit-ts4', { id: 'caldigit-ts4', name: 'CalDigit TS4', source: 'shipped' as const, hubs: [], uplink: { kind: 'thunderbolt4', max_link: 'usb4_40' as const } }]])
    render(TopologyGraph, { topology: dockedTopologyWithUsb4(), ctx: { ...quiet, docks }, loading: false, detail: 'physical' })
    await tick()
    const badge = screen.getByTitle(/Connected over USB4/)
    expect(badge).toHaveTextContent('20 Gbps USB4')
    expect(badge.getAttribute('title')).toBe('Connected over USB4 / Thunderbolt at 20 Gbps USB4. The 10 Gbps badge is the USB tunnel inside that link, which every USB device in the dock shares. Slower than it could be')
    expect(badge.classList.contains('health-slow')).toBe(true)
    // The dock's own router is folded away; the SSD router still shows.
    expect(screen.queryByText('USB4 Router (1.0), CalDigit. Inc. - TS4')).not.toBeInTheDocument()
    expect(screen.getByText('Corsair EX400U')).toBeInTheDocument()
  })

  it('flags and focuses the dock when a finding names the router folded into it', async () => {
    const cableFinding = insight({ rule_id: 'usb4-link-below-max', title: 'Dock on a slow cable', device_ids: ['USB4\\TS4'] })
    const ctx: TreeContext = { ...quiet, flagged: flaggedDevices([cableFinding]) }
    const { container } = render(TopologyGraph, { topology: dockedTopologyWithUsb4(), ctx, loading: false, detail: 'physical' })
    await tick()
    const dock = container.querySelector('.node.kind-box.flagged')
    expect(dock?.getAttribute('data-node-id')).toBe('dock:caldigit-ts4:1')
    expect(screen.getByRole('button', { name: 'Named in a warning. Show the findings for CalDigit TS4' })).toBeInTheDocument()
    // "Show me" on the router lands on the dock, since the router is drawn as the dock.
    focus.request('USB4\\TS4')
    await tick()
    expect(dock?.classList.contains('flash')).toBe(true)
  })

  it('offers to share a dock the user set up, and tags a community dock', async () => {
    const open = vi.spyOn(window, 'open').mockImplementation(() => null)
    const t = dockedTopologyWithUsb4()
    const stamp = (source: 'local' | 'shared'): Topology =>
      JSON.parse(JSON.stringify(t), (key: string, value: unknown) => (key === 'enclosure' && value && typeof value === 'object' ? { ...(value as object), source } : value)) as Topology
    const docks = new Map([['caldigit-ts4', { id: 'caldigit-ts4', name: 'CalDigit TS4', source: 'local' as const, hubs: ['2188:5500'] }]])
    const { unmount } = render(TopologyGraph, { topology: stamp('local'), ctx: { ...quiet, docks }, loading: false, detail: 'physical' })
    await tick()
    expect(screen.getByText('your dock')).toBeInTheDocument()
    await fireEvent.click(screen.getByRole('button', { name: 'Share CalDigit TS4 with the community' }))
    expect(open).toHaveBeenCalledTimes(1)
    const url = new URL(String(open.mock.calls[0][0]))
    expect(url.pathname).toBe('/harvey-withington/usb-device-kb/issues/new')
    expect(url.searchParams.get('title')).toBe('Dock: CalDigit TS4')
    unmount()
    render(TopologyGraph, { topology: stamp('shared'), ctx: quiet, loading: false, detail: 'physical' })
    await tick()
    expect(screen.getByText('community dock')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Share/ })).not.toBeInTheDocument()
    open.mockRestore()
  })

  it('opens the findings panel narrowed to the device whose flag was clicked', async () => {
    findingFilter.clear()
    if (layout.panelsOpen) layout.togglePanels()
    render(TopologyGraph, { topology, ctx: busy, loading: false, detail: 'logical' })
    await tick()
    await fireEvent.click(screen.getByRole('button', { name: 'Named in a warning. Show the findings for Corsair EX400U' }))
    expect(layout.panelsOpen).toBe(true)
    expect(findingFilter.current?.name).toBe('Corsair EX400U')
    expect(findingFilter.current?.ids).toEqual(['USB\\VID_1B1C&PID_1A20\\SSD'])
    findingFilter.clear()
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
  const mouseFinding = insight({ rule_id: 'other', severity: 'info', title: 'Mouse is fine', device_ids: ['USB\\VID_046D&PID_C52B\\MOUSE'] })

  it('shows the empty state', () => {
    render(InsightList, { insights: [], index, filter: null, onFocusDevice: () => {}, onClearFilter: () => {} })
    expect(screen.getByText('Everything is connected at the speed it should be.')).toBeInTheDocument()
  })

  it('renders a card with details and resolves device names', async () => {
    const onFocus = vi.fn()
    render(InsightList, { insights: [{ ...finding, evidence: ['negotiated link ss10'] }], index, filter: null, onFocusDevice: onFocus, onClearFilter: () => {} })
    expect(screen.getByText('SSD could be faster')).toBeInTheDocument()
    await fireEvent.click(screen.getByText('Corsair EX400U'))
    expect(onFocus).toHaveBeenCalledWith('USB\\VID_1B1C&PID_1A20\\SSD')
    await fireEvent.click(screen.getByText('Details'))
    expect(screen.getByText('negotiated link ss10')).toBeInTheDocument()
    expect(screen.getByText('Confidence 90%')).toBeInTheDocument()
  })

  it('lists only the findings naming the filtered device, and flashes them', () => {
    const filter = { ids: ['usb\\vid_1b1c&pid_1a20\\ssd'], name: 'Corsair EX400U', token: 1 }
    const { container } = render(InsightList, { insights: [finding, mouseFinding], index, filter, onFocusDevice: () => {}, onClearFilter: () => {} })
    expect(screen.getByText('SSD could be faster')).toBeInTheDocument()
    expect(screen.queryByText('Mouse is fine')).not.toBeInTheDocument()
    expect(container.querySelector('.card.flash')).not.toBeNull()
  })

  it('unfolds a folded card when a flag points at it', async () => {
    const key = insightKey(finding)
    findings.toggle(key)
    expect(findings.isOpen(key)).toBe(false)
    const filter = { ids: ['USB\\VID_1B1C&PID_1A20\\SSD'], name: 'Corsair EX400U', token: 2 }
    const { container } = render(InsightList, { insights: [finding, mouseFinding], index, filter, onFocusDevice: () => {}, onClearFilter: () => {} })
    await tick()
    expect(findings.isOpen(key)).toBe(true)
    expect(container.querySelector('.card.folded')).toBeNull()
  })

  it('offers to show everything when nothing names the filtered device', async () => {
    const onClear = vi.fn()
    const filter = { ids: ['USB\\GONE'], name: 'Old hub', token: 1 }
    render(InsightList, { insights: [finding], index, filter, onFocusDevice: () => {}, onClearFilter: onClear })
    expect(screen.getByText('Nothing currently mentions Old hub.')).toBeInTheDocument()
    await fireEvent.click(screen.getByRole('button', { name: 'Show all findings' }))
    expect(onClear).toHaveBeenCalled()
  })
})

describe('DockSetupDialog', () => {
  /** The docked laptop with the knowledge base's groupings stripped: every hub loose. */
  const loose = (): Topology => JSON.parse(JSON.stringify(dockedTopologyWithUsb4()), (key: string, value: unknown) => (key === 'enclosure' || key === 'enclosure_id' ? undefined : value)) as Topology
  const finding = insight({ rule_id: 'dock-not-recognised', title: 'x looks like a dock', device_ids: ['USB\\TS4_USB3_A', 'USB\\TS4_USB3_B', 'USB4\\TS4'] })

  it('offers every loose hub, pre-ticks what the finding named, and saves a dock entry', async () => {
    const t = loose()
    const onSave = vi.fn(async () => {})
    const onClose = vi.fn()
    render(DockSetupDialog, { insight: finding, topology: t, index: indexTopology(t), onSave, onClose })
    const boxes = screen.getAllByRole('checkbox') as HTMLInputElement[]
    expect(boxes).toHaveLength(4)
    expect(boxes.filter((b) => b.checked)).toHaveLength(2)
    expect(screen.getByRole('textbox')).toHaveValue('CalDigit, Inc. TS4')
    const [routerSelect, uplinkSelect] = screen.getAllByRole('combobox')
    expect(routerSelect).toHaveValue('USB4\\TS4')
    await fireEvent.change(uplinkSelect, { target: { value: 'usb4_40' } })
    await fireEvent.click(screen.getByRole('button', { name: 'Save dock' }))
    expect(onSave).toHaveBeenCalledWith({
      name: 'CalDigit, Inc. TS4',
      hubs: ['0000:0000'],
      usb4: { vendor: 'CalDigit, Inc.', model: 'TS4' },
      uplink: { kind: 'usb4', max_link: 'usb4_40' },
    })
    expect(onClose).toHaveBeenCalled()
  })

  it('refuses an empty dock and shows why the service said no', async () => {
    const t = loose()
    const onSave = vi.fn(async () => {
      throw new Error('kb: a dock needs a name')
    })
    render(DockSetupDialog, { insight: finding, topology: t, index: indexTopology(t), onSave, onClose: () => {} })
    for (const box of screen.getAllByRole('checkbox')) {
      if ((box as HTMLInputElement).checked) await fireEvent.click(box)
    }
    await fireEvent.click(screen.getByRole('button', { name: 'Save dock' }))
    expect(screen.getByRole('alert')).toHaveTextContent('Give the dock a name and tick at least one hub.')
    expect(onSave).not.toHaveBeenCalled()
    await fireEvent.click(screen.getAllByRole('checkbox')[0])
    await fireEvent.click(screen.getByRole('button', { name: 'Save dock' }))
    expect(onSave).toHaveBeenCalled()
    expect(screen.getByRole('alert')).toHaveTextContent('kb: a dock needs a name')
  })
})

describe('ConfirmDialog', () => {
  it('confirms, cancels, and closes on Escape', async () => {
    const onConfirm = vi.fn()
    const onCancel = vi.fn()
    render(ConfirmDialog, { title: 'Forget it?', body: 'Gone.', confirmLabel: 'Forget', danger: true, onConfirm, onCancel })
    expect(screen.getByRole('dialog', { name: 'Forget it?' })).toBeInTheDocument()
    await fireEvent.click(screen.getByRole('button', { name: 'Forget' }))
    expect(onConfirm).toHaveBeenCalled()
    await fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(onCancel).toHaveBeenCalledTimes(1)
    await fireEvent.keyDown(window, { key: 'Escape' })
    expect(onCancel).toHaveBeenCalledTimes(2)
  })
})

describe('the dock finding', () => {
  afterEach(() => dockEditor.close())

  it('offers to set the dock up and opens the editor', async () => {
    const finding = insight({ rule_id: 'dock-not-recognised', title: 'Acme looks like a dock', device_ids: [] })
    render(InsightList, { insights: [finding], index, filter: null, onFocusDevice: () => {}, onClearFilter: () => {} })
    await fireEvent.click(screen.getByRole('button', { name: 'Set up this dock…' }))
    expect(dockEditor.state.kind).toBe('setup')
  })
})

describe('FindingsFilter', () => {
  afterEach(() => findingFilter.clear())

  it('renders nothing when no device is flagged', () => {
    const { container } = render(FindingsFilter, { flagged: new Map(), index })
    expect(container.querySelector('select')).toBeNull()
  })

  it('offers every flagged device, narrows to the chosen one and clears again', async () => {
    render(FindingsFilter, { flagged: flaggedDevices([finding]), index })
    const select = screen.getByLabelText('Showing')
    expect(screen.getByRole('option', { name: 'All findings' })).toBeInTheDocument()
    const option = screen.getByRole('option', { name: 'Corsair EX400U' })
    await fireEvent.change(select, { target: { value: option.getAttribute('value') } })
    expect(findingFilter.current?.name).toBe('Corsair EX400U')
    await fireEvent.click(screen.getByRole('button', { name: 'Show all findings' }))
    expect(findingFilter.current).toBeNull()
  })

  it('offers a box filter as its own entry when it covers several devices', () => {
    findingFilter.show(['USB\\A', 'USB\\B'], 'CalDigit TS4')
    render(FindingsFilter, { flagged: flaggedDevices([finding]), index })
    expect(screen.getByRole('option', { name: 'CalDigit TS4' })).toBeInTheDocument()
    expect((screen.getByLabelText('Showing') as HTMLSelectElement).selectedOptions[0].textContent).toBe('CalDigit TS4')
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
    // One row per finding severity, each wearing its glyph.
    expect(screen.getByText('Findings')).toBeInTheDocument()
    for (const label of ['Info', 'Warning', 'Critical']) expect(screen.getByText(label)).toBeInTheDocument()
    expect(container.querySelectorAll('.severity .glyph svg')).toHaveLength(3)
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
