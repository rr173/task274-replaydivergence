package trace

import (
	"fmt"

	"task274-replaydivergence/internal/model"
)

// ValidationResult 记录一次导入校验的结果。
type ValidationResult struct {
	Accepted int              `json:"accepted"`
	Skipped  []SkippedEvent   `json:"skipped"`
	Warnings []string         `json:"warnings"`
}

// SkippedEvent 描述因冲突或非法被跳过的输入。
type SkippedEvent struct {
	Seq   int64  `json:"seq"`
	Opcode string `json:"opcode"`
	Reason string `json:"reason"`
}

// Validator 负责把输入片段转换为可入库事件，并执行幂等检查。
type Validator struct {
	// exists 由调用方注入：判断 (batch, side, seq) 是否已存在。
	exists func(batchID int64, side model.TrailSide, seq int64) (bool, error)
}

// NewValidator 构造校验器。
func NewValidator(exists func(batchID int64, side model.TrailSide, seq int64) (bool, error)) *Validator {
	return &Validator{exists: exists}
}

// Validate 转换输入并过滤冲突项。
// - 已存在的序号 → skipped（幂等，不报错）
// - 资源格式非法 → skipped + warning
// - 序号乱序 → skipped + warning（批次内要求单调）
func (v *Validator) Validate(batchID int64, side model.TrailSide, inputs []EventInput, status model.EventStatus) (*ValidationResult, []*model.ExecEvent, error) {
	res := &ValidationResult{}
	events := []*model.ExecEvent{}
	last := int64(-1)
	for _, in := range DedupeInputs(inputs) {
		if in.Seq <= last {
			res.Skipped = append(res.Skipped, SkippedEvent{Seq: in.Seq, Opcode: in.Opcode, Reason: "out_of_order"})
			res.Warnings = append(res.Warnings, fmt.Sprintf("seq %d out of order", in.Seq))
			continue
		}
		last = in.Seq
		exists, err := v.exists(batchID, side, in.Seq)
		if err != nil {
			return nil, nil, err
		}
		if exists {
			res.Skipped = append(res.Skipped, SkippedEvent{Seq: in.Seq, Opcode: in.Opcode, Reason: "duplicate"})
			continue
		}
		ev, err := in.ToModel(batchID, side, status)
		if err != nil {
			res.Skipped = append(res.Skipped, SkippedEvent{Seq: in.Seq, Opcode: in.Opcode, Reason: err.Error()})
			res.Warnings = append(res.Warnings, err.Error())
			continue
		}
		events = append(events, ev)
	}
	res.Accepted = len(events)
	return res, events, nil
}
