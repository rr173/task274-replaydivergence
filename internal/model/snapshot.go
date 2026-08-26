package model

import (
	"encoding/json"
	"time"
)

// SnapshotConfig 是定位快照固定的比较配置：范围、算法、检查点与不可信集合。
type SnapshotConfig struct {
	SeqMin            int64    `json:"seq_min"`
	SeqMax            int64    `json:"seq_max"`
	HashAlgo          string   `json:"hash_algo"`
	CheckpointSeqs    []int64  `json:"checkpoint_seqs"`
	UnreliableSeqs    []int64  `json:"unreliable_seqs,omitempty"`
	ExcludedSeqs      []int64  `json:"excluded_seqs,omitempty"`
}

// SnapshotSummary 是定位结果摘要：首次分歧指令与依赖链规模。
type SnapshotSummary struct {
	FirstDivergentSeq int64  `json:"first_divergent_seq"`
	FirstDivergentPC  string `json:"first_divergent_pc"`
	FirstDivergentOp  string `json:"first_divergent_op"`
	DivergenceCount   int    `json:"divergence_count"`
	ChainDepth        int    `json:"chain_depth"`
	RootCause         string `json:"root_cause"`
}

// LocSnapshot 是不可变的定位快照：固化比较配置与定位证据。
type LocSnapshot struct {
	ID          int64          `json:"id"`
	BatchID     int64          `json:"batch_id"`
	Name        string         `json:"name"`
	Status      SnapshotStatus `json:"status"`
	ConfigJSON  string         `json:"config_json"`
	SummaryJSON string         `json:"summary_json"`
	CreatedAt   time.Time      `json:"created_at"`
	PublishedAt *time.Time     `json:"published_at,omitempty"`
	SupersededBy int64         `json:"superseded_by,omitempty"`
}

// NewLocSnapshot 构造草稿快照，序列化配置。
func NewLocSnapshot(batchID int64, name string, cfg SnapshotConfig) (*LocSnapshot, error) {
	cfgJSON, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	return &LocSnapshot{
		BatchID:    batchID,
		Name:       name,
		Status:     SnapDraft,
		ConfigJSON: string(cfgJSON),
		CreatedAt:  time.Now().UTC(),
	}, nil
}

// SetSummary 写入定位结果摘要（发布前最后一次设置）。
func (s *LocSnapshot) SetSummary(sum SnapshotSummary) error {
	if s.Status != SnapDraft {
		return ErrInvalidState
	}
	b, err := json.Marshal(sum)
	if err != nil {
		return err
	}
	s.SummaryJSON = string(b)
	return nil
}

// Publish 发布快照：状态置为 published，记录发布时间。
func (s *LocSnapshot) Publish() error {
	if s.Status != SnapDraft {
		return ErrInvalidState
	}
	now := time.Now().UTC()
	s.Status = SnapPublished
	s.PublishedAt = &now
	return nil
}

// Supersede 把快照置为被替代（仅已发布快照可被替代）。
func (s *LocSnapshot) Supersede(byID int64) error {
	if s.Status != SnapPublished {
		return ErrInvalidState
	}
	s.Status = SnapSuperseded
	s.SupersededBy = byID
	return nil
}

// ParseConfig 解析快照配置。
func (s *LocSnapshot) ParseConfig() (SnapshotConfig, error) {
	var cfg SnapshotConfig
	err := json.Unmarshal([]byte(s.ConfigJSON), &cfg)
	return cfg, err
}

// ParseSummary 解析快照摘要。
func (s *LocSnapshot) ParseSummary() (SnapshotSummary, error) {
	var sum SnapshotSummary
	err := json.Unmarshal([]byte(s.SummaryJSON), &sum)
	return sum, err
}
