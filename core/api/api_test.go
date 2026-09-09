package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"portauthority/core/insight"
	"portauthority/core/model"
	"portauthority/core/provider"
	"portauthority/core/provider/mock"
)

const (
	fixturePath = "../../testdata/fixtures/deviant-caldigit-ts4.json"
	ex400uID    = `USB\VID_1B1C&PID_1A20\MSFT30SCRUBBED-06`
	ex400uRule  = "faster-port-available"
)

// appOrigin is where Wails serves the app window from.
const appOrigin = "http://wails.localhost"

func newFixtureServer(t *testing.T, opts ...Option) *httptest.Server {
	t.Helper()
	p, err := mock.Load(fixturePath)
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	srv := httptest.NewServer(NewService(p, opts...).Handler())
	t.Cleanup(srv.Close)
	return srv
}

// get performs a GET, checks the content type and status, and decodes the
// body into out.
func get(t *testing.T, srv *httptest.Server, path string, wantStatus int, out any) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	// Ask as the app's own window does, so every endpoint is covered by
	// the origin allow-list and not just the CORS tests.
	req.Header.Set("Origin", appOrigin)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		t.Fatalf("GET %s: status %d, want %d; body %s", path, resp.StatusCode, wantStatus, body)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("GET %s: Content-Type %q, want application/json", path, ct)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != appOrigin {
		t.Errorf("GET %s: Access-Control-Allow-Origin %q, want %q", path, got, appOrigin)
	}
	if out != nil {
		if err := json.Unmarshal(body, out); err != nil {
			t.Fatalf("GET %s: decode: %v; body %s", path, err, body)
		}
	}
}

func hasRule(ins []insight.Insight, rule string) bool {
	for _, in := range ins {
		if in.RuleID == rule {
			return true
		}
	}
	return false
}

func TestHealth(t *testing.T) {
	srv := newFixtureServer(t)
	var got struct {
		OK            bool `json:"ok"`
		SchemaVersion int  `json:"schema_version"`
	}
	get(t, srv, "/api/v1/health", http.StatusOK, &got)
	if !got.OK || got.SchemaVersion != 1 {
		t.Errorf("health = %+v, want ok and schema_version 1", got)
	}
}

func TestCapabilities(t *testing.T) {
	srv := newFixtureServer(t)
	var got struct {
		SchemaVersion int                `json:"schema_version"`
		Capabilities  model.ProviderCaps `json:"capabilities"`
	}
	get(t, srv, "/api/v1/capabilities", http.StatusOK, &got)
	if got.SchemaVersion != 1 || got.Capabilities.Platform != "mock" || !got.Capabilities.Topology {
		t.Errorf("capabilities = %+v", got)
	}
}

func TestTopology(t *testing.T) {
	srv := newFixtureServer(t)
	var got struct {
		SchemaVersion int               `json:"schema_version"`
		CapturedAt    time.Time         `json:"captured_at"`
		Topology      model.Topology    `json:"topology"`
		Insights      []insight.Insight `json:"insights"`
	}
	get(t, srv, "/api/v1/topology", http.StatusOK, &got)
	if got.SchemaVersion != 1 {
		t.Errorf("schema_version = %d, want 1", got.SchemaVersion)
	}
	if got.CapturedAt.IsZero() {
		t.Error("captured_at is zero")
	}
	if len(got.Topology.Controllers) == 0 {
		t.Error("topology has no controllers")
	}
	if !hasRule(got.Insights, ex400uRule) {
		t.Errorf("topology insights lack %s: %+v", ex400uRule, got.Insights)
	}
	// Enrichment ran: the EX400U has a vendor name from the knowledge base.
	if d := findDevice(&got.Topology, ex400uID); d == nil || d.VendorName == "" {
		t.Errorf("EX400U not enriched: %+v", d)
	}
}

func TestInsights(t *testing.T) {
	srv := newFixtureServer(t)
	var got struct {
		SchemaVersion int               `json:"schema_version"`
		CapturedAt    time.Time         `json:"captured_at"`
		Insights      []insight.Insight `json:"insights"`
	}
	get(t, srv, "/api/v1/insights", http.StatusOK, &got)
	if got.SchemaVersion != 1 || got.CapturedAt.IsZero() {
		t.Errorf("schema_version=%d captured_at=%v", got.SchemaVersion, got.CapturedAt)
	}
	if !hasRule(got.Insights, ex400uRule) {
		t.Errorf("insights lack %s: %+v", ex400uRule, got.Insights)
	}
}

