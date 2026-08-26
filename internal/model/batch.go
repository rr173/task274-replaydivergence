package model

import "time"

// ReplayBatch 是一次处理器指令重放验证批次：装载双轨迹、检查点与比较配置。
type ReplayBatch struct {
	ID            int64       `json:"id"`
	Name          string      `json:"name"`
	RefTrailName  string      `json:"ref_trail_name"`
	TestTrailName string      `json:"test_trail_name"`
	Status        BatchStatus `json:"status"`
	SeqMin        int64       `json:"seq_min,omitempty"`  // 比较范围下限（指令序号）
	SeqMax        int64       `json:"seq_max,omitempty"`  // 比较范围上限
	HashAlgo      string      `json:"hash_algo"`          // 状态指纹算法标识
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
}

// NewReplayBatch 构造处于接收中的批次。hashAlgo 为空时使用默认算法。
func NewReplayBatch(name, ref, test, hashAlgo string) *ReplayBatch {
	if hashAlgo == "" {
		hashAlgo = "fnv1a-sorted"
	}
	now := time.Now().UTC()
	return &ReplayBatch{
		Name:          name,
		RefTrailName:  ref,
		TestTrailName: test,
		Status:        BatchReceiving,
		HashAlgo:      hashAlgo,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

// Transition 按状态机把批次流转到目标状态，非法流转返回 ErrInvalidState。
func (b *ReplayBatch) Transition(to BatchStatus) error {
	if !CanTransitionBatch(b.Status, to) {
		return ErrInvalidState
	}
	b.Status = to
	b.UpdatedAt = time.Now().UTC()
	return nil
}

// LockRange 在同步阶段固定比较范围（指令序号闭区间），已封存批次拒绝修改。
func (b *ReplayBatch) LockRange(seqMin, seqMax int64) error {
	if b.Status == BatchSealed {
		return ErrSealedBatch
	}
	if seqMin < 0 || seqMax < seqMin {
		return ErrConflict
	}
	b.SeqMin = seqMin
	b.SeqMax = seqMax
	b.UpdatedAt = time.Now().UTC()
	return nil
}

// IsSealed 判断批次是否已封存。
func (b *ReplayBatch) IsSealed() bool {
	return b.Status == BatchSealed
}
