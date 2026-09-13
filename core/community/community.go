// Package community keeps the shared knowledge base layer up to date from
// the usb-device-kb repository's rolling "latest" release.
//
// The files are fetched with If-None-Match and cached beside the user's
// own docks, so a start with no network runs on the last copy, and a
// start with network only downloads when something changed. What arrives
// is decoded strictly and handed to core/kb, which keeps the entries it
// can index and reports the rest. Nothing here writes anywhere but the
// cache directory, and nothing is sent: sharing a dock goes through a
// GitHub issue the app merely opens in the browser.
package community

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"portauthority/core/kb"
)

// Repo is the community knowledge base.
const Repo = "https://github.com/harvey-withington/usb-device-kb"

// ReleaseBase is where the rolling "latest" release serves the files.
const ReleaseBase = Repo + "/releases/download/latest/"

// maxFileBytes bounds a download; the real files are a few kilobytes.
const maxFileBytes = 4 << 20

// cacheSubdir is the folder under the knowledge base directory the shared
// copies live in, apart from the user's own docks.json.
const cacheSubdir = "shared"

var files = []string{"docks.json", "devices.json"}

// FileStatus is what is known about one cached file.
type FileStatus struct {
	ETag      string    `json:"etag,omitempty"`
	FetchedAt time.Time `json:"fetched_at"`
}

// Status is the fetcher's state as the API reports it.
type Status struct {
	CacheDir    string                `json:"cache_dir"`
	Files       map[string]FileStatus `json:"files"`
	LastChecked time.Time             `json:"last_checked,omitempty"`
	LastError   string                `json:"last_error,omitempty"`
}

// Fetcher downloads and caches the shared files for one directory.
type Fetcher struct {
	dir    string
	base   string
	client *http.Client
	logger *log.Logger

	mu     sync.Mutex
	status Status
}

// Option configures a Fetcher.
type Option func(*Fetcher)

// WithBaseURL points the fetcher elsewhere, for tests.
func WithBaseURL(base string) Option { return func(f *Fetcher) { f.base = base } }

// WithClient replaces the HTTP client.
func WithClient(c *http.Client) Option { return func(f *Fetcher) { f.client = c } }

// WithLogger sets where problems are logged.
func WithLogger(l *log.Logger) Option {
	return func(f *Fetcher) {
		if l != nil {
			f.logger = l
		}
	}
}

// New makes a fetcher caching under dir, the knowledge base directory.
func New(dir string, opts ...Option) *Fetcher {
	f := &Fetcher{
		dir:    filepath.Join(dir, cacheSubdir),
		base:   ReleaseBase,
		client: &http.Client{Timeout: 30 * time.Second},
		logger: log.New(io.Discard, "", 0),
	}
	for _, o := range opts {
		o(f)
	}
	f.status = Status{CacheDir: f.dir, Files: map[string]FileStatus{}}
	f.readMeta()
	return f
}

// LoadCache applies the cached copies, if any, without touching the
// network. Call it before the first snapshot so the shared layer is in
// place from the start.
func (f *Fetcher) LoadCache() error {
	var errs []error
	for _, name := range files {
		raw, err := os.ReadFile(filepath.Join(f.dir, name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if err := apply(name, raw); err != nil {
			errs = append(errs, fmt.Errorf("cached %s: %w", name, err))
		}
	}
	return errors.Join(errs...)
}

// Refresh asks the release for each file, downloading only what changed,
// and applies anything new. It reports whether the layer changed.
func (f *Fetcher) Refresh(ctx context.Context) (changed bool, err error) {
	var errs []error
	for _, name := range files {
		updated, ferr := f.refreshFile(ctx, name)
		if ferr != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, ferr))
			continue
		}
		changed = changed || updated
	}
	err = errors.Join(errs...)
	f.mu.Lock()
	f.status.LastChecked = time.Now()
	f.status.LastError = ""
	if err != nil {
		f.status.LastError = err.Error()
	}
	f.mu.Unlock()
	f.writeMeta()
	return changed, err
}

