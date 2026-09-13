package kb

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"portauthority/core/model"
)

//go:embed data/docks.json
var docksRaw []byte

//go:embed data/devices.json
var devicesRaw []byte

// Source is the knowledge base layer an entry came from.
//
// Three layers stack, later ones winning by id and by hub: what ships
// inside the binary, what the community knowledge base publishes (loaded
// from a cached file; the fetcher is a later phase), and what the user
// added on this machine. A user's own entry therefore always beats a
// shipped one for the same dock, which is what lets someone correct a
// wrong grouping without waiting for a release.
type Source string

const (
	SourceShipped Source = "shipped"
	SourceShared  Source = "shared"
	SourceLocal   Source = "local"
)

// LocalFile is the name of the user's own dock file inside the local
// knowledge base directory. Same schema as the shipped data/docks.json.
const LocalFile = "docks.json"

// DockPort describes one kind of physical port on a dock.
type DockPort struct {
	Label     string          `json:"label"`
	Position  string          `json:"position"`
	Connector string          `json:"connector"`
	MaxLink   model.LinkSpeed `json:"max_link"`
	Count     int             `json:"count"`
	Notes     string          `json:"notes,omitempty"`
}

// PortMapping pins a logical hub port to a physical, labelled port.
type PortMapping struct {
	HubVendorID  uint16
	HubProductID uint16
	Port         int
	Label        string
	Position     string
	Verified     string
}

// UplinkSpec is how a dock reaches the computer.
type UplinkSpec struct {
	Kind    string          `json:"kind"`
	MaxLink model.LinkSpeed `json:"max_link"`
}

// USB4Spec is the vendor and model strings the dock's own USB4 router
// announces. The router's vid:pid is the bridge silicon, shared by many
// docks, so the strings are what tell one dock's router from another's.
type USB4Spec struct {
	Vendor string `json:"vendor"`
	Model  string `json:"model"`
	Notes  string `json:"notes,omitempty"`
}

// PortMapEntry is the on-disk form of a PortMapping.
type PortMapEntry struct {
	Hub      string `json:"hub"`
	Port     int    `json:"port"`
	Label    string `json:"label"`
	Position string `json:"position"`
	Verified string `json:"verified,omitempty"`
}

// DockEntry is a dock as written in a docks file, the same in every layer.
// It is also the shape the API accepts and returns, and the shape a future
// "share with the community" submission carries.
type DockEntry struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Hubs lists every logical hub the dock exposes as "vid:pid", so its
	// internal chain folds into one box.
	Hubs    []string       `json:"hubs"`
	Uplink  *UplinkSpec    `json:"uplink,omitempty"`
	USB4    *USB4Spec      `json:"usb4,omitempty"`
	Ports   []DockPort     `json:"ports,omitempty"`
	PortMap []PortMapEntry `json:"port_map,omitempty"`
	// Verified says how the entry was confirmed: "hardware" for entries
	// checked on a real dock, otherwise where the data came from.
	Verified string `json:"verified,omitempty"`
	Notes    string `json:"notes,omitempty"`
}

// Dock is a known dock: the logical hubs it exposes and its physical ports.
type Dock struct {
	ID            string
	Name          string
	Source        Source
	Verified      string
	Notes         string
	UplinkKind    string
	UplinkMaxLink model.LinkSpeed
	USB4          *USB4Spec
	Ports         []DockPort
	PortMap       []PortMapping
	hubs          map[uint32]bool
	// usb4Vendor and usb4Model are the normalised router strings.
	usb4Vendor string
	usb4Model  string
	entry      DockEntry
}

// KnownDevice is capability data a device does not report over USB.
type KnownDevice struct {
	Name             string          `json:"name"`
	Kind             string          `json:"kind"`
	MaxLink          model.LinkSpeed `json:"max_link"`
	ClaimedReadMBps  int             `json:"claimed_read_mbps"`
	ClaimedWriteMBps int             `json:"claimed_write_mbps"`
	Notes            string          `json:"notes,omitempty"`
	// USB4ID is the router "vid:pid" the device presents when it tunnels
	// PCIe over USB4 / Thunderbolt, a different namespace from its USB
	// identity. Optional.
	USB4ID string `json:"usb4_id,omitempty"`
}

// docksFile is the schema of every docks file, shipped or not.
type docksFile struct {
	Comment             string            `json:"$comment,omitempty"`
	GenericInternalHubs map[string]string `json:"generic_internal_hubs,omitempty"`
	Docks               []DockEntry       `json:"docks"`
}

