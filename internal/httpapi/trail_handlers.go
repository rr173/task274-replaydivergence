package httpapi

import (
	"encoding/json"
	"net/http"

	"task274-replaydivergence/internal/model"
	"task274-replaydivergence/internal/trace"
)

type importEventsReq struct {
	Side   model.TrailSide     `json:"side"`
	Events []trace.EventInput  `json:"events"`
}

// handleImportEvents POST /api/batches/{id}/events
func (s *Server) handleImportEvents(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt64(r, "id")
	if err != nil {
		s.writeErr(w, err)
		return
	}
	var req importEventsReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeErr(w, err)
		return
	}
	res, err := s.svc.ImportEvents(r.Context(), id, req.Side, req.Events)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.writeJSON(w, http.StatusCreated, res)
}

// handleListEvents GET /api/batches/{id}/events?side=reference&status=aligned
func (s *Server) handleListEvents(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt64(r, "id")
	if err != nil {
		s.writeErr(w, err)
		return
	}
	side := model.TrailSide(r.URL.Query().Get("side"))
	if side == "" {
		side = model.TrailReference
	}
	if !model.IsValidTrailSide(side) {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid side"})
		return
	}
	events, err := s.svc.Events().ListBySide(id, side, model.EventStatus(r.URL.Query().Get("status")))
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, events)
}

type excludeEventReq struct {
	Note string `json:"note"`
}

// handleExcludeEvent PATCH /api/batches/{id}/events/{eid}/exclude
func (s *Server) handleExcludeEvent(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt64(r, "id")
	if err != nil {
		s.writeErr(w, err)
		return
	}
	eid, err := pathInt64(r, "eid")
	if err != nil {
		s.writeErr(w, err)
		return
	}
	var req excludeEventReq
	_ = json.NewDecoder(r.Body).Decode(&req)
	e, err := s.svc.ExcludeEvent(id, eid, req.Note)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, e)
}