func (f *Fetcher) refreshFile(ctx context.Context, name string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.base+name, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "port-authority (+"+Repo+")")
	f.mu.Lock()
	if etag := f.status.Files[name].ETag; etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	f.mu.Unlock()

	res, err := f.client.Do(req)
	if err != nil {
		return false, err
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusNotModified:
		return false, nil
	case http.StatusOK:
	default:
		return false, fmt.Errorf("unexpected status %s", res.Status)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxFileBytes+1))
	if err != nil {
		return false, err
	}
	if len(raw) > maxFileBytes {
		return false, fmt.Errorf("larger than %d bytes", maxFileBytes)
	}
	// Apply before caching: a file the knowledge base rejects outright is
	// not worth keeping, and the previous cache stays in force.
	if err := apply(name, raw); err != nil {
		return false, err
	}
	if err := writeAtomic(filepath.Join(f.dir, name), raw); err != nil {
		return false, err
	}
	f.mu.Lock()
	f.status.Files[name] = FileStatus{ETag: res.Header.Get("ETag"), FetchedAt: time.Now()}
	f.mu.Unlock()
	return true, nil
}

// Status is a copy of the fetcher's state.
func (f *Fetcher) Status() Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := f.status
	s.Files = map[string]FileStatus{}
	for k, v := range f.status.Files {
		s.Files[k] = v
	}
	return s
}

// apply hands one downloaded or cached file to the knowledge base. The
// decode is strict, so a file with a shape this build does not know is
// refused rather than half-read; kb then keeps what it can index.
func apply(name string, raw []byte) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	switch name {
	case "docks.json":
		var f struct {
			Comment             string            `json:"$comment"`
			GenericInternalHubs map[string]string `json:"generic_internal_hubs"`
			Docks               []kb.DockEntry    `json:"docks"`
		}
		if err := dec.Decode(&f); err != nil {
			return err
		}
		return kb.UseShared(f.Docks)
	case "devices.json":
		var f struct {
			Comment string                    `json:"$comment"`
			Devices map[string]kb.KnownDevice `json:"devices"`
		}
		if err := dec.Decode(&f); err != nil {
			return err
		}
		return kb.UseSharedDevices(f.Devices)
	}
	return fmt.Errorf("unknown file %s", name)
}

func (f *Fetcher) metaPath() string { return filepath.Join(f.dir, "meta.json") }

func (f *Fetcher) readMeta() {
	raw, err := os.ReadFile(f.metaPath())
	if err != nil {
		return
	}
	var s Status
	if json.Unmarshal(raw, &s) == nil && s.Files != nil {
		f.status.Files = s.Files
		f.status.LastChecked = s.LastChecked
	}
}

func (f *Fetcher) writeMeta() {
	raw, err := json.MarshalIndent(f.Status(), "", "  ")
	if err != nil {
		return
	}
	if err := writeAtomic(f.metaPath(), append(raw, '\n')); err != nil {
		f.logger.Printf("community: cache metadata not written: %v", err)
	}
}

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

var (
	currentMu sync.Mutex
	current   *Fetcher
)

// Run is what the app and pactl call at start: it applies the cache at
// once, then refreshes in the background and calls onChange when the
// layer changed, so the machine is re-read with the new data. Problems
// are logged; the app never waits on the network.
func Run(ctx context.Context, dir string, onChange func(ctx context.Context, detail string) error, logger *log.Logger) *Fetcher {
	f := New(dir, WithLogger(logger))
	currentMu.Lock()
	current = f
	currentMu.Unlock()
	if err := f.LoadCache(); err != nil {
		f.logger.Printf("community: cached knowledge base not fully loaded: %v", err)
	}
	go func() {
		changed, err := f.Refresh(ctx)
		if err != nil {
			f.logger.Printf("community: refresh: %v", err)
		}
		if changed && onChange != nil {
			if err := onChange(ctx, "community knowledge base updated"); err != nil && ctx.Err() == nil {
				f.logger.Printf("community: re-read after update: %v", err)
			}
		}
	}()
	return f
}

// CurrentStatus reports the fetcher Run started, or an empty status.
func CurrentStatus() Status {
	currentMu.Lock()
	f := current
	currentMu.Unlock()
	if f == nil {
		return Status{Files: map[string]FileStatus{}}
	}
	return f.Status()
}