// catalog is one immutable, fully indexed view across every layer. It is
// swapped atomically whenever a layer changes, so lookups never see a
// half-built index.
type catalog struct {
	docks          []*Dock
	byID           map[string]*Dock
	dockByHub      map[uint32]*Dock
	genericDockHub map[uint32]string
	knownDevices   map[uint32]*KnownDevice
	knownByUSB4    map[uint32]*KnownDevice
}

var (
	current atomic.Pointer[catalog]

	// layersMu guards the mutable layers and the local directory; the
	// catalog itself is read lock-free through `current`.
	layersMu      sync.Mutex
	shared        []DockEntry
	sharedDevices map[string]KnownDevice
	local         []DockEntry
	localDir      string
)

// cat is the current catalog, built from the shipped data on first use.
// Readers never take the lock once it exists; the first one does, and a
// writer that got there first has already built it.
func cat() *catalog {
	if c := current.Load(); c != nil {
		return c
	}
	layersMu.Lock()
	defer layersMu.Unlock()
	return catalogLocked()
}

// catalogLocked is cat for callers already holding layersMu. Taking the
// lock again from inside would deadlock, which is exactly what a first
// write before any read used to do.
func catalogLocked() *catalog {
	if c := current.Load(); c != nil {
		return c
	}
	rebuild()
	return current.Load()
}

// DockForHub returns the dock a logical hub belongs to.
func DockForHub(vid, pid uint16) (*Dock, bool) {
	d, ok := cat().dockByHub[productKey(vid, pid)]
	return d, ok
}

// DockByID returns a dock from any layer.
func DockByID(id string) (*Dock, bool) {
	d, ok := cat().byID[id]
	return d, ok
}

// Docks lists every dock across the layers, shipped first.
func Docks() []*Dock {
	return append([]*Dock(nil), cat().docks...)
}

// IsGenericDockHub reports whether a hub is a chipset hub found inside many
// docks (it belongs to whichever dock sits below it).
func IsGenericDockHub(vid, pid uint16) bool {
	_, ok := cat().genericDockHub[productKey(vid, pid)]
	return ok
}

// KnownDeviceInfo returns curated capability data for a device.
func KnownDeviceInfo(vid, pid uint16) (*KnownDevice, bool) {
	d, ok := cat().knownDevices[productKey(vid, pid)]
	return d, ok
}

// KnownDeviceByUSB4ID resolves a device by its USB4 router vendor and
// product id (the "usb4_id" alias in devices.json).
func KnownDeviceByUSB4ID(vid, pid uint16) (*KnownDevice, bool) {
	d, ok := cat().knownByUSB4[productKey(vid, pid)]
	return d, ok
}

// DockForUSB4Strings resolves the dock whose own USB4 router announces
// these DROM vendor and model strings ("CalDigit, Inc.", "TS4"). Only
// docks that record their router strings can match.
func DockForUSB4Strings(vendor, model string) (*Dock, bool) {
	vendorWords := strings.Fields(NormalizeName(vendor))
	model = NormalizeName(model)
	if len(vendorWords) == 0 || len(model) < 2 {
		return nil, false
	}
	for _, d := range cat().docks {
		if d.usb4Model != "" && d.usb4Vendor == vendorWords[0] && d.usb4Model == model {
			return d, true
		}
	}
	return nil, false
}

// DockForUSB4Router resolves a dock from the inbox driver's friendly name
// ("USB4 Router (1.0), CalDigit. Inc. - TS4"), for providers and fixtures
// that carry no DROM strings.
func DockForUSB4Router(name string) (*Dock, bool) {
	vendor, model, ok := ParseUSB4Name(name)
	if !ok {
		return nil, false
	}
	return DockForUSB4Strings(vendor, model)
}

// ParseUSB4Name splits the inbox driver's router name, "USB4 Router (2.0),
// Corsair - EX400U", into a normalised vendor token ("corsair") and product
// token ("ex400u"). The strings come from the router's configuration
// space, so they name the product on the desk rather than its silicon.
func ParseUSB4Name(name string) (vendor, product string, ok bool) {
	_, rest, found := strings.Cut(name, ",")
	if !found {
		return "", "", false
	}
	v, p, found := strings.Cut(rest, " - ")
	if !found {
		return "", "", false
	}
	vendorWords := strings.Fields(NormalizeName(v))
	product = strings.TrimSpace(NormalizeName(p))
	if len(vendorWords) == 0 || len(product) < 3 {
		return "", "", false
	}
	return vendorWords[0], product, true
}

