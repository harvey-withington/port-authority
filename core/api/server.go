package api

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// ErrNotLoopback is returned by ListenAndServe for a bind address that
// would expose the API beyond this machine.
var ErrNotLoopback = errors.New("api: bind address must be loopback (127.0.0.1, localhost or ::1)")

// shutdownGrace is how long in-flight requests get after ctx is cancelled.
const shutdownGrace = 5 * time.Second

// ListenAndServe serves h on addr until ctx is cancelled, then shuts down
// gracefully. It refuses any address that is not loopback.
func ListenAndServe(ctx context.Context, addr string, h http.Handler) error {
	if err := checkLoopback(addr); err != nil {
		return err
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("api: listen %s: %w", addr, err)
	}
	srv := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		// Requests derive from ctx so long-lived handlers (the WebSocket
		// stream, which Shutdown cannot see once hijacked) end with it.
		BaseContext: func(net.Listener) context.Context { return ctx },
	}

	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()

	select {
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		sctx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		if err := srv.Shutdown(sctx); err != nil {
			return fmt.Errorf("api: shutdown: %w", err)
		}
		<-errc
		return nil
	}
}

// checkLoopback validates that addr's host is a loopback address.
func checkLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("api: bad address %q: %w", addr, err)
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("%w: got %q", ErrNotLoopback, host)
	}
	return nil
}
