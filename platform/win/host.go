//go:build windows

package win

import (
	"os"
	"strings"
	"unicode"

	"golang.org/x/sys/windows/registry"

	"portauthority/core/model"
)

// hostInfo names the machine: its network name, and the make and model
// the firmware wrote into SMBIOS, which Windows mirrors under
// HKLM\HARDWARE\DESCRIPTION\System\BIOS. Every field is best effort.
func hostInfo() *model.HostInfo {
	h := &model.HostInfo{}
	if name, err := os.Hostname(); err == nil {
		h.Name = strings.TrimSpace(name)
	}
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\BIOS`, registry.QUERY_VALUE); err == nil {
		defer k.Close()
		read := func(name string) string {
			v, _, err := k.GetStringValue(name)
			if err != nil {
				return ""
			}
			return strings.TrimSpace(v)
		}
		h.Manufacturer = tidyMaker(read("SystemManufacturer"))
		// SystemFamily carries the name people know ("Yoga Pro 9 16IRP8")
		// where SystemProductName is often an order code ("83BY").
		h.Model = firstNonEmpty(read("SystemFamily"), read("SystemVersion"), read("SystemProductName"))
	}
	if h.Name == "" && h.Manufacturer == "" && h.Model == "" {
		return nil
	}
	return h
}

// tidyMaker turns the shouting some firmware does ("LENOVO") into a name
// ("Lenovo"). Mixed-case values are left as written, and so are short
// all-caps ones, which are usually initialisms ("HP", "MSI", "ASUS").
func tidyMaker(s string) string {
	if s == "" || strings.ToUpper(s) != s {
		return s
	}
	words := strings.Fields(s)
	for i, w := range words {
		runes := []rune(w)
		if len(runes) < 5 {
			continue
		}
		lower := []rune(strings.ToLower(w))
		lower[0] = unicode.ToUpper(lower[0])
		words[i] = string(lower)
	}
	return strings.Join(words, " ")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