// NormalizeName lower-cases and keeps only letters, digits and spaces so
// "CalDigit. Inc." and "CALDIGIT_INC" compare equal.
func NormalizeName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-' || r == ' ' || r == '.' || r == ',':
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// HasHub reports whether the given logical hub is part of this dock.
func (d *Dock) HasHub(vid, pid uint16) bool {
	return d.hubs[productKey(vid, pid)]
}

// Entry is the dock as it would be written to a docks file.
func (d *Dock) Entry() DockEntry {
	return d.entry
}

// BestLink is the fastest link any of the dock's ports can offer.
func (d *Dock) BestLink() model.LinkSpeed {
	best := model.LinkUnknown
	for _, p := range d.Ports {
		if p.MaxLink > best {
			best = p.MaxLink
		}
	}
	return best
}

// PortsAtLeast lists port kinds that can carry the given link or better.
func (d *Dock) PortsAtLeast(link model.LinkSpeed) []DockPort {
	var out []DockPort
	for _, p := range d.Ports {
		if p.MaxLink >= link {
			out = append(out, p)
		}
	}
	return out
}

// PhysicalPort resolves a logical hub port to its printed label, when known.
func (d *Dock) PhysicalPort(hubVID, hubPID uint16, port int) (PortMapping, bool) {
	for _, m := range d.PortMap {
		if m.HubVendorID == hubVID && m.HubProductID == hubPID && m.Port == port {
			return m, true
		}
	}
	return PortMapping{}, false
}

// ---------------------------------------------------------------------------
// Layers
// ---------------------------------------------------------------------------

// DefaultLocalDir is where the user's own knowledge base lives:
// the per-user config directory, so it survives reinstalls and is never
// inside the program folder.
func DefaultLocalDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "PortAuthority", "kb"), nil
}

// UseLocalDir loads the user's docks from dir (a missing file is an empty
// layer) and makes dir the target of later additions. An empty dir turns
// the local layer off, which is what a read-only run wants.
func UseLocalDir(dir string) error {
	layersMu.Lock()
	defer layersMu.Unlock()
	entries := []DockEntry{}
	if dir != "" {
		loaded, err := readDocksFile(filepath.Join(dir, LocalFile))
		if err != nil {
			return err
		}
		entries = loaded
	}
	localDir = dir
	local = entries
	rebuild()
	return nil
}

// LocalDir is the directory local additions are written to; empty when
// the local layer is off.
func LocalDir() string {
	layersMu.Lock()
	defer layersMu.Unlock()
	return localDir
}

// UseShared replaces the community dock layer, which core/community
// fetches from the usb-device-kb releases. An entry that cannot be indexed
// (a malformed hub id) is left out and named in the returned error; the
// rest still apply, since one bad entry must not cost the whole layer.
func UseShared(entries []DockEntry) error {
	layersMu.Lock()
	defer layersMu.Unlock()
	kept, err := wellFormed(entries)
	shared = kept
	rebuild()
	return err
}

// UseSharedDevices replaces the community device layer. Keys are
// "vid:pid"; a malformed key is left out and named in the returned error.
func UseSharedDevices(devices map[string]KnownDevice) error {
	layersMu.Lock()
	defer layersMu.Unlock()
	var rejected []string
	kept := map[string]KnownDevice{}
	for key, dev := range devices {
		if _, err := parsePair(key); err != nil {
			rejected = append(rejected, key)
			continue
		}
		if dev.USB4ID != "" {
			if _, err := parsePair(dev.USB4ID); err != nil {
				rejected = append(rejected, key)
				continue
			}
		}
		kept[key] = dev
	}
	sharedDevices = kept
	rebuild()
	if len(rejected) > 0 {
		return fmt.Errorf("kb: %d shared device(s) rejected: %s", len(rejected), strings.Join(rejected, ", "))
	}
	return nil
}

