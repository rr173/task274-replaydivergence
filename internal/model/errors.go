// Package model 定义处理器指令重放状态分歧定位服务的领域实体与错误类型。
package model

import "errors"

// 领域错误：所有错误在 service 层统一映射为 HTTP 状态码。
var (
	// ErrNotFound 表示目标实体不存在。
	ErrNotFound = errors.New("entity not found")
	// ErrConflict 表示唯一约束冲突（如指令序号重复导入、快照替代冲突）。
	ErrConflict = errors.New("conflict with existing entity")
	// ErrInvalidState 表示状态机流转不合法。
	ErrInvalidState = errors.New("invalid state transition")
	// ErrSealedBatch 表示对已封存批次执行修改操作。
	ErrSealedBatch = errors.New("batch is sealed, modification forbidden")
	// ErrFingerprintMissing 表示比较时缺少对应状态指纹。
	ErrFingerprintMissing = errors.New("state fingerprint missing")
	// ErrDependencyCycle 表示依赖图出现自环（指令读取自身写入的资源）。
	ErrDependencyCycle = errors.New("dependency cycle detected")
	// ErrRangeLocked 表示比较范围被不可信检查点排除后无法继续。
	ErrRangeLocked = errors.New("comparison range locked by unreliable checkpoint")
)

// BatchStatus 是重放批次生命周期状态机。
type BatchStatus string

const (
	// BatchReceiving 接收中：可导入轨迹事件与检查点。
	BatchReceiving BatchStatus = "receiving"
	// BatchSyncing 同步中：轨迹已对齐，等待比较。
	BatchSyncing BatchStatus = "syncing"
	// BatchPendingLoc 待定位：已完成比较，等待依赖回溯。
	BatchPendingLoc BatchStatus = "pending_loc"
	// BatchConfirmed 已确认：首次分歧已定位。
	BatchConfirmed BatchStatus = "confirmed"
	// BatchSealed 封存：只读，配置与证据不可再变。
	BatchSealed BatchStatus = "sealed"
)

// TrailSide 标识一条执行轨迹来自参考实现还是待测实现。
type TrailSide string

const (
	// TrailReference 参考轨迹（黄金模型输出）。
	TrailReference TrailSide = "reference"
	// TrailUnderTest 待测轨迹（被验证实现输出）。
	TrailUnderTest TrailSide = "under_test"
)

// ValidTrailSides 返回合法的轨迹侧集合。
func ValidTrailSides() []TrailSide {
	return []TrailSide{TrailReference, TrailUnderTest}
}

// IsValidTrailSide 判断轨迹侧是否合法。
func IsValidTrailSide(s TrailSide) bool {
	return s == TrailReference || s == TrailUnderTest
}

// EventStatus 是执行事件对齐状态。
type EventStatus string

const (
	// EventRaw 原始：已导入未参与对齐。
	EventRaw EventStatus = "raw"
	// EventAligned 对齐：双轨迹事件按序号配对成功。
	EventAligned EventStatus = "aligned"
	// EventDivergent 状态不同：该事件处参考与待测状态指纹不一致。
	EventDivergent EventStatus = "divergent"
	// EventMissing 缺失：一侧轨迹缺少对应序号的事件。
	EventMissing EventStatus = "missing"
	// EventExcluded 排除：被工程师手动排除出比较范围。
	EventExcluded EventStatus = "excluded"
)

// FingerprintScope 指纹覆盖范围。
type FingerprintScope string

const (
	// ScopeRegister 仅寄存器文件。
	ScopeRegister FingerprintScope = "register"
	// ScopeMemory 仅内存访问摘要。
	ScopeMemory FingerprintScope = "memory"
	// ScopeFull 寄存器与内存的完整状态。
	ScopeFull FingerprintScope = "full"
)

// DivergenceStatus 是分歧链判定状态机。
type DivergenceStatus string

const (
	// DivCandidate 候选：发现指纹差异但未回溯。
	DivCandidate DivergenceStatus = "candidate"
	// DivFirst 首次分歧：回溯确定最早不一致指令。
	DivFirst DivergenceStatus = "first_divergence"
	// DivPropagated 依赖传播：差异沿读写依赖链传播。
	DivPropagated DivergenceStatus = "dependency_propagated"
	// DivInsufficient 证据不足：无法确定根因指令。
	DivInsufficient DivergenceStatus = "insufficient_evidence"
	// DivConfirmed 确认：根因证据已固化。
	DivConfirmed DivergenceStatus = "confirmed"
)

// SnapshotStatus 是定位快照状态机。
type SnapshotStatus string

const (
	// SnapDraft 草稿：可修改。
	SnapDraft SnapshotStatus = "draft"
	// SnapPublished 发布：配置与证据不可变。
	SnapPublished SnapshotStatus = "published"
	// SnapSuperseded 替代：被更新的快照取代。
	SnapSuperseded SnapshotStatus = "superseded"
)

// 状态机合法流转表。
var batchTransitions = map[BatchStatus][]BatchStatus{
	BatchReceiving:  {BatchSyncing},
	BatchSyncing:    {BatchPendingLoc, BatchReceiving},
	BatchPendingLoc: {BatchConfirmed, BatchReceiving},
	BatchConfirmed:  {BatchSealed, BatchReceiving},
	BatchSealed:     {},
}

// CanTransitionBatch 校验批次状态机流转。
func CanTransitionBatch(from, to BatchStatus) bool {
	for _, next := range batchTransitions[from] {
		if next == to {
			return true
		}
	}
	return false
}
