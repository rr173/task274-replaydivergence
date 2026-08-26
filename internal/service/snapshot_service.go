package service

import (
	"context"
	"database/sql"
	"time"

	"task274-replaydivergence/internal/model"
	"task274-replaydivergence/internal/snapshot"
)

// CreateSnapshot 为批次创建定位快照草稿，冻结当前比较配置。
func (s *Service) CreateSnapshot(batchID int64, name string) (*model.LocSnapshot, error) {
	b, err := s.batches.Get(batchID)
	if err != nil {
		return nil, err
	}
	if b.IsSealed() {
		return nil, model.ErrSealedBatch
	}
	if b.Status != model.BatchConfirmed && b.Status != model.BatchPendingLoc {
		return nil, model.ErrInvalidState
	}
	if name == "" {
		name = "snapshot-" + time.Now().UTC().Format("20060102T150405")
	}
	assembler := snapshot.NewConfigAssembler(b)
	cps, err := s.checkpoints.ListByBatch(batchID)
	if err != nil {
		return nil, err
	}
	for _, cp := range cps {
		if cp.IsUnreliable() {
			assembler.AddUnreliable(cp.Seq)
			continue
		}
		assembler.AddCheckpoint(cp.Seq)
	}
	excluded, err := s.events.ListBySide(batchID, model.TrailUnderTest, model.EventExcluded)
	if err != nil {
		return nil, err
	}
	for _, e := range excluded {
		assembler.AddExcluded(e.Seq)
	}
	snap, err := model.NewLocSnapshot(batchID, name, assembler.Build())
	if err != nil {
		return nil, err
	}
	if err := s.snapshots.Create(snap); err != nil {
		return nil, err
	}
	return snap, nil
}

// PublishSnapshot 发布快照：替代批次内旧已发布快照并封存批次。
// 同一批次任一时刻最多一个已发布快照。
func (s *Service) PublishSnapshot(ctx context.Context, batchID, snapID int64) (*model.LocSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b, err := s.batches.Get(batchID)
	if err != nil {
		return nil, err
	}
	snap, err := s.snapshots.Get(snapID)
	if err != nil {
		return nil, err
	}
	if snap.BatchID != batchID {
		return nil, model.ErrNotFound
	}
	if snap.Status != model.SnapDraft {
		return nil, model.ErrInvalidState
	}
	// 汇总定位证据：取最近确认的分歧。
	divs, err := s.divergences.ListByBatch(batchID)
	if err != nil {
		return nil, err
	}
	var confirmed *model.Divergence
	for _, d := range divs {
		if d.Status == model.DivConfirmed || d.Status == model.DivFirst {
			if confirmed == nil || d.ID > confirmed.ID {
				confirmed = d
			}
		}
	}
	sum := model.SnapshotSummary{DivergenceCount: len(divs)}
	if confirmed != nil {
		sum.FirstDivergentSeq = confirmed.FirstDivergentSeq
		sum.FirstDivergentPC = confirmed.FirstDivergentPC
		sum.FirstDivergentOp = confirmed.FirstDivergentOp
		sum.RootCause = confirmed.RootCause
		if depth, err := s.divergences.MaxDepth(confirmed.ID); err == nil {
			sum.ChainDepth = depth
		}
	}
	if err := snap.SetSummary(sum); err != nil {
		return nil, err
	}
	if err := snap.Publish(); err != nil {
		return nil, err
	}

	// 替代旧已发布快照 + 更新批次为封存，同一事务保证原子性。
	err = s.store.WithTx(func(tx *sql.Tx) error {
		existing, err := s.snapshots.ListByBatchTx(tx, batchID)
		if err != nil {
			return err
		}
		for _, old := range existing {
			if old.Status == model.SnapPublished && old.ID != snapID {
				_ = old.Supersede(snapID)
				if err := s.snapshots.UpdateTx(tx, old); err != nil {
					return err
				}
			}
		}
		if err := s.snapshots.UpdateTx(tx, snap); err != nil {
			return err
		}
		if b.Status != model.BatchSealed {
			if err := b.Transition(model.BatchSealed); err != nil {
				return err
			}
			if err := s.batches.UpdateTx(tx, b); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return snap, nil
}