// wellFormed keeps the entries whose ids parse, reporting the rest.
func wellFormed(entries []DockEntry) ([]DockEntry, error) {
	kept := make([]DockEntry, 0, len(entries))
	var rejected []string
	for _, e := range entries {
		ok := e.ID != "" && len(e.Hubs) > 0
		for _, h := range e.Hubs {
			if _, err := parsePair(h); err != nil {
				ok = false
			}
		}
		for _, m := range e.PortMap {
			if _, err := parsePair(m.Hub); err != nil {
				ok = false
			}
		}
		if !ok {
			rejected = append(rejected, firstNonEmpty(e.ID, e.Name, "unnamed"))
			continue
		}
		kept = append(kept, e)
	}
	if len(rejected) > 0 {
		return kept, fmt.Errorf("kb: %d shared dock(s) rejected: %s", len(rejected), strings.Join(rejected, ", "))
	}
	return kept, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// ErrNoLocalLayer is returned when writing without a local directory.
var ErrNoLocalLayer = errors.New("kb: no local knowledge base directory")

// ErrNotLocal is returned when removing a dock that is not the user's own.
var ErrNotLocal = errors.New("kb: dock is not in the local layer")

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// AddLocalDock validates the entry, gives it an id when it has none, writes
// the local file and rebuilds the catalog. It returns the dock as indexed.
func AddLocalDock(entry DockEntry) (*Dock, error) {
	layersMu.Lock()
	defer layersMu.Unlock()
	if localDir == "" {
		return nil, ErrNoLocalLayer
	}
	entry.Name = strings.TrimSpace(entry.Name)
	if entry.Name == "" {
		return nil, errors.New("kb: a dock needs a name")
	}
	if len(entry.Hubs) == 0 {
		return nil, errors.New("kb: a dock needs at least one hub")
	}
	// A chipset hub found inside many docks (Goshen Ridge's) must not be
	// claimed by one of them: it folds into whichever dock it sits in
	// already, and listing it here would pull every such dock into this one.
	generic := catalogLocked().genericDockHub
	hubs := make([]string, 0, len(entry.Hubs))
	for _, h := range entry.Hubs {
		key, err := parsePair(h)
		if err != nil {
			return nil, fmt.Errorf("kb: hub %q: %w", h, err)
		}
		if _, isGeneric := generic[key]; isGeneric {
			continue
		}
		hubs = append(hubs, pairString(key))
	}
	entry.Hubs = hubs
	if len(entry.Hubs) == 0 {
		return nil, errors.New("kb: a dock needs at least one hub of its own")
	}
	if entry.USB4 != nil && strings.TrimSpace(entry.USB4.Model) == "" {
		entry.USB4 = nil
	}
	if entry.ID == "" {
		entry.ID = uniqueLocalID(entry.Name)
	} else if !strings.HasPrefix(entry.ID, "local:") {
		return nil, errors.New("kb: a local dock id must start with \"local:\"")
	}
	if entry.Verified == "" {
		entry.Verified = "user, " + time.Now().Format("2006-01-02")
	}
	next := make([]DockEntry, 0, len(local)+1)
	for _, e := range local {
		if e.ID != entry.ID {
			next = append(next, e)
		}
	}
	next = append(next, entry)
	if err := writeDocksFile(filepath.Join(localDir, LocalFile), next); err != nil {
		return nil, err
	}
	local = next
	rebuild()
	return current.Load().byID[entry.ID], nil
}

// RemoveLocalDock deletes one of the user's own docks.
func RemoveLocalDock(id string) error {
	layersMu.Lock()
	defer layersMu.Unlock()
	if localDir == "" {
		return ErrNoLocalLayer
	}
	next := make([]DockEntry, 0, len(local))
	found := false
	for _, e := range local {
		if e.ID == id {
			found = true
			continue
		}
		next = append(next, e)
	}
	if !found {
		return ErrNotLocal
	}
	if err := writeDocksFile(filepath.Join(localDir, LocalFile), next); err != nil {
		return err
	}
	local = next
	rebuild()
	return nil
}

func uniqueLocalID(name string) string {
	slug := strings.Trim(slugRe.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if slug == "" {
		slug = "dock"
	}
	taken := map[string]bool{}
	for _, e := range local {
		taken[e.ID] = true
	}
	id := "local:" + slug
	for n := 2; taken[id]; n++ {
		id = fmt.Sprintf("local:%s-%d", slug, n)
	}
	return id
}

func readDocksFile(path string) ([]DockEntry, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return []DockEntry{}, nil
	}
	if err != nil {
		return nil, err
	}
	var f docksFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("kb: %s: %w", path, err)
	}
	if f.Docks == nil {
		f.Docks = []DockEntry{}
	}
	return f.Docks, nil
}