func TestDeviceByID(t *testing.T) {
	srv := newFixtureServer(t)
	var got struct {
		SchemaVersion int               `json:"schema_version"`
		Device        model.Device      `json:"device"`
		Insights      []insight.Insight `json:"insights"`
	}
	path := "/api/v1/devices/" + url.PathEscape(ex400uID)
	if !strings.Contains(path, "%5C") {
		t.Fatalf("expected backslashes to be escaped in %s", path)
	}
	get(t, srv, path, http.StatusOK, &got)
	if got.SchemaVersion != 1 {
		t.Errorf("schema_version = %d", got.SchemaVersion)
	}
	if got.Device.ID != ex400uID || got.Device.VendorID != 0x1B1C || got.Device.ProductID != 0x1A20 {
		t.Errorf("device = %s %04x:%04x, want EX400U", got.Device.ID, got.Device.VendorID, got.Device.ProductID)
	}
	if len(got.Insights) != 1 || got.Insights[0].RuleID != ex400uRule {
		t.Errorf("device insights = %+v, want exactly one %s", got.Insights, ex400uRule)
	}

	// Case-insensitive match.
	var lower struct {
		Device model.Device `json:"device"`
	}
	get(t, srv, "/api/v1/devices/"+url.PathEscape(strings.ToLower(ex400uID)), http.StatusOK, &lower)
	if lower.Device.ID != ex400uID {
		t.Errorf("lower-case lookup returned %q", lower.Device.ID)
	}
}

func TestDeviceNotFound(t *testing.T) {
	srv := newFixtureServer(t)
	var got struct {
		Error string `json:"error"`
	}
	get(t, srv, "/api/v1/devices/"+url.PathEscape(`USB\VID_DEAD&PID_BEEF\nope`), http.StatusNotFound, &got)
	if got.Error == "" {
		t.Error("404 body has no error field")
	}
}

func TestUnknownEndpointAndMethod(t *testing.T) {
	srv := newFixtureServer(t)
	var got struct {
		Error string `json:"error"`
	}
	get(t, srv, "/api/v1/nothing", http.StatusNotFound, &got)
	if got.Error == "" {
		t.Error("404 body has no error field")
	}

	resp, err := http.Post(srv.URL+"/api/v1/topology", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST status = %d, want 405", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("POST Content-Type = %q", ct)
	}
}

func TestOptionsPreflight(t *testing.T) {
	srv := newFixtureServer(t)
	req, _ := http.NewRequest(http.MethodOptions, srv.URL+"/api/v1/topology", nil)
	req.Header.Set("Origin", appOrigin)
	req.Header.Set("Access-Control-Request-Method", "GET")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("OPTIONS status = %d, want 204", resp.StatusCode)
	}
	if resp.Header.Get("Access-Control-Allow-Origin") != appOrigin {
		t.Error("preflight lacks Access-Control-Allow-Origin")
	}
	if !strings.Contains(resp.Header.Get("Access-Control-Allow-Methods"), "GET") {
		t.Error("preflight lacks Access-Control-Allow-Methods GET")
	}

	// A preflight from anywhere else is answered, but without the headers
	// that would let the browser follow through.
	req, _ = http.NewRequest(http.MethodOptions, srv.URL+"/api/v1/topology", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	denied, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer denied.Body.Close()
	if got := denied.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("preflight from a foreign origin returned Access-Control-Allow-Origin %q, want none", got)
	}
}

// countingProvider wraps another provider and counts Snapshot calls. An
// optional gate blocks every Snapshot until released, to test in-flight
// sharing.
type countingProvider struct {
	provider.Provider
	calls atomic.Int32
	gate  chan struct{}
}

func (c *countingProvider) Snapshot(ctx context.Context) (*model.Topology, error) {
	c.calls.Add(1)
	if c.gate != nil {
		<-c.gate
	}
	return c.Provider.Snapshot(ctx)
}

