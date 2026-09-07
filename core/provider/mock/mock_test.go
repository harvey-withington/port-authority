package mock

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"portauthority/core/model"
)

func sampleTopology() *model.Topology {
	ssd := &model.Device{
		ID: "dev-ssd", PortPath: "1/2/3", VendorID: 0x1b1c, ProductID: 0x1a20,
		Class: model.ClassStorage, ClaimedSpeed: model.LinkSuperPlus, PowerDrawMA: 896,
	}
	hub := &model.Device{
		ID: "dev-hub", PortPath: "1/2", Class: model.ClassHub, ClaimedSpeed: model.LinkSuperPlus,
		Hub: &model.Hub{Kind: model.HubUSB3, Depth: 1, PortCount: 4, Ports: []model.Port{
			{Number: 3, Status: model.StatusConnected, NegotiatedLink: model.LinkSuper, MaxLink: model.LinkSuperPlus, Device: ssd},
			{Number: 4, Status: model.StatusNoDevice, NegotiatedLink: model.LinkNone, MaxLink: model.LinkSuperPlus},
		}},
	}
	root := &model.Device{
		ID: "dev-root", PortPath: "1", Class: model.ClassHub,
		Hub: &model.Hub{Kind: model.HubRoot, PortCount: 2, Ports: []model.Port{
			{Number: 2, Status: model.StatusConnected, NegotiatedLink: model.LinkSuperPlus, MaxLink: model.LinkSuperPlus,
				Connector: &model.Connector{TypeC: true, UserConnectable: true}, Device: hub},
		}},
	}
	return &model.Topology{
		SchemaVersion: model.SchemaVersion,
		CapturedAt:    time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC),
		Platform:      "test",
		Controllers: []model.Controller{{
			ID: "ctl-1", Name: "Test xHCI", Kind: model.ControllerXHCI, MaxBandwidth: 10 * model.Gbps, RootHub: root,
		}},
	}
}

func TestSnapshotRoundTripsThroughJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.json")
	data, err := New(sampleTopology()).Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	p, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	var visited []string
	got.Walk(func(_ *model.Controller, _ *model.Device, port *model.Port, d *model.Device) {
		visited = append(visited, d.ID)
		if d.ID == "dev-ssd" {
			if port.NegotiatedLink != model.LinkSuper || port.MaxLink != model.LinkSuperPlus {
				t.Errorf("ssd port links = %s/%s, want ss5/ss10", port.NegotiatedLink, port.MaxLink)
			}
			if d.ClaimedSpeed != model.LinkSuperPlus {
				t.Errorf("ssd claimed = %s, want ss10", d.ClaimedSpeed)
			}
		}
	})
	want := []string{"dev-root", "dev-hub", "dev-ssd"}
	if len(visited) != len(want) {
		t.Fatalf("walk visited %v, want %v", visited, want)
	}
	for i := range want {
		if visited[i] != want[i] {
			t.Fatalf("walk visited %v, want %v", visited, want)
		}
	}
	if !got.Controllers[0].RootHub.Hub.Ports[0].Connector.TypeC {
		t.Error("connector type-c flag lost in round trip")
	}
}

func TestSnapshotReturnsIndependentCopies(t *testing.T) {
	p := New(sampleTopology())
	first, _ := p.Snapshot(context.Background())
	first.Controllers[0].Name = "mutated"
	second, _ := p.Snapshot(context.Background())
	if second.Controllers[0].Name != "Test xHCI" {
		t.Errorf("mutation leaked between snapshots: %q", second.Controllers[0].Name)
	}
}

func TestLinkSpeedJSONNames(t *testing.T) {
	for _, speed := range []model.LinkSpeed{model.LinkNone, model.LinkHigh, model.LinkSuperPlus, model.LinkUSB4Gen3} {
		raw, err := speed.MarshalJSON()
		if err != nil {
			t.Fatal(err)
		}
		var back model.LinkSpeed
		if err := back.UnmarshalJSON(raw); err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if back != speed {
			t.Errorf("%s round-tripped to %s", speed, back)
		}
	}
	var bad model.LinkSpeed
	if err := bad.UnmarshalJSON([]byte(`"warp9"`)); err == nil {
		t.Error("expected error for unknown speed name")
	}
}
