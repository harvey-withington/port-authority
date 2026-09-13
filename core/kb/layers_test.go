package kb

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// resetLayers puts the knowledge base back to the shipped data alone.
func resetLayers(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		UseShared(nil)
		if err := UseLocalDir(""); err != nil {
			t.Errorf("reset local layer: %v", err)
		}
	})
}

func TestLocalDockIsAddedWrittenAndReloaded(t *testing.T) {
	resetLayers(t)
	dir := t.TempDir()
	if err := UseLocalDir(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := AddLocalDock(DockEntry{Name: "Acme Dock 9", Hubs: []string{"1234:0001", "1234:0002"}, USB4: &USB4Spec{Vendor: "Acme", Model: "Dock 9"}}); err != nil {
		t.Fatal(err)
	}
	d, ok := DockForHub(0x1234, 0x0002)
	if !ok || d.ID != "local:acme-dock-9" || d.Source != SourceLocal {
		t.Fatalf("hub lookup = %+v %v, want the local dock", d, ok)
	}
	if got, ok := DockForUSB4Strings("Acme Corp", "Dock 9"); !ok || got != d {
		t.Errorf("router strings did not resolve the local dock: %v %v", got, ok)
	}
	if _, err := os.Stat(filepath.Join(dir, LocalFile)); err != nil {
		t.Fatalf("local file not written: %v", err)
	}

	// A fresh load of the same directory sees the same dock.
	if err := UseLocalDir(""); err != nil {
		t.Fatal(err)
	}
	if _, ok := DockForHub(0x1234, 0x0002); ok {
		t.Fatal("local layer still visible after turning it off")
	}
	if err := UseLocalDir(dir); err != nil {
		t.Fatal(err)
	}
	if _, ok := DockByID("local:acme-dock-9"); !ok {
		t.Fatal("local dock not reloaded from disk")
	}
}

func TestLocalDockOverridesShippedAndCanBeForgotten(t *testing.T) {
	resetLayers(t)
	if err := UseLocalDir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	// The user says the TS4's USB 3 hub is really part of their own box.
	if _, err := AddLocalDock(DockEntry{Name: "My box", Hubs: []string{"2188:5500"}}); err != nil {
		t.Fatal(err)
	}
	if d, _ := DockForHub(0x2188, 0x5500); d.ID != "local:my-box" {
		t.Errorf("hub 2188:5500 resolves to %s, want the local override", d.ID)
	}
	if d, _ := DockForHub(0x2188, 0x5501); d.ID != "caldigit-ts4" {
		t.Errorf("hub 2188:5501 resolves to %s, want the shipped TS4 untouched", d.ID)
	}
	if err := RemoveLocalDock("caldigit-ts4"); err != ErrNotLocal {
		t.Errorf("removing a shipped dock: err = %v, want ErrNotLocal", err)
	}
	if err := RemoveLocalDock("local:my-box"); err != nil {
		t.Fatal(err)
	}
	if d, _ := DockForHub(0x2188, 0x5500); d.ID != "caldigit-ts4" {
		t.Errorf("after forgetting, hub resolves to %s, want the shipped TS4 back", d.ID)
	}
}

func TestFirstWriteBeforeAnyReadDoesNotDeadlock(t *testing.T) {
	// A fresh process that sets the local directory and adds a dock before
	// anything has looked a hub up: the catalog is built inside the write.
	resetLayers(t)
	current.Store(nil)
	done := make(chan error, 1)
	go func() {
		if err := UseLocalDir(t.TempDir()); err != nil {
			done <- err
			return
		}
		_, err := AddLocalDock(DockEntry{Name: "Fresh", Hubs: []string{"1234:0001"}})
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("AddLocalDock deadlocked on a cold catalog")
	}
	if d, ok := DockForHub(0x1234, 0x0001); !ok || d.Name != "Fresh" {
		t.Errorf("dock added on a cold catalog not found: %+v %v", d, ok)
	}
}

func TestLocalDockNeverClaimsAGenericChipsetHub(t *testing.T) {
	resetLayers(t)
	if err := UseLocalDir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	d, err := AddLocalDock(DockEntry{Name: "Acme", Hubs: []string{"8087:0b40", "1234:0001"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := d.Entry().Hubs; len(got) != 1 || got[0] != "1234:0001" {
		t.Errorf("hubs = %v, want the Goshen Ridge hub dropped", got)
	}
	if !IsGenericDockHub(0x8087, 0x0b40) {
		t.Error("the generic hub should stay generic")
	}
	if _, err := AddLocalDock(DockEntry{Name: "Only generic", Hubs: []string{"8087:0b40"}}); err == nil {
		t.Error("a dock made only of a generic hub was accepted")
	}
}

func TestLocalLayerRejectsBadEntriesAndNeedsADirectory(t *testing.T) {
	resetLayers(t)
	if _, err := AddLocalDock(DockEntry{Name: "x", Hubs: []string{"1:2"}}); err != ErrNoLocalLayer {
		t.Errorf("without a directory: err = %v, want ErrNoLocalLayer", err)
	}
	if err := UseLocalDir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []DockEntry{
		{Name: "", Hubs: []string{"1234:0001"}},
		{Name: "No hubs"},
		{Name: "Bad hub", Hubs: []string{"zz:0001"}},
		{ID: "caldigit-ts4", Name: "Not local id", Hubs: []string{"1234:0001"}},
	} {
		if _, err := AddLocalDock(bad); err == nil {
			t.Errorf("entry %+v was accepted", bad)
		}
	}
}

func TestSharedLayerSitsBetweenShippedAndLocal(t *testing.T) {
	resetLayers(t)
	UseShared([]DockEntry{{ID: "caldigit-ts4", Name: "CalDigit TS4 (community)", Hubs: []string{"2188:5500"}}})
	if d, _ := DockByID("caldigit-ts4"); d.Name != "CalDigit TS4 (community)" || d.Source != SourceShared {
		t.Errorf("shared entry did not replace the shipped one: %+v", d)
	}
	if err := UseLocalDir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if _, err := AddLocalDock(DockEntry{ID: "local:ts4", Name: "TS4 (mine)", Hubs: []string{"2188:5500"}}); err != nil {
		t.Fatal(err)
	}
	if d, _ := DockForHub(0x2188, 0x5500); d.Source != SourceLocal {
		t.Errorf("hub resolves to the %s layer, want local on top", d.Source)
	}
}
