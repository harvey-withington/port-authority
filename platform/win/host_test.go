//go:build windows

package win

import "testing"

func TestTidyMaker(t *testing.T) {
	for in, want := range map[string]string{"LENOVO": "Lenovo", "Dell Inc.": "Dell Inc.", "HP": "HP", "MSI": "MSI", "SAMSUNG ELECTRONICS CO., LTD.": "Samsung Electronics CO., LTD.", "": ""} {
		if got := tidyMaker(in); got != want {
			t.Errorf("tidyMaker(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHostInfoNamesThisMachine(t *testing.T) {
	h := hostInfo()
	if h == nil || h.Name == "" {
		t.Fatalf("hostInfo = %+v, want at least the machine name", h)
	}
}
