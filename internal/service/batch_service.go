package service

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"

	"task274-replaydivergence/internal/compare"
	"task274-replaydivergence/internal/model"
	"task274-replaydivergence/internal/sync"
	"task274-replaydivergence/internal/trace"
)

// syncResult 是对齐操作的结果视图。
type syncResult struct {
	Align *sync.AlignResult `json:"align"`
	Batch *model.ReplayBatch `json:"batch"`
}

// CreateBatch 创建接收中的重放批次。
func (s *Service) CreateBatch(name, ref, test, algo string) (*model.ReplayBatch, error) {
	if name == "" || ref == "" || test == "" {
		return nil, fmt.Errorf("name, ref and test trail names are required")
	}
	b := model.NewReplayBatch(name, ref, test, algo)
	if err := s.batches.Create(b); err != nil {
		return nil, err
	}
	return b, nil
}

// ImportEvents 导入一侧轨迹的事件片段（幂等：重复序号跳过）。
func (s *Service) ImportEvents(ctx context.Context, batchID int64, side model.TrailSide, inputs []trace.EventInput) (*trace.ValidationResult, error) {
	_ = ctx
	b, err := s.batches.Get(batchID)
	if err != nil {
		return nil, err
	}
	if b.IsSealed() {
		return nil, model.ErrSealedBatch
	}
	if !model.IsValidTrailSide(side) {
		return nil, fmt.Errorf("invalid trail side %q", side)
	}
	if b.Status != model.BatchReceiving && b.Status != model.BatchSyncing {
		return nil, model.ErrInvalidState
	}
	res, events, err := s.validator.Validate(batchID, side, inputs, model.EventRaw)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return res, nil
	}
	now := time.Now().UTC()
	for _, e := range events {
		e.CreatedAt = now
	}
	err = s.store.WithTx(func(tx *sql.Tx) error {
		return s.events.InsertBatch(tx, events)
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// AddCheckpoint 在指定指令序号处登记状态检查点。
func (s *Service) AddCheckpoint(batchID, seq int64, kind model.CheckpointKind, values map[string]string) (*model.Checkpoint, error) {
	b, err := s.batches.Get(batchID)
	if err != nil {
		return nil, err
	}
	if b.IsSealed() {
		return nil, model.ErrSealedBatch
	}
	if seq < 0 || kind == "" || len(values) == 0 {
		return nil, fmt.Errorf("checkpoint requires non-negative seq, kind and values")
	}
	c := &model.Checkpoint{
		BatchID:   batchID,
		Seq:       seq,
		Kind:      kind,
		Values:    values,
		Status:    "trusted",
		CreatedAt: time.Now().UTC(),
	}
	if err := s.checkpoints.Create(c); err != nil {
		return nil, err
	}
	return c, nil
}

// MarkCheckpointUnreliable 标记检查点不可信（比较时跳过该点）。
func (s *Service) MarkCheckpointUnreliable(batchID, cpID int64, reason string) (*model.Checkpoint, error) {
	b, err := s.batches.Get(batchID)
	if err != nil {
		return nil, err
	}
	if b.IsSealed() {
		return nil, model.ErrSealedBatch
	}
	c, err := s.checkpoints.Get(batchID, cpID)
	if err != nil {
		return nil, err
	}
	c.MarkUnreliable(reason)
	if err := s.checkpoints.UpdateStatus(c); err != nil {
		return nil, err
	}
	return c, nil
}

// Sync 对齐双轨迹并锁定比较范围，批次进入 syncing。
func (s *Service) Sync(batchID int64) (*syncResult, error) {
	b, err := s.batches.Get(batchID)
	if err != nil {
		return nil, err
	}
	if b.Status != model.BatchReceiving {
		return nil, model.ErrInvalidState
	}
	res, err := s.aligner.Align(batchID)
	if err != nil {
		return nil, err
	}
	if err := b.LockRange(res.SeqMin, res.SeqMax); err != nil {
		return nil, err
	}
	if err := b.Transition(model.BatchSyncing); err != nil {
		return nil, err
	}
	if err := s.batches.Update(b); err != nil {
		return nil, err
	}
	return &syncResult{Align: res, Batch: b}, nil
}

// ScanFingerprints 对每个可信检查点回放双侧状态并持久化指纹。
// 返回已扫描的检查点数量。
func (s *Service) ScanFingerprints(ctx context.Context, batchID int64) (int, error) {
	_ = ctx
	b, err := s.batches.Get(batchID)
	if err != nil {
		return 0, err
	}
	if b.Status != model.BatchSyncing && b.Status != model.BatchPendingLoc {
		return 0, model.ErrInvalidState
	}
	cps, err := s.checkpoints.ListByBatch(batchID)
	if err != nil {
		return 0, err
	}
	refEvents, err := s.events.ListBySide(batchID, model.TrailReference, "")
	if err != nil {
		return 0, err
	}
	testEvents, err := s.events.ListBySide(batchID, model.TrailUnderTest, "")
	if err != nil {
		return 0, err
	}
	sort.Slice(refEvents, func(i, j int) bool { return refEvents[i].Seq < refEvents[j].Seq })
	sort.Slice(testEvents, func(i, j int) bool { return testEvents[i].Seq < testEvents[j].Seq })

	fp := compare.NewFingerprinter(b.HashAlgo)
	replayer := compare.NewReplayer()
	refState := map[string]string{}
	testState := map[string]string{}
	count := 0
	for _, cp := range cps {
		if cp.IsUnreliable() {
			continue
		}
		if err := replayer.Replay(refState, eventsUpTo(refEvents, cp.Seq)); err != nil {
			return count, err
		}
		if err := replayer.Replay(testState, eventsUpTo(testEvents, cp.Seq)); err != nil {
			return count, err
		}
		refHash := fp.HashState(compare.SelectScope(refState, cp.Scope()))
		testHash := fp.HashState(compare.SelectScope(testState, cp.Scope()))
		now := time.Now().UTC()
		if err := s.fingerprints.Upsert(&model.StateFingerprint{
			BatchID: batchID, Side: model.TrailReference, Seq: cp.Seq,
			Scope: cp.Scope(), Hash: refHash, Algo: b.HashAlgo, CreatedAt: now,
		}); err != nil {
			return count, err
		}
		if err := s.fingerprints.Upsert(&model.StateFingerprint{
			BatchID: batchID, Side: model.TrailUnderTest, Seq: cp.Seq,
			Scope: cp.Scope(), Hash: testHash, Algo: b.HashAlgo, CreatedAt: now,
		}); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// ConfirmBatch 手动确认定位结果：pending_loc -> confirmed。
func (s *Service) ConfirmBatch(batchID int64) (*model.ReplayBatch, error) {
	b, err := s.batches.Get(batchID)
	if err != nil {
		return nil, err
	}
	if b.Status != model.BatchPendingLoc {
		return nil, model.ErrInvalidState
	}
	if err := b.Transition(model.BatchConfirmed); err != nil {
		return nil, err
	}
	if err := s.batches.Update(b); err != nil {
		return nil, err
	}
	return b, nil
}

// SealBatch 封存批次（不发布快照时的只读保护）：confirmed -> sealed。
func (s *Service) SealBatch(batchID int64) (*model.ReplayBatch, error) {
	b, err := s.batches.Get(batchID)
	if err != nil {
		return nil, err
	}
	if b.Status != model.BatchConfirmed {
		return nil, model.ErrInvalidState
	}
	if err := b.Transition(model.BatchSealed); err != nil {
		return nil, err
	}
	if err := s.batches.Update(b); err != nil {
		return nil, err
	}
	return b, nil
}

// eventsUpTo 返回 seq <= limit 的事件子集（events 必须按 seq 升序）。
func eventsUpTo(events []*model.ExecEvent, limit int64) []*model.ExecEvent {
	for i, e := range events {
		if e.Seq > limit {
			return events[:i]
		}
	}
	return events
}
