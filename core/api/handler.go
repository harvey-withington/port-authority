package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"portauthority/core/insight"
	"portauthority/core/model"
)

// Handler returns the /api/v1 router wrapped in CORS and request logging.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	route(mux, "/api/v1/health", s.handleHealth)
	route(mux, "/api/v1/capabilities", s.handleCapabilities)
	route(mux, "/api/v1/topology", s.handleTopology)
	route(mux, "/api/v1/insights", s.handleInsights)
	route(mux, "/api/v1/devices/{id}", s.handleDevice)
	route(mux, "/api/v1/throughput", s.handleThroughput)
	route(mux, "/api/v1/stream", s.handleStream)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "no such endpoint")
	})
	return s.logging(cors(mux))
}

// route registers a GET-only endpoint. The method-less pattern catches
// every other verb so that the 405 is JSON rather than the mux's plain
// text default.
func route(mux *http.ServeMux, pattern string, h http.HandlerFunc) {
	mux.HandleFunc("GET "+pattern, h)
	mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Allow", "GET, OPTIONS")
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	})
}

func (s *Service) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":             true,
		"schema_version": SchemaVersion,
	})
}

func (s *Service) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": SchemaVersion,
		"capabilities":   s.Capabilities(),
	})
}

func (s *Service) handleTopology(w http.ResponseWriter, r *http.Request) {
	a, ok := s.snapshotOrError(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": SchemaVersion,
		"captured_at":    a.Topology.CapturedAt,
		"topology":       a.Topology,
		"insights":       nonNil(a.Insights),
	})
}

func (s *Service) handleInsights(w http.ResponseWriter, r *http.Request) {
	a, ok := s.snapshotOrError(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": SchemaVersion,
		"captured_at":    a.Topology.CapturedAt,
		"insights":       nonNil(a.Insights),
	})
}

func (s *Service) handleDevice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing device id")
		return
	}
	a, ok := s.snapshotOrError(w, r)
	if !ok {
		return
	}
	dev := findDevice(a.Topology, id)
	if dev == nil {
		writeError(w, http.StatusNotFound, "no device with id "+id)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": SchemaVersion,
		"captured_at":    a.Topology.CapturedAt,
		"device":         dev,
		"insights":       insightsFor(a.Insights, dev.ID),
	})
}

// snapshotOrError fetches the current snapshot, writing a JSON error and
// returning false when it is unavailable.
func (s *Service) snapshotOrError(w http.ResponseWriter, r *http.Request) (*Annotated, bool) {
	a, err := s.Snapshot(r.Context())
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			status = http.StatusServiceUnavailable
		}
		writeError(w, status, "snapshot failed: "+err.Error())
		return nil, false
	}
	return a, true
}

// findDevice matches a device ID case-insensitively. IDs are PnP instance
// IDs on Windows, which are case-insensitive by convention.
func findDevice(t *model.Topology, id string) *model.Device {
	var found *model.Device
	t.Walk(func(_ *model.Controller, _ *model.Device, _ *model.Port, d *model.Device) {
		if found == nil && strings.EqualFold(d.ID, id) {
			found = d
		}
	})
	return found
}

// insightsFor filters insights to those that name the device.
func insightsFor(all []insight.Insight, id string) []insight.Insight {
	out := []insight.Insight{}
	for _, in := range all {
		for _, did := range in.DeviceIDs {
			if strings.EqualFold(did, id) {
				out = append(out, in)
				break
			}
		}
	}
	return out
}

func nonNil(in []insight.Insight) []insight.Insight {
	if in == nil {
		return []insight.Insight{}
	}
	return in
}

// writeJSON encodes v fully before writing so an encoding failure can
// still produce a proper error response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		writeError(w, http.StatusInternalServerError, "encode response: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	// Every endpoint reports the machine as it is right now, so a cached
	// copy is always wrong. Without this a browser is free to serve a
	// heuristically cached body and the UI stops seeing changes.
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

func writeError(w http.ResponseWriter, status int, msg string) {
	body, _ := json.Marshal(map[string]string{"error": msg})
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(append(body, '\n'))
}

// OriginAllowed reports whether a browser at this origin may read the API.
//
// A snapshot names every device attached to the machine, with serial
// numbers and PnP instance ids: a stable hardware fingerprint. Binding to
// loopback does not keep that away from the web, because any page the user
// happens to be visiting can ask a loopback address for it. So only the
// origins that are actually part of this app may read it:
//
//   - wails.localhost, which is where Wails serves the app window from
//   - loopback, which covers `wails dev`, `vite preview` and opening the
//     UI in a browser against a `pactl serve`
//
// Anything else gets no CORS header at all, so the browser refuses to hand
// over the response. A request with no Origin is not from a browser and is
// left alone; this is a defence against web pages, not against local
// programs, which need the token auth that is still to come.
func OriginAllowed(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	switch strings.ToLower(u.Hostname()) {
	case "wails.localhost", "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		// The response differs per origin, so it must never be reused for
		// another one.
		h.Add("Vary", "Origin")
		if origin := r.Header.Get("Origin"); origin != "" && OriginAllowed(origin) {
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			h.Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions {
			h.Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// statusWriter records the status code for the request log.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	if sw.status == 0 {
		sw.status = code
	}
	sw.ResponseWriter.WriteHeader(code)
}

func (sw *statusWriter) Write(b []byte) (int, error) {
	if sw.status == 0 {
		sw.status = http.StatusOK
	}
	return sw.ResponseWriter.Write(b)
}

// Unwrap exposes the underlying writer so http.ResponseController and the
// WebSocket upgrade can reach Hijack and Flush through the wrapper.
func (sw *statusWriter) Unwrap() http.ResponseWriter { return sw.ResponseWriter }

func (s *Service) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		if sw.status == 0 {
			sw.status = http.StatusOK
		}
		s.logger.Printf("%s %s %d %s", r.Method, r.URL.RequestURI(), sw.status, time.Since(start).Round(time.Microsecond))
	})
}
