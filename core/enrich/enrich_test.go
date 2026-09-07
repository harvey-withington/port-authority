package enrich

import (
	"strings"
	"testing"

	"portauthority/core/model"
)

func TestAnnotate(t *testing.T) {
	receiver := &model.Device{ID: "recv", VendorID: 0x046d, ProductID: 0xc52b}
	unknown := &model.Device{ID: "unk", VendorID: 0xfffe, ProductID: 0x0001}
	preset := &model.Device{ID: "pre", VendorID: 0x8087, ProductID: 0x0001, VendorName: "keep me"}
	root := &model.Device{
		ID: "root",
		Hub: &model.Hub{Ports: []model.Port{
			{Number: 1, Device: receiver},
			{Number: 2, Device: unknown},
			{Number: 3, Device: preset},
		}},
	}
	topo := &model.Topology{Controllers: []model.Controller{{ID: "c1", RootHub: root}}}

	Annotate(topo)

	if !strings.Contains(receiver.VendorName, "Logitech") {
		t.Errorf("receiver VendorName = %q, want Logitech", receiver.VendorName)
	}
	if !strings.Contains(receiver.ProductName, "Unifying Receiver") {
		t.Errorf("receiver ProductName = %q, want Unifying Receiver", receiver.ProductName)
	}
	if unknown.VendorName != "" || unknown.ProductName != "" {
		t.Errorf("unknown device annotated: %q / %q", unknown.VendorName, unknown.ProductName)
	}
	if preset.VendorName != "keep me" {
		t.Errorf("preset VendorName overwritten: %q", preset.VendorName)
	}
	if root.VendorName != "" {
		t.Errorf("zero-ID root hub annotated: %q", root.VendorName)
	}

	Annotate(nil) // must not panic
}

func TestAnnotateRefinesVagueClassFromKnowledgeBase(t *testing.T) {
	prompter := &model.Device{ID: "prompter", VendorID: 0x17e9, ProductID: 0xff1a, Class: model.ClassVendor}
	storage := &model.Device{ID: "ssd", VendorID: 0x1b1c, ProductID: 0x1a20, Class: model.ClassStorage}
	root := &model.Device{ID: "root", Class: model.ClassHub, Hub: &model.Hub{Ports: []model.Port{
		{Number: 1, Status: model.StatusConnected, Device: prompter},
		{Number: 2, Status: model.StatusConnected, Device: storage},
	}}}
	topo := &model.Topology{Controllers: []model.Controller{{ID: "c", RootHub: root}}}
	Annotate(topo)
	if prompter.Class != model.ClassDisplay {
		t.Errorf("DisplayLink device class = %s, want display", prompter.Class)
	}
	if storage.Class != model.ClassStorage {
		t.Errorf("descriptor-derived class must not be overridden, got %s", storage.Class)
	}
}
