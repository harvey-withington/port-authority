package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"portauthority/core/kb"
)

func useTempKB(t *testing.T) {
	t.Helper()
	if err := kb.UseLocalDir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := kb.UseLocalDir(""); err != nil {
			t.Errorf("reset local layer: %v", err)
		}
	})
}

// do performs a request with a JSON body as the app window would.
func do(t *testing.T, srv *httptest.Server, method, path string, body any, wantStatus int, out any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req, err := http.NewRequest(method, srv.URL+path, &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", appOrigin)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		t.Fatalf("%s %s: status %d, want %d: %s", method, path, resp.StatusCode, wantStatus, raw)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			t.Fatalf("%s %s: decode: %v: %s", method, path, err, raw)
		}
	}
}

type docksResponse struct {
	Docks    []DockView `json:"docks"`
	Writable bool       `json:"writable"`
}

func TestKBDocksListsEveryLayer(t *testing.T) {
	useTempKB(t)
	srv := newFixtureServer(t)
	var res docksResponse
	get(t, srv, "/api/v1/kb/docks", http.StatusOK, &res)
	if !res.Writable {
		t.Error("writable = false with a local directory set")
	}
	found := false
	for _, d := range res.Docks {
		if d.ID == "caldigit-ts4" && d.Source == kb.SourceShipped && len(d.Hubs) > 0 {
			found = true
		}
	}
	if !found {
		t.Errorf("shipped TS4 missing from %+v", res.Docks)
	}
}

func TestKBDocksAddsAndForgetsALocalDock(t *testing.T) {
	useTempKB(t)
	srv := newFixtureServer(t)
	var created struct {
		Dock DockView `json:"dock"`
	}
	do(t, srv, http.MethodPost, "/api/v1/kb/docks", map[string]any{
		"name": "Acme Dock 9", "hubs": []string{"1234:0001"}, "usb4": map[string]string{"vendor": "Acme", "model": "Dock 9"},
	}, http.StatusCreated, &created)
	if created.Dock.ID != "local:acme-dock-9" || created.Dock.Source != kb.SourceLocal {
		t.Fatalf("created = %+v", created.Dock)
	}
	var res docksResponse
	get(t, srv, "/api/v1/kb/docks", http.StatusOK, &res)
	if len(res.Docks) == 0 || res.Docks[len(res.Docks)-1].ID != "local:acme-dock-9" {
		t.Errorf("local dock not listed last: %+v", res.Docks)
	}
	do(t, srv, http.MethodDelete, "/api/v1/kb/docks/local:acme-dock-9", nil, http.StatusOK, nil)
	do(t, srv, http.MethodDelete, "/api/v1/kb/docks/local:acme-dock-9", nil, http.StatusNotFound, nil)
	do(t, srv, http.MethodDelete, "/api/v1/kb/docks/caldigit-ts4", nil, http.StatusNotFound, nil)
}

func TestKBDocksRejectsBadInputAndReadOnlyRuns(t *testing.T) {
	useTempKB(t)
	srv := newFixtureServer(t)
	do(t, srv, http.MethodPost, "/api/v1/kb/docks", map[string]any{"name": "", "hubs": []string{"1234:0001"}}, http.StatusBadRequest, nil)
	do(t, srv, http.MethodPost, "/api/v1/kb/docks", map[string]any{"name": "x", "hubs": []string{"1234:0001"}, "bogus": 1}, http.StatusBadRequest, nil)
	do(t, srv, http.MethodPut, "/api/v1/kb/docks", nil, http.StatusMethodNotAllowed, nil)

	if err := kb.UseLocalDir(""); err != nil {
		t.Fatal(err)
	}
	do(t, srv, http.MethodPost, "/api/v1/kb/docks", map[string]any{"name": "x", "hubs": []string{"1234:0001"}}, http.StatusServiceUnavailable, nil)
	var res docksResponse
	get(t, srv, "/api/v1/kb/docks", http.StatusOK, &res)
	if res.Writable {
		t.Error("writable = true without a local directory")
	}
}
