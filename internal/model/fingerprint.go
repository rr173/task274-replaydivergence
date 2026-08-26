package model

import "time"

// StateFingerprint 是某个指令序号后，指定侧轨迹的状态摘要哈希。
// 指纹由 compare 包按排序后的 (资源, 值) 对迭代计算，同批同算法可比。
type StateFingerprint struct {
	ID        int64            `json:"id"`
	BatchID   int64            `json:"batch_id"`
	Side      TrailSide        `json:"side"`
	Seq       int64            `json:"seq"`
	Scope     FingerprintScope `json:"scope"`
	Hash      string           `json:"hash"` // 十六进制哈希
	Algo      string           `json:"algo"`
	CreatedAt time.Time        `json:"created_at"`
}

// FingerprintPair 同一检查点两侧指纹的成对视图。
type FingerprintPair struct {
	Seq       int64            `json:"seq"`
	Scope     FingerprintScope `json:"scope"`
	RefHash   string           `json:"ref_hash"`
	TestHash  string           `json:"test_hash"`
	Matched   bool             `json:"matched"`
}

// Match 判断一对指纹是否一致。
func (p *FingerprintPair) Match() bool {
	return p.RefHash == p.TestHash && p.RefHash != ""
}
