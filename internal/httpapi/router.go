// Package httpapi 提供 HTTP 路由与处理器，统一 /api 前缀与 JSON 错误映射。
package httpapi

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"task274-replaydivergence/internal/model"
	"task274-replaydivergence/internal/service"
)

// Server 是 HTTP 服务入口，持有编排服务与路由表。
type Server struct {
	svc     *service.Service
	mux     *http.ServeMux
	logger  *log.Logger
	started bool
}

// NewServer 构造 HTTP 服务器并注册全部路由。
func NewServer(svc *service.Service, logger *log.Logger) *Server {
	s := &Server{svc: svc, mux: http.NewServeMux(), logger: logger}
	s.routes()
	return s
}

// Handler 返回 http.Handler（供 http.Server 使用）。
func (s *Server) Handler() http.Handler {
	s.started = true
	return s.mux
}

// routes 注册全部 API 路由（/api 前缀）。
func (s *Server) routes() {
	// 批次
	s.mux.HandleFunc("POST /api/batches", s.handleCreateBatch)
	s.mux.HandleFunc("GET /api/batches", s.handleListBatches)
	s.mux.HandleFunc("GET /api/batches/{id}", s.handleGetBatch)
	s.mux.HandleFunc("POST /api/batches/{id}/sync", s.handleSyncBatch)
	s.mux.HandleFunc("POST /api/batches/{id}/confirm", s.handleConfirmBatch)
	s.mux.HandleFunc("POST /api/batches/{id}/seal", s.handleSealBatch)
	s.mux.HandleFunc("GET /api/batches/{id}/stats", s.handleBatchStats)
	// 轨迹事件
	s.mux.HandleFunc("POST /api/batches/{id}/events", s.handleImportEvents)
	s.mux.HandleFunc("GET /api/batches/{id}/events", s.handleListEvents)
	s.mux.HandleFunc("PATCH /api/batches/{id}/events/{eid}/exclude", s.handleExcludeEvent)
	// 检查点
	s.mux.HandleFunc("POST /api/batches/{id}/checkpoints", s.handleAddCheckpoint)
	s.mux.HandleFunc("GET /api/batches/{id}/checkpoints", s.handleListCheckpoints)
	s.mux.HandleFunc("POST /api/batches/{id}/checkpoints/{cid}/mark-unreliable", s.handleMarkUnreliable)
	// 指纹与比较
	s.mux.HandleFunc("POST /api/batches/{id}/fingerprints/scan", s.handleScanFingerprints)
	s.mux.HandleFunc("GET /api/batches/{id}/fingerprints", s.handleListFingerprints)
	s.mux.HandleFunc("POST /api/batches/{id}/compare", s.handleCompare)
	s.mux.HandleFunc("GET /api/batches/{id}/divergences", s.handleListDivergences)
	s.mux.HandleFunc("POST /api/batches/{id}/divergences/{did}/trace", s.handleTraceDivergence)
	s.mux.HandleFunc("GET /api/batches/{id}/divergences/{did}/chain", s.handleDivergenceChain)
	// 快照
	s.mux.HandleFunc("POST /api/batches/{id}/snapshots", s.handleCreateSnapshot)
	s.mux.HandleFunc("GET /api/batches/{id}/snapshots", s.handleListSnapshots)
	s.mux.HandleFunc("POST /api/batches/{id}/snapshots/{sid}/publish", s.handlePublishSnapshot)
	s.mux.HandleFunc("GET /api/snapshots/{sid}", s.handleGetSnapshot)
	// 系统
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("GET /api/selfcheck", s.handleSelfCheck)
}

// writeJSON 写出 JSON 响应。
func (s *Server) writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		s.logger.Printf("encode response: %v", err)
	}
}

// writeErr 把领域错误映射为 HTTP 状态码。
func (s *Server) writeErr(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, model.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, model.ErrConflict), errors.Is(err, model.ErrFingerprintMissing):
		status = http.StatusConflict
	case errors.Is(err, model.ErrInvalidState), errors.Is(err, model.ErrSealedBatch),
		errors.Is(err, model.ErrRangeLocked), errors.Is(err, model.ErrDependencyCycle):
		status = http.StatusUnprocessableEntity
	}
	s.writeJSON(w, status, map[string]string{"error": err.Error()})
}

// pathInt64 解析路径参数为 int64；失败返回 0 与错误。
func pathInt64(r *http.Request, key string) (int64, error) {
	return strconv.ParseInt(r.PathValue(key), 10, 64)
}

// pathStr 读取路径参数。
func pathStr(r *http.Request, key string) string {
	return r.PathValue(key)
}
