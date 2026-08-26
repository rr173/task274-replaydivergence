package httpapi

import (
	"encoding/json"
	"net/http"
)

type createBatchReq struct {
	Name    string `json:"name"`
	Ref     string `json:"ref"`
	Test    string `json:"test"`
	HashAlgo string `json:"hash_algo"`
}

// handleCreateBatch POST /api/batches
func (s *Server) handleCreateBatch(w http.ResponseWriter, r *http.Request) {
	var req createBatchReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeErr(w, err)
		return
	}
	b, err := s.svc.CreateBatch(req.Name, req.Ref, req.Test, req.HashAlgo)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.writeJSON(w, http.StatusCreated, b)
}

// handleListBatches GET /api/batches
func (s *Server) handleListBatches(w http.ResponseWriter, r *http.Request) {
	batches, err := s.svc.Batches().List()
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, batches)
}

// handleGetBatch GET /api/batches/{id}
func (s *Server) handleGetBatch(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt64(r, "id")
	if err != nil {
		s.writeErr(w, err)
		return
	}
	b, err := s.svc.Batches().Get(id)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, b)
}

// handleSyncBatch POST /api/batches/{id}/sync
func (s *Server) handleSyncBatch(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt64(r, "id")
	if err != nil {
		s.writeErr(w, err)
		return
	}
	res, err := s.svc.Sync(id)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, res)
}

// handleConfirmBatch POST /api/batches/{id}/confirm
func (s *Server) handleConfirmBatch(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt64(r, "id")
	if err != nil {
		s.writeErr(w, err)
		return
	}
	b, err := s.svc.ConfirmBatch(id)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, b)
}

// handleSealBatch POST /api/batches/{id}/seal
func (s *Server) handleSealBatch(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt64(r, "id")
	if err != nil {
		s.writeErr(w, err)
		return
	}
	b, err := s.svc.SealBatch(id)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, b)
}

// handleBatchStats GET /api/batches/{id}/stats
func (s *Server) handleBatchStats(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt64(r, "id")
	if err != nil {
		s.writeErr(w, err)
		return
	}
	st, err := s.svc.Stats(id)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, st)
}
