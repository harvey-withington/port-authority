// Package kb is the static knowledge base: data that is not observed from
// the machine but looked up from bundled reference tables. It currently
// resolves USB vendor and product IDs to names via the public usb.ids
// database embedded at build time.
package kb

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

//go:embed data/usb.ids
var usbIDsRaw string

// overridesRaw layers corrections over usb.ids (see data/overrides.json).
//
//go:embed data/overrides.json
var overridesRaw []byte

var (
	parseOnce sync.Once
	vendors   map[uint16]string
	products  map[uint32]string
	version   string
)

type overrides struct {
	Vendors  map[string]string `json:"vendors"`
	Products map[string]string `json:"products"`
}

// applyOverrides merges data/overrides.json on top of the parsed tables.
// A malformed overrides file is a build-time data bug, so it panics.
func applyOverrides() {
	var o overrides
	if err := json.Unmarshal(overridesRaw, &o); err != nil {
		panic(fmt.Sprintf("kb: overrides.json: %v", err))
	}
	for k, name := range o.Vendors {
		vid, err := strconv.ParseUint(k, 16, 16)
		if err != nil {
			panic(fmt.Sprintf("kb: overrides.json vendor key %q: %v", k, err))
		}
		vendors[uint16(vid)] = name
	}
	for k, name := range o.Products {
		vidStr, pidStr, ok := strings.Cut(k, ":")
		if !ok {
			panic(fmt.Sprintf("kb: overrides.json product key %q: want vid:pid", k))
		}
		vid, err1 := strconv.ParseUint(vidStr, 16, 16)
		pid, err2 := strconv.ParseUint(pidStr, 16, 16)
		if err1 != nil || err2 != nil {
			panic(fmt.Sprintf("kb: overrides.json product key %q: bad hex", k))
		}
		products[productKey(uint16(vid), uint16(pid))] = name
	}
}

// VendorName returns the usb.ids name for a vendor ID, or "" when unknown.
func VendorName(vid uint16) string {
	parseOnce.Do(parse)
	return vendors[vid]
}

// ProductName returns the usb.ids name for a vendor/product pair, or ""
// when unknown.
func ProductName(vid, pid uint16) string {
	parseOnce.Do(parse)
	return products[productKey(vid, pid)]
}

// Version returns the "# Version:" stamp from the embedded usb.ids header.
func Version() string {
	parseOnce.Do(parse)
	return version
}

func productKey(vid, pid uint16) uint32 {
	return uint32(vid)<<16 | uint32(pid)
}

// parse reads the vendor and product sections of usb.ids. Vendor lines are
// "XXXX  Name"; product lines are one tab then "XXXX  Name"; interface
// lines (two tabs) and everything from the first "C " class line onward are
// ignored.
func parse() {
	vendors = make(map[uint16]string, 4096)
	products = make(map[uint32]string, 32768)

	var curVendor uint16
	haveVendor := false

	for _, line := range strings.Split(usbIDsRaw, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			if version == "" {
				if v, ok := strings.CutPrefix(line, "# Version:"); ok {
					version = strings.TrimSpace(v)
				}
			}
			continue
		}
		if strings.HasPrefix(line, "C ") {
			break
		}
		if line[0] == '\t' {
			if !haveVendor || len(line) > 1 && line[1] == '\t' {
				continue
			}
			id, name, ok := splitEntry(line[1:])
			if !ok {
				continue
			}
			products[productKey(curVendor, id)] = name
			continue
		}
		id, name, ok := splitEntry(line)
		if !ok {
			haveVendor = false
			continue
		}
		curVendor = id
		haveVendor = true
		vendors[id] = name
	}
	applyOverrides()
}

// splitEntry parses "XXXX  Name" into its hex ID and trimmed name.
func splitEntry(s string) (uint16, string, bool) {
	if len(s) < 6 || s[4] != ' ' || s[5] != ' ' {
		return 0, "", false
	}
	id, err := strconv.ParseUint(s[:4], 16, 16)
	if err != nil {
		return 0, "", false
	}
	name := strings.TrimSpace(s[6:])
	if name == "" {
		return 0, "", false
	}
	return uint16(id), name, true
}
