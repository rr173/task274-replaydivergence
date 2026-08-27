package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"task274-replaydivergence/internal/model"
	"task274-replaydivergence/internal/store"
	"task274-replaydivergence/internal/trace"
)

// newTestService 打开一个临时 SQLite 库并构造 Service。
func newTestService(t *testing.T) (*Service, func()) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return NewService(st), func() { _ = st.Close() }
}

// transitionBatch 直接驱动批次状态机并落库（绕过 Sync/Compare 等需要
// 完整数据的流程），仅用于把批次推到目标状态做封存断言。
func transitionBatch(t *testing.T, svc *Service, id int64, to model.BatchStatus) {
	t.Helper()
	b, err := svc.Batches().Get(id)
	if err != nil {
		t.Fatalf("get batch: %v", err)
	}
	if err := b.Transition(to); err != nil {
		t.Fatalf("transition %s -> %s: %v", b.Status, to, err)
	}
	if err := svc.Batches().Update(b); err != nil {
		t.Fatalf("update batch: %v", err)
	}
}

// TestImportEventsOnSealedBatchPropagatesDomainError 验证修复：
// 批次封存后再导入轨迹，service 必须返回经 errors.Is 可识别的
// ErrSealedBatch（而非被 fmt %v 字符串化的内部错误），使接口层
// writeErr 能把它映射为 422，而非泄漏为 500。
func TestImportEventsOnSealedBatchPropagatesDomainError(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	b, err := svc.CreateBatch("b", "ref", "test", "")
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	refInputs := []trace.EventInput{
		{Seq: 0, Opcode: "mov", Writes: []string{"reg:rax"}, Values: map[string]string{"reg:rax": "0x1"}},
	}
	if _, err := svc.ImportEvents(context.Background(), b.ID, model.TrailReference, refInputs); err != nil {
		t.Fatalf("import ref: %v", err)
	}
	// receiving -> syncing -> pending_loc -> confirmed -> sealed。
	transitionBatch(t, svc, b.ID, model.BatchSyncing)
	transitionBatch(t, svc, b.ID, model.BatchPendingLoc)
	transitionBatch(t, svc, b.ID, model.BatchConfirmed)
	transitionBatch(t, svc, b.ID, model.BatchSealed)

	// 封存后导入：错误必须沿链携带 ErrSealedBatch。
	_, err = svc.ImportEvents(context.Background(), b.ID, model.TrailReference, refInputs)
	if err == nil {
		t.Fatal("expected error importing into sealed batch, got nil")
	}
	if !errors.Is(err, model.ErrSealedBatch) {
		t.Fatalf("error must wrap model.ErrSealedBatch so the HTTP layer maps it to 422, got: %v", err)
	}
}

// TestCreateSnapshotOnSealedBatchPropagatesDomainError 覆盖另一条曾用 %v 截断的链路：
// 封存批次上创建快照草稿同样应返回可被 errors.Is 识别的 ErrSealedBatch。
func TestCreateSnapshotOnSealedBatchPropagatesDomainError(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	b, err := svc.CreateBatch("b", "ref", "test", "")
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	transitionBatch(t, svc, b.ID, model.BatchSyncing)
	transitionBatch(t, svc, b.ID, model.BatchPendingLoc)
	transitionBatch(t, svc, b.ID, model.BatchConfirmed)
	transitionBatch(t, svc, b.ID, model.BatchSealed)

	_, err = svc.CreateSnapshot(b.ID, "snap")
	if err == nil {
		t.Fatal("expected error creating snapshot on sealed batch, got nil")
	}
	if !errors.Is(err, model.ErrSealedBatch) {
		t.Fatalf("error must wrap model.ErrSealedBatch, got: %v", err)
	}
}
