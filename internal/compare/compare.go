package compare

import (
	"fmt"
	"sort"

	"task274-replaydivergence/internal/model"
)

// CheckpointState 是某个检查点处某侧轨迹的完整状态。
type CheckpointState struct {
	CheckpointSeq int64
	State         map[string]string
}

// Comparator 比较双轨迹在检查点处的状态一致性。
// 流程：对每个可信检查点，分别回放参考/待测轨迹重建状态，
// 计算指纹并比对；不一致的检查点生成候选分歧。
type Comparator struct {
	fp *Fingerprinter
}

// NewComparator 构造比较器。
func NewComparator(fp *Fingerprinter) *Comparator {
	return &Comparator{fp: fp}
}

// CompareResult 是一次比较的结果。
type CompareResult struct {
	Scanned      int                 `json:"scanned"`      // 检查点总数
	Matched      int                 `json:"matched"`      // 指纹一致数
	DivergentCPs []DivergentCheckpoint `json:"divergent_cps"`
}

// DivergentCheckpoint 描述一个不一致的检查点。
type DivergentCheckpoint struct {
	Seq       int64  `json:"seq"`
	Kind      string `json:"kind"`
	RefHash   string `json:"ref_hash"`
	TestHash  string `json:"test_hash"`
	DiffCount int    `json:"diff_count"`
}

// DiffState 返回两个状态映射的差异键（排序）。
func DiffState(ref, test map[string]string) []string {
	keys := map[string]bool{}
	for k := range ref {
		keys[k] = true
	}
	for k := range test {
		keys[k] = true
	}
	diff := []string{}
	for k := range keys {
		if ref[k] != test[k] {
			diff = append(diff, k)
		}
	}
	sort.Strings(diff)
	return diff
}

// CompareAtCheckpoint 比较单个检查点：回放双侧状态、计算指纹、判断是否一致。
// refEvents / testEvents 必须按 seq 升序。
func (c *Comparator) CompareAtCheckpoint(cp *model.Checkpoint, refEvents, testEvents []*model.ExecEvent) (*DivergentCheckpoint, error) {
	refState := map[string]string{}
	testState := map[string]string{}
	if err := NewReplayer().Replay(refState, refEvents); err != nil {
		return nil, err
	}
	if err := NewReplayer().Replay(testState, testEvents); err != nil {
		return nil, err
	}
	refSub := SelectScope(refState, cp.Scope())
	testSub := SelectScope(testState, cp.Scope())
	refHash := c.fp.HashState(refSub)
	testHash := c.fp.HashState(testSub)
	dc := &DivergentCheckpoint{
		Seq:      cp.Seq,
		Kind:     string(cp.Kind),
		RefHash:  refHash,
		TestHash: testHash,
	}
	if refHash == testHash {
		return dc, nil
	}
	diff := DiffState(refSub, testSub)
	dc.DiffCount = len(diff)
	_ = diff
	return dc, fmt.Errorf("fingerprint mismatch at checkpoint seq=%d", cp.Seq)
}
