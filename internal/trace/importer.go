// Package trace 负责指令轨迹的导入与校验：解析事件片段、归一化资源、幂等检查。
package trace

import (
	"fmt"
	"sort"

	"task274-replaydivergence/internal/model"
)

// ImportRequest 是一次轨迹片段导入请求（HTTP 层反序列化目标）。
type ImportRequest struct {
	Side   model.TrailSide `json:"side"`
	Events []EventInput    `json:"events"`
}

// EventInput 是轨迹事件的外部输入形态。
type EventInput struct {
	Seq    int64             `json:"seq"`
	PC     string            `json:"pc"`
	Opcode string            `json:"opcode"`
	Stage  string            `json:"stage"`
	Reads  []string          `json:"reads,omitempty"`  // 形如 reg:rax / mem:0x1000
	Writes []string          `json:"writes,omitempty"` // 形如 reg:rax / mem:0x1000
	Values map[string]string `json:"values,omitempty"` // 完整资源 key(reg:rax / mem:0x1000) -> 写入值
}

// ToModel 把输入事件转换为领域事件并归一化资源。
func (in EventInput) ToModel(batchID int64, side model.TrailSide, status model.EventStatus) (*model.ExecEvent, error) {
	e := &model.ExecEvent{
		BatchID: batchID,
		Side:    side,
		Seq:     in.Seq,
		PC:      in.PC,
		Opcode:  in.Opcode,
		Stage:   in.Stage,
		Status:  status,
		Values:  in.Values,
	}
	if e.PC == "" {
		e.PC = fmt.Sprintf("0x%x", e.Seq*4)
	}
	if e.Opcode == "" {
		return nil, fmt.Errorf("event seq=%d missing opcode", e.Seq)
	}
	for _, r := range in.Reads {
		ra, err := parseResource(r)
		if err != nil {
			return nil, fmt.Errorf("event seq=%d bad read %q: %w", e.Seq, r, err)
		}
		e.Reads = append(e.Reads, ra)
	}
	for _, w := range in.Writes {
		ra, err := parseResource(w)
		if err != nil {
			return nil, fmt.Errorf("event seq=%d bad write %q: %w", e.Seq, w, err)
		}
		e.Writes = append(e.Writes, ra)
	}
	e.NormalizeReads()
	e.NormalizeWrites()
	return e, nil
}

// parseResource 解析 reg:rax / mem:0x1000 形态的资源描述。
func parseResource(s string) (model.ResourceAccess, error) {
	for i := 0; i < len(s); i++ {
		if s[i] == ':' {
			kind, name := s[:i], s[i+1:]
			if kind == "reg" || kind == "mem" {
				if name == "" {
					return model.ResourceAccess{}, fmt.Errorf("empty resource name")
				}
				return model.ResourceAccess{Kind: kind, Name: name}, nil
			}
		}
	}
	return model.ResourceAccess{}, fmt.Errorf("resource must be reg:<name> or mem:<addr>")
}

// ValidateSequence 校验片段内序号连续递增（允许跳号但禁止重复与乱序）。
func ValidateSequence(events []*model.ExecEvent) error {
	last := int64(-1)
	for _, e := range events {
		if e.Seq < 0 {
			return fmt.Errorf("seq %d must be non-negative", e.Seq)
		}
		if e.Seq <= last {
			return fmt.Errorf("duplicate or out-of-order seq %d after %d", e.Seq, last)
		}
		last = e.Seq
	}
	return nil
}

// DedupeInputs 按 seq 排序输入事件（导入前规范化）。
func DedupeInputs(in []EventInput) []EventInput {
	sort.Slice(in, func(i, j int) bool { return in[i].Seq < in[j].Seq })
	return in
}
