package httpapi

import (
	"encoding/json"
	"net/http"

	"task274-replaydivergence/internal/model"
)

type addCheckpointReq struct {
	Seq    int64             `json:"seq"`
	Kind   string            `json:"kind"`
	Values map[string]string `json:"values"`
}

// handleAddCheckpoint POST /api/batches/{id}/checkpoints
func (s *Server) handleAddCheckpoint(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt64(r, "id")
	if err != nil {
		s.writeErr(w, err)
		return
	}
	var req addCheckpointReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeErr(w, err)
		return
	}
	c, err := s.svc.AddCheckpoint(id, req.Seq, model.CheckpointKind(req.Kind), req.Values)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.writeJSON(w, http.StatusCreated, c)
}

// handleListCheckpoints GET /api/batches/{id}/checkpoints
func (s *Server) handleListCheckpoints(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt64(r, "id")
	if err != nil {
		s.writeErr(w, err)
		return
	}
	cps, err := s.svc.Checkpoints().ListByBatch(id)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, cps)
}

type markUnreliableReq struct {
	Reason string `json:"reason"`
}

// handleMarkUnreliable POST /api/batches/{id}/checkpoints/{cid}/mark-unreliable
func (s *Server) handleMarkUnreliable(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt64(r, "id")
	if err != nil {
		s.writeErr(w, err)
		return
	}
	cid, err := pathInt64(r, "cid")
	if err != nil {
		s.writeErr(w, err)
		return
	}
	var req markUnreliableReq
	_ = json.NewDecoder(r.Body).Decode(&req)
	c, err := s.svc.MarkCheckpointUnreliable(id, cid, req.Reason)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, c)
}

// handleScanFingerprints POST /api/batches/{id}/fingerprints/scan
func (s *Server) handleScanFingerprints(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt64(r, "id")
	if err != nil {
		s.writeErr(w, err)
		return
	}
	n, err := s.svc.ScanFingerprints(r.Context(), id)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]int{"scanned": n})
}

// handleListFingerprints GET /api/batches/{id}/fingerprints
func (s *Server) handleListFingerprints(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt64(r, "id")
	if err != nil {
		s.writeErr(w, err)
		return
	}
	fps, err := s.svc.Fingerprints().ListByBatch(id)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, fps)
}

// handleCompare POST /api/batches/{id}/compare
func (s *Server) handleCompare(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt64(r, "id")
	if err != nil {
		s.writeErr(w, err)
		return
	}
	out, err := s.svc.Compare(r.Context(), id)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	if out == nil {
		s.writeJSON(w, http.StatusOK, map[string]string{"error": "empty compare"})
		return
	}
	s.writeJSON(w, http.StatusOK, out)
}

// handleListDivergences GET /api/batches/{id}/divergences
func (s *Server) handleListDivergences(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt64(r, "id")
	if err != nil {
		s.writeErr(w, err)
		return
	}
	divs, err := s.svc.Divergences().ListByBatch(id)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, divs)
}

// handleTraceDivergence POST /api/batches/{id}/divergences/{did}/trace
func (s *Server) handleTraceDivergence(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt64(r, "id")
	if err != nil {
		s.writeErr(w, err)
		return
	}
	did, err := pathInt64(r, "did")
	if err != nil {
		s.writeErr(w, err)
		return
	}
	out, err := s.svc.TraceDivergence(r.Context(), id, did)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, out)
}

// handleDivergenceChain GET /api/batches/{id}/divergences/{did}/chain
func (s *Server) handleDivergenceChain(w http.ResponseWriter, r *http.Request) {
	id, err := pathInt64(r, "id")
	if err != nil {
		s.writeErr(w, err)
		return
	}
	did, err := pathInt64(r, "did")
	if err != nil {
		s.writeErr(w, err)
		return
	}
	if _, err := s.svc.Divergences().Get(id, did); err != nil {
		s.writeErr(w, err)
		return
	}
	chain, err := s.svc.Divergences().ListChain(did)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, chain)
}
