package community

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"portauthority/core/kb"
)

const docksBody = `{"docks":[{"id":"acme-dock-9","name":"Acme Dock 9","hubs":["1234:0001"],"usb4":{"vendor":"Acme","model":"Dock 9"}}]}`
const devicesBody = `{"devices":{"1234:9999":{"name":"Acme Drive","kind":"storage","max_link":"usb4_40","usb4_id":"1234:9998"}}}`

// release stands in for the GitHub release: it honours If-None-Match.
// The version goes into the tags, so two fake releases never look alike.
func release(t *testing.T, version, docks, devices string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		body, etag := "", ""
		switch r.URL.Path {
		case "/docks.json":
			body, etag = docks, `"docks-`+version+`"`
		case "/devices.json":
			body, etag = devices, `"devices-`+version+`"`
		default:
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", etag)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func resetLayers(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		_ = kb.UseShared(nil)
		_ = kb.UseSharedDevices(nil)
	})
}

func TestRefreshDownloadsAppliesCachesAndThenUsesETags(t *testing.T) {
	resetLayers(t)
	srv, hits := release(t, "v1", docksBody, devicesBody)
	dir := t.TempDir()
	f := New(dir, WithBaseURL(srv.URL+"/"))

	changed, err := f.Refresh(context.Background())
	if err != nil || !changed {
		t.Fatalf("first refresh: changed=%v err=%v", changed, err)
	}
	if d, ok := kb.DockByID("acme-dock-9"); !ok || d.Source != kb.SourceShared {
		t.Errorf("shared dock not applied: %+v %v", d, ok)
	}
	if dev, ok := kb.KnownDeviceInfo(0x1234, 0x9999); !ok || dev.Name != "Acme Drive" {
		t.Errorf("shared device not applied: %+v %v", dev, ok)
	}
	if _, ok := kb.KnownDeviceByUSB4ID(0x1234, 0x9998); !ok {
		t.Error("shared device's usb4 id not indexed")
	}
	for _, name := range files {
		if _, err := os.Stat(filepath.Join(dir, cacheSubdir, name)); err != nil {
			t.Errorf("%s not cached: %v", name, err)
		}
	}

	changed, err = f.Refresh(context.Background())
	if err != nil || changed {
		t.Errorf("second refresh: changed=%v err=%v, want a 304 for both", changed, err)
	}
	if hits.Load() != 4 {
		t.Errorf("server hits = %d, want 4 (two files, twice)", hits.Load())
	}

	// A fresh fetcher on the same directory starts from the cache and
	// remembers the tags.
	_ = kb.UseShared(nil)
	g := New(dir, WithBaseURL(srv.URL+"/"))
	if err := g.LoadCache(); err != nil {
		t.Fatal(err)
	}
	if _, ok := kb.DockByID("acme-dock-9"); !ok {
		t.Error("cache not applied by a new fetcher")
	}
	if g.Status().Files["docks.json"].ETag != `"docks-v1"` {
		t.Errorf("etag not remembered: %+v", g.Status())
	}
}

func TestBadFilesAreRefusedAndTheOldLayerStays(t *testing.T) {
	resetLayers(t)
	good, _ := release(t, "v1", docksBody, devicesBody)
	dir := t.TempDir()
	if _, err := New(dir, WithBaseURL(good.URL+"/")).Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}

	bad, _ := release(t, "v2", `{"docks":[{"id":"x","name":"x","hubs":["zz"]}],"surprise":1}`, `not json`)
	_, err := New(dir, WithBaseURL(bad.URL+"/")).Refresh(context.Background())
	if err == nil || !strings.Contains(err.Error(), "docks.json") || !strings.Contains(err.Error(), "devices.json") {
		t.Fatalf("err = %v, want both files reported", err)
	}
	if _, ok := kb.DockByID("acme-dock-9"); !ok {
		t.Error("the previous shared layer was lost")
	}
	raw, _ := os.ReadFile(filepath.Join(dir, cacheSubdir, "docks.json"))
	if string(raw) != docksBody {
		t.Error("the cache was overwritten with a refused file")
	}
}

func TestPartlyBadLayerKeepsTheGoodEntries(t *testing.T) {
	resetLayers(t)
	srv, _ := release(t, "v1", `{"docks":[{"id":"ok","name":"Ok","hubs":["1234:0001"]},{"id":"broken","name":"Broken","hubs":["nope"]}]}`, devicesBody)
	_, err := New(t.TempDir(), WithBaseURL(srv.URL+"/")).Refresh(context.Background())
	if err == nil || !strings.Contains(err.Error(), "broken") {
		t.Fatalf("err = %v, want the broken entry named", err)
	}
	if _, ok := kb.DockByID("ok"); !ok {
		t.Error("the good entry should still apply")
	}
}
