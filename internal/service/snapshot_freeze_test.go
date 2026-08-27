package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"task274-replaydivergence/internal/model"
	"task274-replaydivergence/internal/store"
	"task274-replaydivergence/internal/trace"
)

// TestPublishedSnapshotFreezesEvidence 断言：定位快照发布后，再修改库里的分歧记录，
// 接口读到的首次分歧序号仍保持发布当时的证据，不被实时分歧表覆盖。
func TestPublishedSnapshotFreezesEvidence(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "freeze.db")

	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	svc := NewService(st)

	b, err := svc.CreateBatch("freeze", "ref", "test", "")
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	refInputs := []trace.EventInput{
		{Seq: 0, PC: "0x0000", Opcode: "mov", Writes: []string{"reg:rax"}, Values: map[string]string{"reg:rax": "0x1"}},
		{Seq: 1, PC: "0x0004", Opcode: "add", Reads: []string{"reg:rax"}, Writes: []string{"reg:rbx"}, Values: map[string]string{"reg:rbx": "0x1"}},
		{Seq: 2, PC: "0x0008", Opcode: "store", Reads: []string{"reg:rbx"}, Writes: []string{"mem:0x1000"}, Values: map[string]string{"mem:0x1000": "0x1"}},
		{Seq: 3, PC: "0x000c", Opcode: "load", Reads: []string{"mem:0x1000"}, Writes: []string{"reg:rcx"}, Values: map[string]string{"reg:rcx": "0x1"}},
		{Seq: 4, PC: "0x0010", Opcode: "ret"},
	}
	testInputs := []trace.EventInput{
		{Seq: 0, PC: "0x0000", Opcode: "mov", Writes: []string{"reg:rax"}, Values: map[string]string{"reg:rax": "0x1"}},
		{Seq: 1, PC: "0x0004", Opcode: "add", Reads: []string{"reg:rax"}, Writes: []string{"reg:rbx"}, Values: map[string]string{"reg:rbx": "0x2"}},
		{Seq: 2, PC: "0x0008", Opcode: "store", Reads: []string{"reg:rbx"}, Writes: []string{"mem:0x1000"}, Values: map[string]string{"mem:0x1000": "0x2"}},
		{Seq: 3, PC: "0x000c", Opcode: "load", Reads: []string{"mem:0x1000"}, Writes: []string{"reg:rcx"}, Values: map[string]string{"reg:rcx": "0x2"}},
		{Seq: 4, PC: "0x0010", Opcode: "ret"},
	}
	ctx := context.Background()
	if _, err := svc.ImportEvents(ctx, b.ID, model.TrailReference, refInputs); err != nil {
		t.Fatalf("import ref: %v", err)
	}
	if _, err := svc.ImportEvents(ctx, b.ID, model.TrailUnderTest, testInputs); err != nil {
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
	if _, err := svc.ScanFingerprints(ctx, b.ID); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if _, err := svc.Compare(ctx, b.ID); err != nil {
		t.Fatalf("compare: %v", err)
	}
	divs, err := svc.Divergences().ListByBatch(b.ID)
	if err != nil || len(divs) != 1 {
		t.Fatalf("list divergences: n=%d err=%v", len(divs), err)
	}
	if _, err := svc.TraceDivergence(ctx, b.ID, divs[0].ID); err != nil {
		t.Fatalf("trace: %v", err)
	}

	// 发布快照——此时首次分歧序号 = 1。
	snap, err := svc.CreateSnapshot(b.ID, "freeze-snap")
	if err != nil {
		t.Fatalf("create snapshot: %v", err)
	}
	snap, err = svc.PublishSnapshot(ctx, b.ID, snap.ID)
	if err != nil {
		t.Fatalf("publish snapshot: %v", err)
	}
	publishedSeq := int64(1)
	sum, _ := snap.ParseSummary()
	if sum.FirstDivergentSeq != publishedSeq {
		t.Fatalf("published first divergent seq expected %d, got %d", publishedSeq, sum.FirstDivergentSeq)
	}

	// 发布后：把库里分歧记录的首次分歧序号改成别的值（模拟事后改动）。
	divs, _ = svc.Divergences().ListByBatch(b.ID)
	d := divs[0]
	d.FirstDivergentSeq = 999
	d.FirstDivergentPC = "0x0BAD"
	d.FirstDivergentOp = "poison"
	d.RootCause = "tampered"
	d.UpdatedAt = time.Now().UTC()
	if err := svc.Divergences().Update(d); err != nil {
		t.Fatalf("tamper divergence: %v", err)
	}

	// 重新读快照：OverlayLiveSummary 不应覆盖已发布快照的证据。
	got, err := svc.Snapshots().Get(snap.ID)
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}
	svc.OverlayLiveSummary(got)
	sum2, err := got.ParseSummary()
	if err != nil {
		t.Fatalf("parse summary: %v", err)
	}
	if sum2.FirstDivergentSeq != publishedSeq {
		t.Fatalf("PUBLISHED SNAPSHOT EVIDENCE LEAKED: expected frozen seq=%d, got %d",
			publishedSeq, sum2.FirstDivergentSeq)
	}
	if sum2.FirstDivergentOp == "poison" {
		t.Fatalf("PUBLISHED SNAPSHOT EVIDENCE OVERWRITTEN by live divergence table")
	}
}