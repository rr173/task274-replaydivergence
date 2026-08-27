package service_test

import (
	"context"
	"path/filepath"
	"testing"

	"task274-replaydivergence/internal/model"
	"task274-replaydivergence/internal/service"
	"task274-replaydivergence/internal/store"
	"task274-replaydivergence/internal/trace"
)

// TestPublishSnapshotSupersedesPrevious 验证同一批次发布第二份快照时，
// 旧已发布快照必须被替代：同一时刻只能有一份已发布快照。
func TestPublishSnapshotSupersedesPrevious(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "supersede.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	svc := service.NewService(st)

	b := mustSeedConfirmedBatch(t, svc)

	// 在 confirmed 阶段创建两份草稿快照，随后依次发布。
	snap1, err := svc.CreateSnapshot(b.ID, "snap-1")
	if err != nil {
		t.Fatalf("create snapshot 1: %v", err)
	}
	snap2, err := svc.CreateSnapshot(b.ID, "snap-2")
	if err != nil {
		t.Fatalf("create snapshot 2: %v", err)
	}

	// 发布第一份：批次应被封存，snap1 为 published。
	published1, err := svc.PublishSnapshot(context.Background(), b.ID, snap1.ID)
	if err != nil {
		t.Fatalf("publish snapshot 1: %v", err)
	}
	if published1.Status != model.SnapPublished {
		t.Fatalf("snap1 should be published, got %s", published1.Status)
	}

	// 发布第二份：snap1 必须被替代为 superseded 并指向 snap2。
	published2, err := svc.PublishSnapshot(context.Background(), b.ID, snap2.ID)
	if err != nil {
		t.Fatalf("publish snapshot 2: %v", err)
	}
	if published2.Status != model.SnapPublished {
		t.Fatalf("snap2 should be published, got %s", published2.Status)
	}
	if published2.ID != snap2.ID {
		t.Fatalf("expected snap2 to be the published one, got id=%d", published2.ID)
	}

	// 从持久层重新读取，确认旧快照确实落库为 superseded。
	relit1, err := svc.Snapshots().Get(snap1.ID)
	if err != nil {
		t.Fatalf("reload snap1: %v", err)
	}
	if relit1.Status != model.SnapSuperseded {
		t.Fatalf("snap1 should be superseded after publishing snap2, got %s", relit1.Status)
	}
	if relit1.SupersededBy != snap2.ID {
		t.Fatalf("snap1 should point to snap2 id=%d, got %d", snap2.ID, relit1.SupersededBy)
	}
	relit2, err := svc.Snapshots().Get(snap2.ID)
	if err != nil {
		t.Fatalf("reload snap2: %v", err)
	}
	if relit2.Status != model.SnapPublished {
		t.Fatalf("snap2 should remain published, got %s", relit2.Status)
	}
	if relit2.SupersededBy != 0 {
		t.Fatalf("snap2 should not be superseded, got by=%d", relit2.SupersededBy)
	}

	// 不变量：同一批次同一时刻最多一份已发布快照。
	list, err := svc.Snapshots().ListByBatch(b.ID)
	if err != nil {
		t.Fatalf("list snapshots: %v", err)
	}
	published := 0
	for _, s := range list {
		if s.Status == model.SnapPublished {
			published++
		}
	}
	if published != 1 {
		t.Fatalf("expected exactly 1 published snapshot, got %d", published)
	}
}

// mustSeedConfirmedBatch 走完导入→检查点→同步→指纹→比较→回溯流程，
// 使批次进入 confirmed 状态，满足创建草稿快照的前置条件。
func mustSeedConfirmedBatch(t *testing.T, svc *service.Service) *model.ReplayBatch {
	t.Helper()
	b, err := svc.CreateBatch("b", "ref", "test", "")
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	ref := []trace.EventInput{
		{Seq: 0, PC: "0x0", Opcode: "mov", Writes: []string{"reg:rax"}, Values: map[string]string{"reg:rax": "0x1"}},
		{Seq: 1, PC: "0x4", Opcode: "add", Reads: []string{"reg:rax"}, Writes: []string{"reg:rbx"}, Values: map[string]string{"reg:rbx": "0x1"}},
		{Seq: 2, PC: "0x8", Opcode: "store", Reads: []string{"reg:rbx"}, Writes: []string{"mem:0x1000"}, Values: map[string]string{"mem:0x1000": "0x1"}},
		{Seq: 3, PC: "0xc", Opcode: "load", Reads: []string{"mem:0x1000"}, Writes: []string{"reg:rcx"}, Values: map[string]string{"reg:rcx": "0x1"}},
		{Seq: 4, PC: "0x10", Opcode: "ret"},
	}
	test := []trace.EventInput{
		{Seq: 0, PC: "0x0", Opcode: "mov", Writes: []string{"reg:rax"}, Values: map[string]string{"reg:rax": "0x1"}},
		{Seq: 1, PC: "0x4", Opcode: "add", Reads: []string{"reg:rax"}, Writes: []string{"reg:rbx"}, Values: map[string]string{"reg:rbx": "0x2"}},
		{Seq: 2, PC: "0x8", Opcode: "store", Reads: []string{"reg:rbx"}, Writes: []string{"mem:0x1000"}, Values: map[string]string{"mem:0x1000": "0x2"}},
		{Seq: 3, PC: "0xc", Opcode: "load", Reads: []string{"mem:0x1000"}, Writes: []string{"reg:rcx"}, Values: map[string]string{"reg:rcx": "0x2"}},
		{Seq: 4, PC: "0x10", Opcode: "ret"},
	}
	if _, err := svc.ImportEvents(context.Background(), b.ID, model.TrailReference, ref); err != nil {
		t.Fatalf("import ref: %v", err)
	}
	if _, err := svc.ImportEvents(context.Background(), b.ID, model.TrailUnderTest, test); err != nil {
		t.Fatalf("import test: %v", err)
	}
	if _, err := svc.AddCheckpoint(b.ID, 4, model.CheckpointFull, map[string]string{
		"reg:rax": "0x1", "reg:rbx": "0x1", "reg:rcx": "0x1", "mem:0x1000": "0x1",
	}); err != nil {
		t.Fatalf("add checkpoint: %v", err)
	}
	if _, err := svc.Sync(b.ID); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if _, err := svc.ScanFingerprints(context.Background(), b.ID); err != nil {
		t.Fatalf("scan fingerprints: %v", err)
	}
	if _, err := svc.Compare(context.Background(), b.ID); err != nil {
		t.Fatalf("compare: %v", err)
	}
	divs, err := svc.Divergences().ListByBatch(b.ID)
	if err != nil || len(divs) != 1 {
		t.Fatalf("list divergences: n=%d err=%v", len(divs), err)
	}
	if _, err := svc.TraceDivergence(context.Background(), b.ID, divs[0].ID); err != nil {
		t.Fatalf("trace divergence: %v", err)
	}
	got, err := svc.Batches().Get(b.ID)
	if err != nil {
		t.Fatalf("reload batch: %v", err)
	}
	if got.Status != model.BatchConfirmed {
		t.Fatalf("batch should be confirmed, got %s", got.Status)
	}
	return got
}
