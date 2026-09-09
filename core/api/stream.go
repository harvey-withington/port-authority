package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/coder/websocket"
)

// writeTimeout bounds one WebSocket write so a stalled client cannot pin
// the handler.
const writeTimeout = 10 * time.Second

func (s *Service) handleThroughput(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": SchemaVersion,
		"window_seconds": int(sampleMaxAge / time.Second),
		"samples":        s.RecentSamples(),
	})
}

// handleStream upgrades to a WebSocket and pushes events until the client
// goes away or the server's context ends. Origin checks are skipped
// because the server only ever binds to loopback.
// allowedOriginPatterns is OriginAllowed expressed as host patterns, which
// is the form the websocket handshake check takes. Matched against the
// Origin's host, so the port wildcards cover the dev servers.
var allowedOriginPatterns = []string{
	"wails.localhost", "wails.localhost:*",
	"localhost", "localhost:*",
	"127.0.0.1", "127.0.0.1:*",
	"[::1]", "[::1]:*",
}

func (s *Service) handleStream(w http.ResponseWriter, r *http.Request) {
	types := parseTypes(r.URL.Query().Get("types"))
	// WebSockets are not covered by CORS, so the handshake's Origin check
	// is the only thing standing between a web page and the live stream.
	// Skipping it would hand every event to any site the user is visiting;
	// the patterns mirror OriginAllowed. A request with no Origin is not
	// from a browser and is still accepted.
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: allowedOriginPatterns})
	if err != nil {
		// Accept has already written an HTTP error.
		s.logger.Printf("stream: accept: %v", err)
		return
	}
	defer c.CloseNow()
	// The 101 went straight to the hijacked connection; tell the request
	// log so the upgrade is not recorded as a 200.
	if sw, ok := w.(*statusWriter); ok {
		sw.status = http.StatusSwitchingProtocols
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Read and discard whatever the client sends so that pings and close
	// frames are serviced; a read error means the client is gone. The
	// loop deliberately does not use ctx: a cancelled read tears the
	// socket down immediately, which would pre-empt the clean close
	// below. The deferred CloseNow ends it instead.
	go func() {
		defer cancel()
		for {
			if _, _, err := c.Read(context.Background()); err != nil {
				return
			}
		}
	}()

	sub := s.hub.subscribe(types)
	defer s.hub.unsubscribe(sub)

	send := func(e Event) error {
		body, err := json.Marshal(e)
		if err != nil {
			return err
		}
		wctx, wcancel := context.WithTimeout(ctx, writeTimeout)
		defer wcancel()
		return c.Write(wctx, websocket.MessageText, body)
	}

	if err := send(s.helloEvent(ctx, types)); err != nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			// Either the client closed (read loop cancelled) or the server
			// is shutting down. Both get a clean close; errors are
			// expected when the peer is already gone.
			_ = c.Close(websocket.StatusNormalClosure, "bye")
			return
		case e := <-sub.ch:
			if err := send(e); err != nil {
				if !errors.Is(err, context.Canceled) {
					s.logger.Printf("stream: write: %v", err)
				}
				return
			}
		}
	}
}

// helloEvent builds the greeting sent on connect.
func (s *Service) helloEvent(ctx context.Context, types []string) Event {
	h := Hello{
		SchemaVersion: SchemaVersion,
		Capabilities:  s.Capabilities(),
		Types:         types,
	}
	if a, err := s.Snapshot(ctx); err == nil {
		h.CapturedAt = a.Topology.CapturedAt
		h.Insights = nonNil(a.Insights)
	} else {
		h.Insights = nonNil(nil)
		h.Error = "snapshot failed: " + err.Error()
	}
	return Event{Type: EventHello, At: time.Now(), Data: h}
}