// writeDocksFile writes atomically: a half-written file must never be the
// one the next start reads.
func writeDocksFile(path string, entries []DockEntry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f := docksFile{
		Comment: "Docks added on this machine through Port Authority. Same schema as the shipped docks.json; entries here override shipped and shared ones with the same id or hubs.",
		Docks:   entries,
	}
	raw, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// rebuild indexes every layer into a fresh catalog and publishes it.
// Callers hold layersMu.
func rebuild() {
	var shippedFile docksFile
	if err := json.Unmarshal(docksRaw, &shippedFile); err != nil {
		panic(fmt.Sprintf("kb: docks.json: %v", err))
	}
	c := &catalog{
		byID:           map[string]*Dock{},
		dockByHub:      map[uint32]*Dock{},
		genericDockHub: map[uint32]string{},
		knownDevices:   map[uint32]*KnownDevice{},
		knownByUSB4:    map[uint32]*KnownDevice{},
	}
	for k, desc := range shippedFile.GenericInternalHubs {
		c.genericDockHub[mustParsePair(k, "docks.json generic_internal_hubs")] = desc
	}

	// Later layers replace earlier ones with the same id, and their hubs
	// take over the hub index, so a local correction wins outright.
	type layered struct {
		entry  DockEntry
		source Source
	}
	var ordered []layered
	replace := func(entries []DockEntry, source Source) {
		for _, e := range entries {
			kept := ordered[:0]
			for _, l := range ordered {
				if l.entry.ID != e.ID {
					kept = append(kept, l)
				}
			}
			ordered = append(kept, layered{entry: e, source: source})
		}
	}
	replace(shippedFile.Docks, SourceShipped)
	replace(shared, SourceShared)
	replace(local, SourceLocal)

	for _, l := range ordered {
		d := buildDock(l.entry, l.source)
		c.docks = append(c.docks, d)
		c.byID[d.ID] = d
		for key := range d.hubs {
			c.dockByHub[key] = d
		}
	}

	var rawDev struct {
		Devices map[string]*KnownDevice `json:"devices"`
	}
	if err := json.Unmarshal(devicesRaw, &rawDev); err != nil {
		panic(fmt.Sprintf("kb: devices.json: %v", err))
	}
	for k, dev := range rawDev.Devices {
		c.knownDevices[mustParsePair(k, "devices.json")] = dev
		if dev.USB4ID != "" {
			c.knownByUSB4[mustParsePair(dev.USB4ID, "devices.json usb4_id")] = dev
		}
	}
	// The community layer overlays the shipped devices by key; the keys
	// were checked when the layer was set, so no panic is possible here.
	for k, dev := range sharedDevices {
		dev := dev
		key, _ := parsePair(k)
		c.knownDevices[key] = &dev
		if dev.USB4ID != "" {
			usb4Key, _ := parsePair(dev.USB4ID)
			c.knownByUSB4[usb4Key] = &dev
		}
	}
	current.Store(c)
}

func buildDock(e DockEntry, source Source) *Dock {
	d := &Dock{
		ID: e.ID, Name: e.Name, Source: source, Verified: e.Verified, Notes: e.Notes,
		USB4:  e.USB4,
		Ports: e.Ports,
		hubs:  map[uint32]bool{},
		entry: e,
	}
	if e.Uplink != nil {
		d.UplinkKind, d.UplinkMaxLink = e.Uplink.Kind, e.Uplink.MaxLink
	}
	if e.USB4 != nil && e.USB4.Model != "" {
		vendorWords := strings.Fields(NormalizeName(e.USB4.Vendor))
		if len(vendorWords) == 0 {
			panic(fmt.Sprintf("kb: dock %s: usb4.vendor must name the vendor", e.ID))
		}
		d.usb4Vendor = vendorWords[0]
		d.usb4Model = NormalizeName(e.USB4.Model)
	}
	for _, h := range e.Hubs {
		d.hubs[mustParsePair(h, "docks hubs")] = true
	}
	for _, m := range e.PortMap {
		key := mustParsePair(m.Hub, "docks port_map")
		d.PortMap = append(d.PortMap, PortMapping{
			HubVendorID: uint16(key >> 16), HubProductID: uint16(key),
			Port: m.Port, Label: m.Label, Position: m.Position, Verified: m.Verified,
		})
	}
	return d
}

// parsePair parses "vid:pid" hex into a product key.
func parsePair(s string) (uint32, error) {
	vidStr, pidStr, ok := strings.Cut(strings.TrimSpace(s), ":")
	if !ok {
		return 0, errors.New("must be vid:pid")
	}
	vid, err1 := strconv.ParseUint(vidStr, 16, 16)
	pid, err2 := strconv.ParseUint(pidStr, 16, 16)
	if err1 != nil || err2 != nil {
		return 0, errors.New("is not hex")
	}
	return productKey(uint16(vid), uint16(pid)), nil
}

// mustParsePair is parsePair for embedded data, which is a build error
// rather than a runtime one when malformed.
func mustParsePair(s, where string) uint32 {
	key, err := parsePair(s)
	if err != nil {
		panic(fmt.Sprintf("kb: %s: key %q %v", where, s, err))
	}
	return key
}

func pairString(key uint32) string {
	return fmt.Sprintf("%04x:%04x", uint16(key>>16), uint16(key))
}
