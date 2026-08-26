package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"task274-replaydivergence/internal/backtrace"
	"task274-replaydivergence/internal/compare"
	"task274-replaydivergence/internal/model"
)

// CompareOutcome 是一次状态比较的结果视图。
type CompareOutcome struct {
	Scanned   int                       `json:"scanned"`
	Matched   int                       `json:"matched"`
	Divergent int                       `json:"divergent"`
	Batch     *model.ReplayBatch        `json:"batch"`
	Pairs     []*model.FingerprintPair  `json:"pairs"`
}

// Compare 比较各可信检查点处双轨迹指纹，为不一致点创建候选分歧，
// 批次进入 pending_loc。
func (s *Service) Compare(ctx context.Context, batchID int64) (*CompareOutcome, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b, err := s.batches.Get(batchID)
	if err != nil {
		return nil, err
	}
	if b.Status != model.BatchSyncing {
		return nil, model.ErrInvalidState
	}
	cps, err := s.checkpoints.ListByBatch(batchID)
	if err != nil {
		return nil, err
	}
	out := &CompareOutcome{Batch: b, Pairs: []*model.FingerprintPair{}}
	now := time.Now().UTC()
	for _, cp := range cps {
		if cp.IsUnreliable() {
			continue
		}
		pair, err := s.fingerprints.GetPair(batchID, cp.Seq, cp.Scope())
		if err != nil {
			if errors.Is(err, model.ErrFingerprintMissing) {
				continue // 未扫描的检查点跳过
			}
			return nil, err
		}
		out.Scanned++
		pair.Matched = pair.Match()
		out.Pairs = append(out.Pairs, pair)
		if !pair.Matched {
			out.Divergent++
			// 创建候选分歧（同一检查点只建一条）。
			existing, err := s.divergences.ListByBatch(batchID)
			if err != nil {
				return nil, err
			}
			dup := false
			for _, d := range existing {
				if d.CheckpointSeq == cp.Seq && d.Status == model.DivCandidate {
					dup = true
					break
				}
			}
			if dup {
				continue
			}
			if err := s.divergences.Create(&model.Divergence{
				BatchID:       batchID,
				CheckpointSeq: cp.Seq,
				Status:        model.DivCandidate,
				CreatedAt:     now,
				UpdatedAt:     now,
			}); err != nil {
				return nil, err
			}
		} else {
			out.Matched++
		}
	}
	if out.Scanned == 0 {
		return nil, model.ErrFingerprintMissing
	}
	if err := b.Transition(model.BatchPendingLoc); err != nil {
		return nil, err
	}
	if err := s.batches.Update(b); err != nil {
		return nil, err
	}
	out.Batch = b
	return out, nil
}

// TraceOutcome 是一次依赖回溯的完整结果（分歧 + 依赖链）。
type TraceOutcome struct {
	Divergence *model.Divergence  `json:"divergence"`
	Edges      int                `json:"edges"`
	Batch      *model.ReplayBatch `json:"batch"`
}

