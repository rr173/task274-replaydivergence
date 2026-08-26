package model

import (
	"encoding/json"
	"time"
)

// CheckpointKind 表示检查点覆盖的状态范围。
type CheckpointKind string

const (
	// CheckpointRegister 仅校验寄存器状态。
	CheckpointRegister CheckpointKind = "register"
	// CheckpointMemory 仅校验内存摘要。
	CheckpointMemory CheckpointKind = "memory"
	// CheckpointFull 校验完整状态。
	CheckpointFull CheckpointKind = "full"
)

// Checkpoint 是验证工程师给出的已知状态锚点：在指定指令序号后，
// 期望的寄存器/内存状态。双轨迹在同一检查点分别比对指纹。
type Checkpoint struct {
	ID         int64         `json:"id"`
	BatchID    int64         `json:"batch_id"`
	Seq        int64         `json:"seq"`        // 对应执行事件序号
	Kind       CheckpointKind `json:"kind"`
	Values     map[string]string `json:"values"` // 资源名 -> 期望值（如 rax -> 0x2a）
	Status     string        `json:"status"`     // trusted | unreliable
	Reason     string        `json:"reason,omitempty"`
	CreatedAt  time.Time     `json:"created_at"`
}

// ValuesJSON 序列化期望值字典。
func (c *Checkpoint) ValuesJSON() []byte {
	b, _ := json.Marshal(c.Values)
	return b
}

// IsUnreliable 判断检查点是否被标记为不可信。
func (c *Checkpoint) IsUnreliable() bool {
	return c.Status == "unreliable"
}

// Scope 返回检查点对应的指纹覆盖范围（值语义与 FingerprintScope 一致）。
func (c *Checkpoint) Scope() FingerprintScope {
	return FingerprintScope(c.Kind)
}

// MarkUnreliable 记录不可信标记与原因。
func (c *Checkpoint) MarkUnreliable(reason string) {
	c.Status = "unreliable"
	c.Reason = reason
}

// Trusted 恢复检查点为可信。
func (c *Checkpoint) Trusted() {
	c.Status = "trusted"
	c.Reason = ""
}
