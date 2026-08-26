package httpapi

import (
	"net/http"
	"runtime"

	"task274-replaydivergence/internal/model"
)

// handleHealth GET /api/health
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
		"go":     runtime.Version(),
		"api":    "replaydivergence/v1",
	})
}

// SelfCheckReport 是自检报告：校验核心存储与统计可读。
type SelfCheckReport struct {
	Status      string `json:"status"`
	Batches     int    `json:"batches"`
	Events      int64  `json:"events"`
	Checkpoints int    `json:"checkpoints"`
	Fingerprints int64 `json:"fingerprints"`
	Divergences int    `json:"divergences"`
	Snapshots   int    `json:"snapshots"`
	Message     string `json:"message,omitempty"`
}

// handleSelfCheck GET /api/selfcheck
func (s *Server) handleSelfCheck(w http.ResponseWriter, r *http.Request) {
	report := &SelfCheckReport{Status: "ok"}
	batches, err := s.svc.Batches().List()
	if err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	report.Batches = len(batches)
	for _, b := range batches {
		for _, side := range model.ValidTrailSides() {
			byStatus, err := s.svc.Batches().CountEventsByStatus(b.ID, side)
			if err != nil {
				continue
			}
			for _, n := range byStatus {
				report.Events += n
			}
		}
	}
	s.writeJSON(w, http.StatusOK, report)
}
