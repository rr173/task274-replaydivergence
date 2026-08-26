package model

import "time"

// RWEdge 是一条读写依赖边：待测轨迹中的读者指令依赖参考轨迹中的写者指令。
// 回溯首次分歧时，沿这类边从状态差异点向上游寻找根因指令。
type RWEdge struct {
	ID        int64  `json:"id"`
	BatchID   int64  `json:"batch_id"`
	Resource  string `json:"resource"` // reg:rax / mem:0x1000
	WriterSeq int64  `json:"writer_seq"`
	WriterPC  string `json:"writer_pc"`
	WriterOp  string `json:"writer_op"`
	ReaderSeq int64  `json:"reader_seq"`
	ReaderPC  string `json:"reader_pc"`
	ReaderOp  string `json:"reader_op"`
}

// Divergence 是一次状态差异的定位记录：从候选到确认首次分歧。
type Divergence struct {
	ID                 int64            `json:"id"`
	BatchID            int64            `json:"batch_id"`
	CheckpointSeq      int64            `json:"checkpoint_seq"` // 触发差异的检查点
	Status             DivergenceStatus `json:"status"`
	FirstDivergentSeq  int64            `json:"first_divergent_seq,omitempty"`
	FirstDivergentPC   string           `json:"first_divergent_pc,omitempty"`
	FirstDivergentOp   string           `json:"first_divergent_op,omitempty"`
	RootCause          string           `json:"root_cause,omitempty"`
	Evidence           string           `json:"evidence,omitempty"` // JSON 证据摘要
	CreatedAt          time.Time        `json:"created_at"`
	UpdatedAt          time.Time        `json:"updated_at"`
}

// ChainNode 是依赖链上的一个节点。
type ChainNode struct {
	Depth    int    `json:"depth"`
	Side     TrailSide `json:"side"`
	Seq      int64  `json:"seq"`
	PC       string `json:"pc"`
	Opcode   string `json:"opcode"`
	Resource string `json:"resource,omitempty"`
	Role     string `json:"role"` // writer | reader | checkpoint
}

// ChainEdge 持久化依赖链节点（按分歧记录分组）。
type ChainEdge struct {
	ID          int64  `json:"id"`
	DivergenceID int64 `json:"divergence_id"`
	Depth       int    `json:"depth"`
	Side        string `json:"side"`
	Seq         int64  `json:"seq"`
	PC          string `json:"pc"`
	Opcode      string `json:"opcode"`
	Resource    string `json:"resource,omitempty"`
	Role        string `json:"role"`
}
