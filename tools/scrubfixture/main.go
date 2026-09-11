// scrubfixture replaces device serial numbers and the machine's name in
// topology fixtures with stable placeholders so captured snapshots can be
// published.
//
//	go run ./tools/scrubfixture [-dry] testdata/fixtures/*.json
//
// A serial is anything that appears in a serial_number field, or as the
// last segment of an id / instance_id that is not a bus-position segment
// like "5&270C603&0&4". Every occurrence in the file is replaced textually,
// so formatting and key order are preserved. Replacements are numbered in
// sorted order of the originals, so re-running is a no-op.
//
// The host name (host.name) is replaced in place, not file-wide: a short
// name such as "PC" would otherwise be rewritten inside every string that
// happens to contain it. The make and model stay; they describe hardware,
// not a person.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// busPosition matches Windows' generated instance segments such as
// "7&1B27237F&0&3" (USB) or "3&11583659&1&A0" (PCI, hex function).
var busPosition = regexp.MustCompile(`^\d+&[0-9A-Fa-f]+&\d+&[0-9A-Fa-f]+$`)

// placeholder matches values this tool has already written, including an
// instance segment that keeps its prefix around one ("MSFT30SCRUBBED-06"),
// so a second run leaves a scrubbed file alone.
var placeholder = regexp.MustCompile(`SCRUBBED-\d+$`)

// hostPlaceholder replaces the machine's name.
const hostPlaceholder = "SCRUBBED-HOST"

func main() {
	dry := flag.Bool("dry", false, "report what would change without writing")
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: scrubfixture [-dry] file.json ...")
		os.Exit(2)
	}
	exit := 0
	for _, path := range flag.Args() {
		n, host, err := scrubFile(path, *dry)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			exit = 1
			continue
		}
		hostNote := ""
		if host {
			hostNote = ", host name scrubbed"
		}
		fmt.Printf("%s: %d serial(s) scrubbed%s\n", path, n, hostNote)
	}
	os.Exit(exit)
}

// scrubFile returns how many serials were replaced and whether the host
// name was.
func scrubFile(path string, dry bool) (int, bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, false, err
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return 0, false, fmt.Errorf("parse: %w", err)
	}
	out, host, err := scrubHost(string(raw), doc)
	if err != nil {
		return 0, false, err
	}
	if dry && host {
		fmt.Printf("  host name -> %s\n", hostPlaceholder)
	}

	serials := map[string]bool{}
	collect(doc, serials)

	// An instance-id segment that merely wraps a serial ("MSFT30" + serial
	// for UASP drives) is scrubbed through the serial, keeping the prefix.
	for s := range serials {
		for t := range serials {
			if s != t && strings.HasSuffix(s, t) {
				delete(serials, s)
				break
			}
		}
	}

	ordered := make([]string, 0, len(serials))
	for s := range serials {
		ordered = append(ordered, s)
	}
	sort.Strings(ordered)

	// Replace longest first so one serial that contains another is handled
	// as a whole.
	byLength := append([]string(nil), ordered...)
	sort.SliceStable(byLength, func(i, j int) bool { return len(byLength[i]) > len(byLength[j]) })
	index := map[string]int{}
	for i, s := range ordered {
		index[s] = i + 1
	}
	for _, s := range byLength {
		out = strings.ReplaceAll(out, s, fmt.Sprintf("SCRUBBED-%02d", index[s]))
	}
	if dry {
		for _, s := range ordered {
			fmt.Printf("  %s -> SCRUBBED-%02d\n", s, index[s])
		}
		return len(ordered), host, nil
	}
	if out == string(raw) {
		return 0, false, nil
	}
	return len(ordered), host, os.WriteFile(path, []byte(out), 0o644)
}

// hostNameField finds `"name": "<value>"` as it appears in the file.
var hostNameField = regexp.MustCompile(`"name"\s*:\s*"((?:[^"\\]|\\.)*)"`)

// scrubHost replaces host.name in the raw text, leaving everything else
// as written. It edits the first "name" pair after the "host" key, which
// is the host object's own since the object is small and name comes first
// as the provider writes it.
func scrubHost(raw string, doc any) (string, bool, error) {
	top, ok := doc.(map[string]any)
	if !ok {
		return raw, false, nil
	}
	host, ok := top["host"].(map[string]any)
	if !ok {
		return raw, false, nil
	}
	name, ok := host["name"].(string)
	if !ok || name == "" || name == hostPlaceholder {
		return raw, false, nil
	}
	start := strings.Index(raw, `"host"`)
	if start < 0 {
		return raw, false, fmt.Errorf("host object not found in the text")
	}
	loc := hostNameField.FindStringSubmatchIndex(raw[start:])
	if loc == nil {
		return raw, false, fmt.Errorf("host name not found in the text")
	}
	valueStart, valueEnd := start+loc[2], start+loc[3]
	var decoded string
	if err := json.Unmarshal([]byte(`"`+raw[valueStart:valueEnd]+`"`), &decoded); err != nil || decoded != name {
		return raw, false, fmt.Errorf("host name in the text (%q) does not match the parsed value (%q)", raw[valueStart:valueEnd], name)
	}
	return raw[:valueStart] + hostPlaceholder + raw[valueEnd:], true, nil
}

// collect walks the document gathering serial-like values.
func collect(v any, serials map[string]bool) {
	switch x := v.(type) {
	case map[string]any:
		if s, ok := x["serial_number"].(string); ok {
			addSerial(s, serials)
		}
		for _, key := range []string{"id", "instance_id"} {
			if s, ok := x[key].(string); ok {
				addSerial(lastSegment(s), serials)
			}
		}
		for _, child := range x {
			collect(child, serials)
		}
	case []any:
		for _, child := range x {
			collect(child, serials)
		}
	}
}

func lastSegment(id string) string {
	if i := strings.LastIndex(id, `\`); i >= 0 {
		return id[i+1:]
	}
	return ""
}

func addSerial(s string, serials map[string]bool) {
	s = strings.TrimSpace(s)
	if len(s) < 4 || busPosition.MatchString(s) || placeholder.MatchString(s) {
		return
	}
	if strings.HasPrefix(s, "path:") || strings.HasPrefix(s, "controller-") {
		return
	}
	serials[s] = true
}
