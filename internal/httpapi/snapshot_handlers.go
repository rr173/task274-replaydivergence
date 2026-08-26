package httpapi

import (
	"encoding/json"
	"net/http"
)

type createSnapshotReq struct {
	Name string `json:"name"`
}

// handleCreateSnapshot POST /api/batches/{id}/snapshots
func (s *Server) handleCreateSnapshot(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt64(r, "id")
	if err != nil {
		s.writeErr(w, err)
		return
	}
	var req createSnapshotReq
	_ = json.NewDecoder(r.Body).Decode(&req)
	snap, err := s.svc.CreateSnapshot(id, req.Name)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.writeJSON(w, http.StatusCreated, snap)
}

// handleListSnapshots GET /api/batches/{id}/snapshots
func (s *Server) handleListSnapshots(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt64(r, "id")
	if err != nil {
		s.writeErr(w, err)
		return
	}
	snaps, err := s.svc.Snapshots().ListByBatch(id)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, snaps)
}

// handlePublishSnapshot POST /api/batches/{id}/snapshots/{sid}/publish
func (s *Server) handlePublishSnapshot(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt64(r, "id")
	if err != nil {
		s.writeErr(w, err)
		return
	}
	sid, err := pathInt64(r, "sid")
	if err != nil {
		s.writeErr(w, err)
		return
	}
	snap, err := s.svc.PublishSnapshot(r.Context(), id, sid)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, snap)
}

// handleGetSnapshot GET /api/snapshots/{sid}
func (s *Server) handleGetSnapshot(w http.ResponseWriter, r *http.Request) {
	sid, err := pathInt64(r, "sid")
	if err != nil {
		s.writeErr(w, err)
		return
	}
	snap, err := s.svc.Snapshots().Get(sid)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.svc.OverlayLiveSummary(snap)
	type snapshotView struct {
		ID        int64  `json:"id"`
		BatchID   int64  `json:"batch_id"`
		Name      string `json:"name"`
		Status    string `json:"status"`
		Config    json.RawMessage `json:"config"`
		Summary   json.RawMessage `json:"summary,omitempty"`
		CreatedAt string `json:"created_at"`
	}
	view := snapshotView{
		ID:        snap.ID,
		BatchID:   snap.BatchID,
		Name:      snap.Name,
		Status:    string(snap.Status),
		Config:    json.RawMessage(snap.ConfigJSON),
		CreatedAt: snap.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
	if snap.SummaryJSON != "" {
		view.Summary = json.RawMessage(snap.SummaryJSON)
	}
	s.writeJSON(w, http.StatusOK, view)
}
