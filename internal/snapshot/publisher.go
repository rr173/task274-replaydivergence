// Package snapshot 负责定位快照的草稿固化与发布：冻结比较配置与定位证据。
package snapshot

import (
	"sort"
	"time"

	"task274-replaydivergence/internal/model"
)

// ConfigAssembler 从批次与定位状态组装快照配置。
type ConfigAssembler struct {
	batch      *model.ReplayBatch
	checkpointSeqs []int64
	unreliable []int64
	excluded   []int64
}

// NewConfigAssembler 构造配置组装器。
func NewConfigAssembler(batch *model.ReplayBatch) *ConfigAssembler {
	return &ConfigAssembler{batch: batch}
}

// AddCheckpoint 登记可信检查点序号。
func (a *ConfigAssembler) AddCheckpoint(seq int64) *ConfigAssembler {
	a.checkpointSeqs = append(a.checkpointSeqs, seq)
	return a
}

// AddUnreliable 登记不可信检查点序号。
func (a *ConfigAssembler) AddUnreliable(seq int64) *ConfigAssembler {
	a.unreliable = append(a.unreliable, seq)
	return a
}

// AddExcluded 登记排除事件序号。
func (a *ConfigAssembler) AddExcluded(seq int64) *ConfigAssembler {
	a.excluded = append(a.excluded, seq)
	return a
}

// Build 生成不可变比较配置：范围、算法、检查点与排除集合均来自当前批次状态。
func (a *ConfigAssembler) Build() model.SnapshotConfig {
	sort.Slice(a.checkpointSeqs, func(i, j int) bool { return a.checkpointSeqs[i] < a.checkpointSeqs[j] })
	sort.Slice(a.unreliable, func(i, j int) bool { return a.unreliable[i] < a.unreliable[j] })
	sort.Slice(a.excluded, func(i, j int) bool { return a.excluded[i] < a.excluded[j] })
	return model.SnapshotConfig{
		SeqMin:         a.batch.SeqMin,
		SeqMax:         a.batch.SeqMax,
		HashAlgo:       a.batch.HashAlgo,
		CheckpointSeqs: a.checkpointSeqs,
		UnreliableSeqs: a.unreliable,
		ExcludedSeqs:   a.excluded,
	}
}

// Publisher 负责发布与替代快照，维护"同一批次最多一个已发布快照"的不变量。
type Publisher struct {
	now func() time.Time
}

// NewPublisher 构造发布器。
func NewPublisher() *Publisher {
	return &Publisher{now: time.Now}
}

// SupersedePrevious 把批次内已发布快照置为被替代（发布新快照前调用）。
func (p *Publisher) SupersedePrevious(existing []*model.LocSnapshot, newID int64) []*model.LocSnapshot {
	updated := []*model.LocSnapshot{}
	for _, s := range existing {
		if s.Status == model.SnapPublished {
			_ = s.Supersede(newID)
			updated = append(updated, s)
		}
	}
	return updated
}

// LiveSummary 从当前分歧组装摘要（不应覆盖已发布快照）。
func LiveSummary(divs []*model.Divergence) model.SnapshotSummary {
	sum := model.SnapshotSummary{DivergenceCount: len(divs)}
	for _, d := range divs {
		if d.FirstDivergentSeq != 0 {
			sum.FirstDivergentSeq = d.FirstDivergentSeq
			sum.FirstDivergentPC = d.FirstDivergentPC
			sum.FirstDivergentOp = d.FirstDivergentOp
			sum.RootCause = d.RootCause
		}
	}
	return sum
}
