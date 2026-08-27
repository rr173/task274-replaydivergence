package service

import (
	"context"
	"errors"
	"testing"

	"task274-replaydivergence/internal/model"
	"task274-replaydivergence/internal/store"
	"task274-replaydivergence/internal/trace"
)

// newTestService 打开一个临时 SQLite Store 并构造 Service。
func newTestService(t *testing.T) *Service {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return NewService(st)
}

// setupSyncedBatchWithCheckpoint 构造一个已同步（syncing）且登记了检查点的批次，
// 但故意不扫描指纹，用于触发"未扫描即比较"场景。
func setupSyncedBatchWithCheckpoint(t *testing.T, svc *Service) *model.ReplayBatch {
	t.Helper()
	b, err := svc.CreateBatch("cmp", "ref", "test", "")
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	ref := []trace.EventInput{
		{Seq: 0, PC: "0x0", Opcode: "mov", Writes: []string{"reg:rax"}, Values: map[string]string{"reg:rax": "0x1"}},
		{Seq: 1, PC: "0x4", Opcode: "ret"},
	}
	test := []trace.EventInput{
		{Seq: 0, PC: "0x0", Opcode: "mov", Writes: []string{"reg:rax"}, Values: map[string]string{"reg:rax": "0x1"}},
		{Seq: 1, PC: "0x4", Opcode: "ret"},
	}
	if _, err := svc.ImportEvents(context.Background(), b.ID, model.TrailReference, ref); err != nil {
		t.Fatalf("import ref: %v", err)
	}
	if _, err := svc.ImportEvents(context.Background(), b.ID, model.TrailUnderTest, test); err != nil {
		t.Fatalf("import test: %v", err)
	}
	if _, err := svc.AddCheckpoint(b.ID, 1, model.CheckpointFull, map[string]string{"reg:rax": "0x1"}); err != nil {
		t.Fatalf("add checkpoint: %v", err)
	}
	if _, err := svc.Sync(b.ID); err != nil {
		t.Fatalf("sync: %v", err)
	}
	return b
}

// TestCompareWithoutFingerprintsDoesNotPanic 回归测试：
// 还没扫描指纹就调用 Compare 时，服务必须返回 ErrFingerprintMissing（可识别错误），
// 绝不能 panic。此前 GetPair 在指纹缺失时错误地返回 (nil, nil)，导致 Compare
// 解引用 nil pair 触发空指针崩溃。
func TestCompareWithoutFingerprintsDoesNotPanic(t *testing.T) {
	svc := newTestService(t)
	b := setupSyncedBatchWithCheckpoint(t, svc)

	// 不调用 ScanFingerprints，直接比较。
	out, err := svc.Compare(context.Background(), b.ID)
	if !errors.Is(err, model.ErrFingerprintMissing) {
		t.Fatalf("expected ErrFingerprintMissing, got out=%v err=%v", out, err)
	}
	if out != nil {
		t.Fatalf("expected nil outcome on error, got %+v", out)
	}
}
