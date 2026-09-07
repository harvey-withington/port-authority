// Smoke tests: each shared component renders real data without runtime
// errors and shows the text a user relies on.
import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/svelte'
import TopologyTree from './TopologyTree.svelte'
import InsightList from './InsightList.svelte'
import Timeline from './Timeline.svelte'
import LinkBadge from './LinkBadge.svelte'
import ThroughputMeter from './ThroughputMeter.svelte'
import Legend from './Legend.svelte'
import ConnectionBanner from './ConnectionBanner.svelte'
import Header from './Header.svelte'
import { indexTopology } from '../lib/topology'
import { applySample, emptyThroughput } from '../lib/throughput'
import { flaggedDevices } from '../lib/insights'
import { insight, sampleTopology } from '../lib/fixtures.test-helpers'
import type { TreeContext } from '../lib/tree'

const topology = sampleTopology()
const index = indexTopology(topology)
const finding = insight()

describe('TopologyTree', () => {
  it('renders controllers, devices, link badges, meters and warnings', () => {
    const ctx: TreeContext = {
      flagged: flaggedDevices([finding]),
      throughput: applySample(emptyThroughput(), { device_id: 'USB\\VID_1B1C&PID_1A20\\SSD', at: '', read_bps: 800_000_000, write_bps: 0 }, 0),
      showMeter: true,
    }
    render(TopologyTree, { topology: { ...topology, warnings: ['hub 3 would not open'] }, ctx, loading: false })
    expect(screen.getByText('Test xHCI')).toBeInTheDocument()
    expect(screen.getByText('Corsair EX400U')).toBeInTheDocument()
    expect(screen.getByText('USB Input Device')).toBeInTheDocument()
    expect(screen.getByText('hub 3 would not open')).toBeInTheDocument()
    expect(screen.getByText('100.0 MB/s')).toBeInTheDocument()
    expect(screen.getAllByTitle('Mentioned in a finding').length).toBeGreaterThan(0)
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

  it('Legend lists classes and the speed ramp', () => {
    render(Legend, { onClose: () => {} })
    expect(screen.getByText('Storage')).toBeInTheDocument()
    expect(screen.getByText('80 Gbps USB4')).toBeInTheDocument()
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
