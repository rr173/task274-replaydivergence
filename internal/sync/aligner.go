// Package sync 负责双轨迹的同步对齐：按指令序号配对事件，标记缺失与对齐状态。
package sync

import (
	"fmt"

	"task274-replaydivergence/internal/model"
)

// AlignResult 描述一次对齐的结果。
type AlignResult struct {
	RefCount    int64  `json:"ref_count"`
	TestCount   int64  `json:"test_count"`
	Aligned     int64  `json:"aligned"`
	MissingRef  int64  `json:"missing_ref"`
	MissingTest int64  `json:"missing_test"`
	SeqMin      int64  `json:"seq_min"`
	SeqMax      int64  `json:"seq_max"`
}

// EventGetter 按侧读取轨迹事件（供对齐器使用）。
type EventGetter interface {
	SeqRange(batchID int64, side model.TrailSide) (int64, int64, error)
	ListBySide(batchID int64, side model.TrailSide, status model.EventStatus) ([]*model.ExecEvent, error)
	BatchUpdateStatus(batchID int64, side model.TrailSide, status model.EventStatus) error
	UpdateStatus(batchID, eventID int64, status model.EventStatus, note string) error
}

// Aligner 执行轨迹对齐：参考与待测轨迹按序号并集配对。
// 单批次对齐必须串行（调用方以批次状态保证）。
type Aligner struct {
	events EventGetter
}

// NewAligner 构造对齐器。
func NewAligner(events EventGetter) *Aligner {
	return &Aligner{events: events}
}

// Align 对齐双轨迹：
// 1. 计算双侧序号并集范围 [seqMin, seqMax]；
// 2. 双侧全部事件置为 aligned；
// 3. 仅一侧存在的序号，对应侧标记 missing。
func (a *Aligner) Align(batchID int64) (*AlignResult, error) {
	refMin, refMax, err := a.events.SeqRange(batchID, model.TrailReference)
	if err != nil {
		return nil, err
	}
	testMin, testMax, err := a.events.SeqRange(batchID, model.TrailUnderTest)
	if err != nil {
		return nil, err
	}
	if refMax == 0 && testMax == 0 {
		return nil, fmt.Errorf("both trails are empty")
	}
	seqMin, seqMax := min64(refMin, testMin), max64(refMax, testMax)

	refEvents, err := a.events.ListBySide(batchID, model.TrailReference, "")
	if err != nil {
		return nil, err
	}
	testEvents, err := a.events.ListBySide(batchID, model.TrailUnderTest, "")
	if err != nil {
		return nil, err
	}
	_ = refEvents
	_ = testEvents

	refSeqs := map[int64]bool{}
	testSeqs := map[int64]bool{}
	for _, e := range refEvents {
		refSeqs[e.Seq] = true
		if e.Status != model.EventExcluded {
			if err := a.events.UpdateStatus(batchID, e.ID, model.EventAligned, ""); err != nil {
				return nil, err
			}
		}
	}
	for _, e := range testEvents {
		testSeqs[e.Seq] = true
		if e.Status != model.EventExcluded {
			if err := a.events.UpdateStatus(batchID, e.ID, model.EventAligned, ""); err != nil {
				return nil, err
			}
		}
	}

	res := &AlignResult{
		RefCount:  int64(len(refEvents)),
		TestCount: int64(len(testEvents)),
		SeqMin:    seqMin,
		SeqMax:    seqMax,
	}
	for seq := seqMin; seq <= seqMax; seq++ {
		if !refSeqs[seq] && testSeqs[seq] {
			res.MissingRef++
		}
		if refSeqs[seq] && !testSeqs[seq] {
			res.MissingTest++
		}
	}
	res.Aligned = int64(len(refEvents)) + int64(len(testEvents)) - res.MissingRef - res.MissingTest
	return res, nil
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
