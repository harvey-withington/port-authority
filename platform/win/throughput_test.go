//go:build windows

package win

import (
	"testing"
	"time"

	"portauthority/core/model"
)

// urbStop builds a UCX bulk-transfer Stop event the way TDH decodes it:
// a device pointer, a success status, and a per-function struct holding
// the transfer length and flags.
func urbStop(devPtr uint64, length, flags uint64) *ETWEvent {
	return &ETWEvent{
		Provider:   "UCX",
		OpcodeName: "Stop",
		Properties: []ETWProperty{
			{Name: "fid_UcxController", Value: uint64(0xabc)},
			{Name: "fid_UsbDevice", Value: devPtr},
			{Name: "fid_UCX_URB_BULK_OR_INTERRUPT_TRANSFER", Value: []ETWProperty{
				{Name: "fid_URB_Hdr_Length", Value: uint64(128)},
				{Name: "fid_URB_TransferFlags", Value: flags},
				{Name: "fid_URB_TransferBufferLength", Value: length},
			}},
			{Name: "fid_IRP_NtStatus", Value: uint64(0)},
		},
	}
}

func rundown(devPtr uint64, path string) *ETWEvent {
	return &ETWEvent{
		Provider:   "USBHUB3",
		OpcodeName: "Information",
		Properties: []ETWProperty{
			{Name: "fid_UsbDevice", Value: devPtr},
			{Name: "fid_DeviceInterfacePath", Value: path},
		},
	}
}

func sampleByID(samples []model.ThroughputSample, id string) (model.ThroughputSample, bool) {
	for _, s := range samples {
		if s.DeviceID == id {
			return s, true
		}
	}
	return model.ThroughputSample{}, false
}

func TestThroughputCountsAndAttributes(t *testing.T) {
	m := newThroughputMonitor()
	const dev = uint64(0x6d72a74143b8)
	const id = `USB\VID_1B1C&PID_1A20\MSFT30SCRUBBED-06`

	// The rundown maps the kernel pointer to the PnP instance ID.
	m.handle(rundown(dev, `\??\USB#VID_1B1C&PID_1A20#MSFT30SCRUBBED-06#{a5dcbf10-6530-11d2-901f-00c04fb951ed}`))

	// Two reads (flags bit 0 set) and one write in this interval.
	m.handle(urbStop(dev, 262144, 1))
	m.handle(urbStop(dev, 262144, 1))
	m.handle(urbStop(dev, 65536, 0))

	// A failed transfer must not be counted.
	failed := urbStop(dev, 1<<20, 1)
	failed.Properties[3].Value = uint64(0xC0000001)
	m.handle(failed)

	samples := m.tick(time.Now())
	s, ok := sampleByID(samples, id)
	if !ok {
		t.Fatalf("no sample for %s; got %+v", id, samples)
	}
	if wantRead := model.Bitrate(2 * 262144 * 8); s.ReadBps != wantRead {
		t.Errorf("read = %d, want %d", s.ReadBps, wantRead)
	}
	if wantWrite := model.Bitrate(65536 * 8); s.WriteBps != wantWrite {
		t.Errorf("write = %d, want %d", s.WriteBps, wantWrite)
	}

	// The counter resets each interval; an idle interval yields one zero
	// sample so consumers can decay the reading, then silence.
	idle := m.tick(time.Now())
	if s, ok := sampleByID(idle, id); !ok || s.ReadBps != 0 || s.WriteBps != 0 {
		t.Errorf("expected a single zero decay sample, got %+v", idle)
	}
	if next := m.tick(time.Now()); len(next) != 0 {
		t.Errorf("expected silence after decay, got %+v", next)
	}
}

func TestThroughputUnmappedPointerFallsBackToPointerKey(t *testing.T) {
	m := newThroughputMonitor()
	m.handle(urbStop(0xdeadbeef, 1000, 1))
	samples := m.tick(time.Now())
	if _, ok := sampleByID(samples, "ptr:0xdeadbeef"); !ok {
		t.Fatalf("expected pointer-keyed sample, got %+v", samples)
	}
}

func TestThroughputIgnoresStartAndNonURB(t *testing.T) {
	m := newThroughputMonitor()
	start := urbStop(0x1, 5000, 1)
	start.OpcodeName = "Start"
	m.handle(start)
	m.handle(&ETWEvent{Provider: "UCX", OpcodeName: "Stop", Properties: []ETWProperty{{Name: "fid_UsbDevice", Value: uint64(0x1)}}})
	if s := m.tick(time.Now()); len(s) != 0 {
		t.Fatalf("start events and length-less stops must not count: %+v", s)
	}
}
