package kb

import (
	"strings"
	"testing"
)

func TestVendorName(t *testing.T) {
	cases := []struct {
		vid  uint16
		want string // distinctive substring of the upstream name
	}{
		{0x046d, "Logitech"},
		{0x8087, "Intel"},
		{0x2188, "CalDigit"}, // upstream usb.ids says "No brand"; fixed in data/overrides.json
		{0x0451, "Texas Instruments"},
	}
	for _, c := range cases {
		got := VendorName(c.vid)
		if !strings.Contains(got, c.want) {
			t.Errorf("VendorName(%#04x) = %q, want substring %q", c.vid, got, c.want)
		}
	}
}

func TestProductName(t *testing.T) {
	cases := []struct {
		vid, pid uint16
		want     string
	}{
		{0x046d, 0xc52b, "Unifying Receiver"},
		{0x2188, 0x4042, "CalDigit"},
	}
	for _, c := range cases {
		got := ProductName(c.vid, c.pid)
		if !strings.Contains(got, c.want) {
			t.Errorf("ProductName(%#04x, %#04x) = %q, want substring %q", c.vid, c.pid, got, c.want)
		}
	}
}

func TestUnknown(t *testing.T) {
	if got := VendorName(0xfffe); got != "" {
		t.Errorf("VendorName(0xfffe) = %q, want empty", got)
	}
	if got := ProductName(0x046d, 0x0000); got != "" {
		t.Errorf("ProductName(0x046d, 0x0000) = %q, want empty", got)
	}
	if got := ProductName(0xfffe, 0xc52b); got != "" {
		t.Errorf("ProductName(0xfffe, 0xc52b) = %q, want empty", got)
	}
}

func TestVersion(t *testing.T) {
	if Version() == "" {
		t.Fatal("Version() is empty")
	}
}

func TestClassSectionNotParsed(t *testing.T) {
	// "C 00" would parse as vendor 0xC000 if the class section leaked in;
	// usb.ids has no vendor at that ID.
	if got := VendorName(0xc000); got != "" {
		t.Errorf("VendorName(0xc000) = %q, class section leaked into vendors", got)
	}
}
