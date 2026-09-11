package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"portauthority/core/kb"
	"portauthority/core/model"
)

// DockView is a dock as the API shows it: the entry as it would be
// written to a docks file, plus the layer it came from.
type DockView struct {
	kb.DockEntry
	Source kb.Source `json:"source"`
}

func dockView(d *kb.Dock) DockView {
	return DockView{DockEntry: d.Entry(), Source: d.Source}
}

// maxDockBody bounds a dock submission; a real entry is a few hundred bytes.
const maxDockBody = 64 << 10

// handleKBDocksList is GET /api/v1/kb/docks: every dock across the
// layers, and whether this service can take additions.
func (s *Service) handleKBDocksList(w http.ResponseWriter, r *http.Request) {
	docks := kb.Docks()
	views := make([]DockView, 0, len(docks))
	for _, d := range docks {
		views = append(views, dockView(d))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": SchemaVersion,
		"docks":          views,
		"local_dir":      kb.LocalDir(),
		"writable":       kb.LocalDir() != "",
	})
}

// handleKBDocksCreate is POST /api/v1/kb/docks: adds a dock to the local
// layer and re-reads the machine so the new box appears at once.
func (s *Service) handleKBDocksCreate(w http.ResponseWriter, r *http.Request) {
	var entry kb.DockEntry
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxDockBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&entry); err != nil {
		writeError(w, http.StatusBadRequest, "bad dock: "+err.Error())
		return
	}
	dock, err := kb.AddLocalDock(entry)
	if err != nil {
		writeError(w, kbErrorStatus(err), err.Error())
		return
	}
	s.refreshAfterKBChange(r.Context(), "dock added: "+dock.Name)
	writeJSON(w, http.StatusCreated, map[string]any{
		"schema_version": SchemaVersion,
		"dock":           dockView(dock),
	})
}

// handleKBDocksDelete is DELETE /api/v1/kb/docks/{id}: forgets one of the
// user's own docks. Shipped and shared entries cannot be deleted; a local
// entry with the same id would override them instead.
func (s *Service) handleKBDocksDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing dock id")
		return
	}
	if err := kb.RemoveLocalDock(id); err != nil {
		writeError(w, kbErrorStatus(err), err.Error())
		return
	}
	s.refreshAfterKBChange(r.Context(), "dock forgotten: "+id)
	writeJSON(w, http.StatusOK, map[string]any{"schema_version": SchemaVersion, "ok": true})
}

func kbErrorStatus(err error) int {
	switch {
	case errors.Is(err, kb.ErrNoLocalLayer):
		return http.StatusServiceUnavailable
	case errors.Is(err, kb.ErrNotLocal):
		return http.StatusNotFound
	}
	return http.StatusBadRequest
}

// refreshAfterKBChange re-reads the machine after the knowledge base
// changed and tells subscribers, since the picture changed without
// anything being plugged in. A failed refresh is logged rather than
// failing the write: the dock is saved either way.
func (s *Service) refreshAfterKBChange(ctx context.Context, detail string) {
	if err := s.RefreshAfterChange(ctx, detail); err != nil {
		s.logger.Printf("refresh after knowledge base change failed: %v", err)
	}
}

// kbChangeEvent is the notification a knowledge base change is reported
// as: the topology was re-read without a device changing.
func kbChangeEvent(detail string) model.TopologyEvent {
	return model.TopologyEvent{Kind: model.EventResnapshot, Detail: detail}
}
