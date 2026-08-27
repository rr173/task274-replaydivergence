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

// SupersedePrevious 把批次内已发布的旧快照置为被替代（发布新快照前调用）。
// newID 指向即将发布的新快照，本身不会被替代；仅 SnapPublished 的旧快照会被替代，
// 草稿与已替代的快照保持原状。返回更新后的切片，调用方需在事务内持久化每个变动的快照。
func (p *Publisher) SupersedePrevious(existing []*model.LocSnapshot, newID int64) []*model.LocSnapshot {
	for _, snap := range existing {
		if snap == nil || snap.ID == newID {
			continue
		}
		if snap.Status != model.SnapPublished {
			continue
		}
		// 旧已发布快照只能成功替代或因状态非法跳过，这里静默跳过不影响新快照发布。
		_ = snap.Supersede(newID)
	}
	return existing
}