// TraceDivergence 沿读写依赖回溯首次分歧：
// 1. 重建依赖图并落库；2. 计算检查点处差异资源；3. 回溯根因指令；4. 固化链。
// 成功后批次进入 confirmed；证据不足则保持 pending_loc。
func (s *Service) TraceDivergence(ctx context.Context, batchID, divID int64) (*TraceOutcome, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b, err := s.batches.Get(batchID)
	if err != nil {
		return nil, err
	}
	if b.Status != model.BatchPendingLoc {
		return nil, model.ErrInvalidState
	}
	d, err := s.divergences.Get(batchID, divID)
	if err != nil {
		return nil, err
	}
	refEvents, err := s.events.ListBySide(batchID, model.TrailReference, "")
	if err != nil {
		return nil, err
	}
	testEvents, err := s.events.ListBySide(batchID, model.TrailUnderTest, "")
	if err != nil {
		return nil, err
	}
	sort.Slice(refEvents, func(i, j int) bool { return refEvents[i].Seq < refEvents[j].Seq })
	sort.Slice(testEvents, func(i, j int) bool { return testEvents[i].Seq < testEvents[j].Seq })

	// 1. 依赖图。
	gb := backtrace.NewGraphBuilder(batchID)
	edges, err := gb.Build(refEvents, testEvents)
	if err != nil {
		return nil, err
	}
	backtrace.SortEdges(edges)
	err = s.store.WithTx(func(tx *sql.Tx) error {
		return s.deps.ReplaceAll(tx, batchID, edges)
	})
	if err != nil {
		return nil, err
	}

	// 2. 差异资源：回放双侧状态到检查点，取 diff。
	replayer := compare.NewReplayer()
	refState := map[string]string{}
	testState := map[string]string{}
	if err := replayer.Replay(refState, eventsUpTo(refEvents, d.CheckpointSeq)); err != nil {
		return nil, err
	}
	if err := replayer.Replay(testState, eventsUpTo(testEvents, d.CheckpointSeq)); err != nil {
		return nil, err
	}
	diff := compare.DiffState(refState, testState)
	if len(diff) == 0 {
		d.Status = model.DivInsufficient
		d.Evidence = "no state difference at checkpoint"
		d.UpdatedAt = time.Now().UTC()
		if err := s.divergences.Update(d); err != nil {
			return nil, err
		}
		return &TraceOutcome{Divergence: d, Edges: len(edges), Batch: b}, nil
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// 3. 回溯。
	tracer := s.Backtracer(batchID, edges, testEvents, refEvents)
	tr := tracer.Trace(diff, d.CheckpointSeq, divID)

	d.FirstDivergentSeq = tr.FirstSeq
	d.FirstDivergentPC = tr.FirstPC
	d.FirstDivergentOp = tr.FirstOp
	d.RootCause = tr.RootCause
	d.Status = tr.Status
	d.UpdatedAt = time.Now().UTC()
	d.Evidence = fmt.Sprintf("diff_resources=%d first_seq=%d pc=%s op=%s",
		len(diff), tr.FirstSeq, tr.FirstPC, tr.FirstOp)

	// 4. 固化链。
	chainNodes := tr.ChainNodes
	for i, n := range chainNodes {
		chainNodes[i] = n
		chainNodes[i].DivergenceID = divID
	}
	err = s.store.WithTx(func(tx *sql.Tx) error {
		return s.divergences.SaveChain(tx, divID, chainNodes)
	})
	if err != nil {
		return nil, err
	}
	if err := s.divergences.Update(d); err != nil {
		return nil, err
	}

	// 5. 批次流转。
	if d.Status != model.DivInsufficient {
		if err := b.Transition(model.BatchConfirmed); err != nil {
			return nil, err
		}
		if err := s.batches.Update(b); err != nil {
			return nil, err
		}
	}
	return &TraceOutcome{Divergence: d, Edges: len(edges), Batch: b}, nil
}

// ExcludeEvent 把事件排除出比较范围（封存批次拒绝）。
func (s *Service) ExcludeEvent(batchID, eventID int64, note string) (*model.ExecEvent, error) {
	b, err := s.batches.Get(batchID)
	if err != nil {
		return nil, err
	}
	if b.IsSealed() {
		return nil, model.ErrSealedBatch
	}
	e, err := s.events.Get(batchID, eventID)
	if err != nil {
		return nil, err
	}
	if err := s.events.UpdateStatus(batchID, eventID, model.EventExcluded, note); err != nil {
		return nil, err
	}
	e.Status = model.EventExcluded
	e.Note = note
	return e, nil
}

// BatchStats 汇总批次统计。
type BatchStats struct {
	BatchID       int64                    `json:"batch_id"`
	BatchStatus   model.BatchStatus        `json:"batch_status"`
	RefEvents     int64                    `json:"ref_events"`
	TestEvents    int64                    `json:"test_events"`
	Checkpoints   int                      `json:"checkpoints"`
	Fingerprints  int64                    `json:"fingerprints"`
	Dependencies  int64                    `json:"dependencies"`
	Divergences   int                      `json:"divergences"`
	Snapshots     int                      `json:"snapshots"`
	EventStatus   map[string]int64         `json:"event_status"`
}

// Stats 返回批次统计（含各侧事件状态分布）。
func (s *Service) Stats(batchID int64) (*BatchStats, error) {
	b, err := s.batches.Get(batchID)
	if err != nil {
		return nil, err
	}
	st := &BatchStats{BatchID: batchID, BatchStatus: b.Status, EventStatus: map[string]int64{}}
	for _, side := range model.ValidTrailSides() {
		byStatus, err := s.batches.CountEventsByStatus(batchID, side)
		if err != nil {
			return nil, err
		}
		for status, n := range byStatus {
			key := string(side) + ":" + string(status)
			st.EventStatus[key] = n
			if side == model.TrailReference {
				st.RefEvents += n
			} else {
				st.TestEvents += n
			}
		}
	}
	cps, err := s.checkpoints.ListByBatch(batchID)
	if err != nil {
		return nil, err
	}
	st.Checkpoints = len(cps)
	fpN, err := s.fingerprints.CountByBatch(batchID)
	if err != nil {
		return nil, err
	}
	st.Fingerprints = fpN
	depN, err := s.deps.CountByBatch(batchID)
	if err != nil {
		return nil, err
	}
	st.Dependencies = depN
	divs, err := s.divergences.ListByBatch(batchID)
	if err != nil {
		return nil, err
	}
	st.Divergences = len(divs)
	snaps, err := s.snapshots.ListByBatch(batchID)
	if err != nil {
		return nil, err
	}
	st.Snapshots = len(snaps)
	return st, nil
}