func TestCacheTTL(t *testing.T) {
	inner, err := mock.Load(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	cp := &countingProvider{Provider: inner}
	srv := httptest.NewServer(NewService(cp, WithCacheTTL(time.Hour)).Handler())
	defer srv.Close()

	get(t, srv, "/api/v1/topology", http.StatusOK, nil)
	get(t, srv, "/api/v1/insights", http.StatusOK, nil)
	if n := cp.calls.Load(); n != 1 {
		t.Errorf("provider called %d times for two rapid requests, want 1", n)
	}

	// A zero TTL disables caching.
	cp2 := &countingProvider{Provider: inner}
	svc := NewService(cp2, WithCacheTTL(0))
	for i := 0; i < 2; i++ {
		if _, err := svc.Snapshot(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if n := cp2.calls.Load(); n != 2 {
		t.Errorf("provider called %d times with TTL 0, want 2", n)
	}
}

func TestConcurrentSnapshotShared(t *testing.T) {
	inner, err := mock.Load(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	cp := &countingProvider{Provider: inner, gate: make(chan struct{})}
	svc := NewService(cp, WithCacheTTL(0))

	const n = 8
	var wg sync.WaitGroup
	results := make([]*Annotated, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			a, err := svc.Snapshot(context.Background())
			if err != nil {
				t.Errorf("snapshot %d: %v", i, err)
			}
			results[i] = a
		}(i)
	}
	// Wait for the provider to be entered once, then release everyone.
	deadline := time.Now().Add(2 * time.Second)
	for cp.calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond) // let the other callers queue on the in-flight call
	close(cp.gate)
	wg.Wait()

	if got := cp.calls.Load(); got != 1 {
		t.Errorf("provider called %d times for %d concurrent callers, want 1", got, n)
	}
	for i := 1; i < n; i++ {
		if results[i] != results[0] {
			t.Errorf("caller %d got a different Annotated than caller 0", i)
		}
	}
}

func TestSnapshotError(t *testing.T) {
	failing := &errProvider{err: errors.New("boom")}
	srv := httptest.NewServer(NewService(failing).Handler())
	defer srv.Close()
	var got struct {
		Error string `json:"error"`
	}
	get(t, srv, "/api/v1/topology", http.StatusBadGateway, &got)
	if !strings.Contains(got.Error, "boom") {
		t.Errorf("error = %q", got.Error)
	}
}

type errProvider struct {
	provider.Provider
	err error
}

func (e *errProvider) Snapshot(context.Context) (*model.Topology, error) { return nil, e.err }
func (e *errProvider) Capabilities() model.ProviderCaps                  { return model.ProviderCaps{Platform: "err"} }

func TestListenAndServeRejectsNonLoopback(t *testing.T) {
	for _, addr := range []string{"0.0.0.0:7911", ":7911", "192.168.1.10:7911", "[::]:7911"} {
		err := ListenAndServe(context.Background(), addr, http.NotFoundHandler())
		if err == nil {
			t.Errorf("ListenAndServe(%q) accepted a non-loopback address", addr)
			continue
		}
		if addr != ":7911" && !errors.Is(err, ErrNotLoopback) {
			t.Errorf("ListenAndServe(%q) = %v, want ErrNotLoopback", addr, err)
		}
	}
}

func TestListenAndServeShutsDownOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- ListenAndServe(ctx, "127.0.0.1:0", http.NotFoundHandler()) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("ListenAndServe returned %v after cancel, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ListenAndServe did not return after ctx cancel")
	}
}

// A snapshot is a hardware fingerprint: every attached device, its serial
// number and its PnP instance id. Binding to loopback does not keep that
// away from the web, because any page the user is visiting can ask a
// loopback address for it. Only this app's own origins may read it.
func TestCORSOnlyAnswersThisAppsOrigins(t *testing.T) {
	srv := newFixtureServer(t)

	allowed := []string{
		"http://wails.localhost",
		"http://localhost:4173",
		"http://127.0.0.1:7911",
		"https://localhost:5173",
	}
	for _, origin := range allowed {
		res := getWithOrigin(t, srv.URL+"/api/v1/topology", origin)
		if got := res.Header.Get("Access-Control-Allow-Origin"); got != origin {
			t.Errorf("origin %s: Access-Control-Allow-Origin = %q, want %q", origin, got, origin)
		}
		res.Body.Close()
	}

	// Any website the user happens to have open, including ones that only
	// look like the real thing.
	denied := []string{
		"https://evil.example.com",
		"http://wails.localhost.evil.com",
		"http://notlocalhost",
		"http://127.0.0.1.evil.com",
		"file://",
		"null",
	}
	for _, origin := range denied {
		res := getWithOrigin(t, srv.URL+"/api/v1/topology", origin)
		if got := res.Header.Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("origin %s: Access-Control-Allow-Origin = %q, want none", origin, got)
		}
		res.Body.Close()
	}
}

// A wildcard would let one cached response be replayed to another origin.
func TestCORSVariesOnOrigin(t *testing.T) {
	srv := newFixtureServer(t)

	res := getWithOrigin(t, srv.URL+"/api/v1/topology", "http://wails.localhost")
	defer res.Body.Close()
	if vary := res.Header.Get("Vary"); !strings.Contains(vary, "Origin") {
		t.Errorf("Vary = %q, want it to include Origin", vary)
	}
}

// Anything that is not a browser sends no Origin and must keep working:
// this is a defence against web pages, not against local tools.
func TestRequestWithNoOriginStillWorks(t *testing.T) {
	srv := newFixtureServer(t)

	res, err := http.Get(srv.URL + "/api/v1/topology")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}
	if got := res.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want none when no Origin was sent", got)
	}
}

func TestOriginAllowed(t *testing.T) {
	for _, c := range []struct {
		origin string
		want   bool
	}{
		{"http://wails.localhost", true},
		{"http://localhost:5173", true},
		{"http://127.0.0.1:7911", true},
		{"http://[::1]:7911", true},
		{"https://evil.example.com", false},
		{"http://wails.localhost.evil.com", false},
		{"http://localhost.evil.com", false},
		{"", false},
		{"null", false},
		{"file:///etc/passwd", false},
		{"ftp://localhost", false},
	} {
		if got := OriginAllowed(c.origin); got != c.want {
			t.Errorf("OriginAllowed(%q) = %v, want %v", c.origin, got, c.want)
		}
	}
}

func getWithOrigin(t *testing.T, url, origin string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", origin)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}
