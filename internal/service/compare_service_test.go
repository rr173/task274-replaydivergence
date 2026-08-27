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

// newTestService 构造一个临时库上的服务并返回清理函数。
func newTestService(t *testing.T) (*Service, func()) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return NewService(st), func() { _ = st.Close() }
}

// seedSyncedBatch 建批次、导入双轨迹、登记检查点、同步至 syncing，但不扫描指纹。
func seedSyncedBatch(t *testing.T, svc *Service) *model.ReplayBatch {
	t.Helper()
	b, err := svc.CreateBatch("t", "ref", "test", "")
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	ref := []trace.EventInput{
		{Seq: 0, PC: "0x0", Opcode: "mov", Writes: []string{"reg:rax"}, Values: map[string]string{"reg:rax": "0x1"}},
		{Seq: 1, PC: "0x4", Opcode: "add", Reads: []string{"reg:rax"}, Writes: []string{"reg:rbx"}, Values: map[string]string{"reg:rbx": "0x1"}},
	}
	test := []trace.EventInput{
		{Seq: 0, PC: "0x0", Opcode: "mov", Writes: []string{"reg:rax"}, Values: map[string]string{"reg:rax": "0x1"}},
		{Seq: 1, PC: "0x4", Opcode: "add", Reads: []string{"reg:rax"}, Writes: []string{"reg:rbx"}, Values: map[string]string{"reg:rbx": "0x1"}},
	}
	if _, err := svc.ImportEvents(context.Background(), b.ID, model.TrailReference, ref); err != nil {
		t.Fatalf("import ref: %v", err)
	}
	if _, err := svc.ImportEvents(context.Background(), b.ID, model.TrailUnderTest, test); err != nil {
		t.Fatalf("import test: %v", err)
	}
	if _, err := svc.AddCheckpoint(b.ID, 1, model.CheckpointFull, map[string]string{"reg:rbx": "0x1"}); err != nil {
		t.Fatalf("add checkpoint: %v", err)
	}
	if _, err := svc.Sync(b.ID); err != nil {
		t.Fatalf("sync: %v", err)
	}
	return b
}

// TestCompareMissingFingerprintReturnsConflict 验证未扫描指纹就比较时：
// 必须以 ErrFingerprintMissing（→ 冲突状态）返回，且批次不得离开 syncing。
func TestCompareMissingFingerprintReturnsConflict(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	b := seedSyncedBatch(t, svc) // 处于 syncing，但未调用 ScanFingerprints。

	_, err := svc.Compare(context.Background(), b.ID)
	if !errors.Is(err, model.ErrFingerprintMissing) {
		t.Fatalf("expected ErrFingerprintMissing, got %v", err)
	}

	// 批次必须仍处于 syncing，不得前进到 pending_loc。
	cur, err := svc.Batches().Get(b.ID)
	if err != nil {
		t.Fatalf("get batch: %v", err)
	}
	if cur.Status != model.BatchSyncing {
		t.Fatalf("batch must stay syncing, got %s", cur.Status)
	}
}

// TestCompareAfterScanSucceeds 验证正常路径：扫描后比较应成功推进 pending_loc。
func TestCompareAfterScanSucceeds(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	b := seedSyncedBatch(t, svc)
	if _, err := svc.ScanFingerprints(context.Background(), b.ID); err != nil {
		t.Fatalf("scan: %v", err)
	}
	out, err := svc.Compare(context.Background(), b.ID)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if out.Matched != 1 {
		t.Fatalf("expected 1 matched, got %d", out.Matched)
	}
	cur, err := svc.Batches().Get(b.ID)
	if err != nil {
		t.Fatalf("get batch: %v", err)
	}
	if cur.Status != model.BatchPendingLoc {
		t.Fatalf("batch should be pending_loc, got %s", cur.Status)
	}
}

// TestCompareNoTrustedCheckpointDoesNotAdvance 验证无可信检查点时也不静默前进批次。
func TestCompareNoTrustedCheckpointDoesNotAdvance(t *testing.T) {
	svc, cleanup := newTestService(t)
	defer cleanup()

	b := seedSyncedBatch(t, svc)
	// 把唯一的检查点标记为不可信，无可信检查点可比较。
	cps, err := svc.Checkpoints().ListByBatch(b.ID)
	if err != nil || len(cps) != 1 {
		t.Fatalf("list checkpoints: n=%d err=%v", len(cps), err)
	}
	if _, err := svc.MarkCheckpointUnreliable(b.ID, cps[0].ID, "flaky"); err != nil {
		t.Fatalf("mark unreliable: %v", err)
	}

	_, err = svc.Compare(context.Background(), b.ID)
	if !errors.Is(err, model.ErrFingerprintMissing) {
		t.Fatalf("expected ErrFingerprintMissing when nothing comparable, got %v", err)
	}
	cur, err := svc.Batches().Get(b.ID)
	if err != nil {
		t.Fatalf("get batch: %v", err)
	}
	if cur.Status != model.BatchSyncing {
		t.Fatalf("batch must stay syncing, got %s", cur.Status)
	}
}

