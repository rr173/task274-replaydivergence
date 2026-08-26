// Package service 编排领域流程：批次生命周期、轨迹导入、比较、回溯与快照发布。
package service

import (
	"database/sql"

	"task274-replaydivergence/internal/backtrace"
	"task274-replaydivergence/internal/compare"
	"task274-replaydivergence/internal/model"
	"task274-replaydivergence/internal/snapshot"
	"task274-replaydivergence/internal/store"
	"task274-replaydivergence/internal/sync"
	"task274-replaydivergence/internal/trace"
)

// Service 聚合全部仓储与业务组件，是 HTTP 层唯一入口。
type Service struct {
	db          *sql.DB
	store       *store.Store
	batches     *store.BatchStore
	events      *store.EventStore
	checkpoints *store.CheckpointStore
	fingerprints *store.FingerprintStore
	deps        *store.DependencyStore
	divergences *store.DivergenceStore
	snapshots   *store.SnapshotStore

	validator  *trace.Validator
	aligner    *sync.Aligner
	publisher  *snapshot.Publisher
}

// NewService 基于已打开的 Store 构造服务。
func NewService(st *store.Store) *Service {
	db := st.DB()
	s := &Service{
		db:          db,
		store:       st,
		batches:     store.NewBatchStore(db),
		events:      store.NewEventStore(db),
		checkpoints: store.NewCheckpointStore(db),
		fingerprints: store.NewFingerprintStore(db),
		deps:        store.NewDependencyStore(db),
		divergences: store.NewDivergenceStore(db),
		snapshots:   store.NewSnapshotStore(db),
		publisher:   snapshot.NewPublisher(),
	}
	s.validator = trace.NewValidator(s.events.SeqExists)
	s.aligner = sync.NewAligner(s.events)
	return s
}

// Store 暴露底层仓储。
func (s *Service) Store() *store.Store { return s.store }

// Events 暴露事件仓储。
func (s *Service) Events() *store.EventStore { return s.events }

// Batches 暴露批次仓储。
func (s *Service) Batches() *store.BatchStore { return s.batches }

// Checkpoints 暴露检查点仓储。
func (s *Service) Checkpoints() *store.CheckpointStore { return s.checkpoints }

// Fingerprints 暴露指纹仓储。
func (s *Service) Fingerprints() *store.FingerprintStore { return s.fingerprints }

// Divergences 暴露分歧仓储。
func (s *Service) Divergences() *store.DivergenceStore { return s.divergences }

// Dependencies 暴露依赖边仓储。
func (s *Service) Dependencies() *store.DependencyStore { return s.deps }

// Snapshots 暴露快照仓储。
func (s *Service) Snapshots() *store.SnapshotStore { return s.snapshots }

// Validator 暴露轨迹校验器。
func (s *Service) Validator() *trace.Validator { return s.validator }

// Aligner 暴露轨迹对齐器。
func (s *Service) Aligner() *sync.Aligner { return s.aligner }

// Comparator 构造指定指纹算法的比较器。
func (s *Service) Comparator(hashAlgo string) *compare.Comparator {
	return compare.NewComparator(compare.NewFingerprinter(hashAlgo))
}

// Backtracer 构造依赖回溯器。
func (s *Service) Backtracer(batchID int64, edges []*model.RWEdge, testEvents, refEvents []*model.ExecEvent) *backtrace.Tracer {
	return backtrace.NewTracer(batchID, edges, testEvents, refEvents)
}
