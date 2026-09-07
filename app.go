package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"runtime"
	"sync"
	"time"

	"portauthority/core/api"
	"portauthority/platform"
)

// preferredAPIAddr is used when free; otherwise any loopback port. A
// running `pactl serve` on 7911 therefore does not stop the app.
const preferredAPIAddr = "127.0.0.1:7911"

// App hosts the collector + API for the window and is bound to the
// frontend so it can discover where the API lives.
type App struct {
	ctx     context.Context
	cancel  context.CancelFunc
	runDone chan struct{} // closed when the API event loop has fully stopped

	mu      sync.RWMutex
	addr    string
	ready   bool
	lastErr string
}

// AppStatus is what the frontend polls until the API is up.
type AppStatus struct {
	Ready     bool   `json:"ready"`
	Error     string `json:"error,omitempty"`
	APIBase   string `json:"api_base"`
	StreamURL string `json:"stream_url"`
	AppName   string `json:"app_name"`
	Version   string `json:"version"`
	Platform  string `json:"platform"`
}

// NewApp creates the app shell; nothing starts until startup.
func NewApp() *App { return &App{} }

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	go a.serve()
}

// shutdown cancels the collector and waits for it to release the ETW
// session; exiting early would leave the session running in the kernel.
func (a *App) shutdown(context.Context) {
	if a.cancel == nil {
		return
	}
	a.cancel()
	select {
	case <-a.runDone:
	case <-time.After(10 * time.Second):
		log.Printf("%s: event loop did not stop in time", AppName)
	}
}

// serve starts the provider, the API event loop and the HTTP server.
func (a *App) serve() {
	addr, err := chooseAddr()
	if err != nil {
		a.fail(fmt.Sprintf("no loopback port available: %v", err))
		return
	}
	p, err := platform.New()
	if err != nil {
		a.fail(fmt.Sprintf("no provider for this platform: %v", err))
		return
	}
	svc := api.NewService(p, api.WithLogger(log.Default()))
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.runDone = make(chan struct{})

	go func() {
		defer close(a.runDone)
		if err := svc.Run(ctx); err != nil && ctx.Err() == nil {
			log.Printf("api: event loop: %v", err)
		}
	}()
	go func() {
		if err := api.ListenAndServe(ctx, addr, svc.Handler()); err != nil && ctx.Err() == nil {
			a.fail(fmt.Sprintf("api server: %v", err))
		}
	}()

	// Mark ready once the health endpoint answers.
	for i := 0; i < 50; i++ {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			a.mu.Lock()
			a.addr, a.ready = addr, true
			a.mu.Unlock()
			log.Printf("%s: API on http://%s", AppName, addr)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	a.fail("api server did not start in time")
}

func (a *App) fail(msg string) {
	log.Printf("%s: %s", AppName, msg)
	a.mu.Lock()
	a.lastErr = msg
	a.mu.Unlock()
}

// Status is bound to the frontend.
func (a *App) Status() AppStatus {
	a.mu.RLock()
	defer a.mu.RUnlock()
	s := AppStatus{
		Ready: a.ready, Error: a.lastErr,
		AppName: AppName, Version: Version, Platform: runtime.GOOS,
	}
	if a.addr != "" {
		s.APIBase = "http://" + a.addr
		s.StreamURL = "ws://" + a.addr + "/api/v1/stream"
	}
	return s
}

// chooseAddr returns the preferred port if free, else a free loopback port.
func chooseAddr() (string, error) {
	if l, err := net.Listen("tcp", preferredAPIAddr); err == nil {
		l.Close()
		return preferredAPIAddr, nil
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer l.Close()
	return l.Addr().String(), nil
}
