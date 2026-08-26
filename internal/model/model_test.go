package model

import (
	"errors"
	"testing"
)

func TestBatchStateMachine(t *testing.T) {
	b := NewReplayBatch("b", "ref", "test", "")
	// receiving -> syncing 合法。
	if err := b.Transition(BatchSyncing); err != nil {
		t.Fatalf("receiving->syncing should pass: %v", err)
	}
	// syncing -> sealed 非法。
	if err := b.Transition(BatchSealed); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("syncing->sealed should be rejected, got %v", err)
	}
	// syncing -> pending_loc -> confirmed -> sealed 完整链路。
	if err := b.Transition(BatchPendingLoc); err != nil {
		t.Fatalf("pending_loc: %v", err)
	}
	if err := b.Transition(BatchConfirmed); err != nil {
		t.Fatalf("confirmed: %v", err)
	}
	if err := b.Transition(BatchSealed); err != nil {
		t.Fatalf("sealed: %v", err)
	}
	if !b.IsSealed() {
		t.Fatal("batch should be sealed")
	}
}

func TestSealedBatchRejectsRangeLock(t *testing.T) {
	b := NewReplayBatch("b", "ref", "test", "")
	_ = b.Transition(BatchSyncing)
	_ = b.Transition(BatchPendingLoc)
	_ = b.Transition(BatchConfirmed)
	_ = b.Transition(BatchSealed)
	if err := b.LockRange(0, 10); !errors.Is(err, ErrSealedBatch) {
		t.Fatalf("sealed batch should reject range lock, got %v", err)
	}
}

func TestSnapshotLifecycle(t *testing.T) {
	snap, err := NewLocSnapshot(1, "s1", SnapshotConfig{SeqMin: 0, SeqMax: 4})
	if err != nil {
		t.Fatalf("new snapshot: %v", err)
	}
	if snap.Status != SnapDraft {
		t.Fatalf("expected draft, got %s", snap.Status)
	}
	if err := snap.SetSummary(SnapshotSummary{FirstDivergentSeq: 1, RootCause: "rbx" }); err != nil {
		t.Fatalf("set summary: %v", err)
	}
	if err := snap.Publish(); err != nil {
		t.Fatalf("publish: %v", err)
	}
	// 已发布快照不可再次发布。
	if err := snap.Publish(); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("double publish should fail, got %v", err)
	}
	if err := snap.Supersede(2); err != nil {
		t.Fatalf("supersede: %v", err)
	}
	if snap.Status != SnapSuperseded {
		t.Fatalf("expected superseded, got %s", snap.Status)
	}
}

func TestEventResourceKey(t *testing.T) {
	r := ResourceAccess{Kind: "reg", Name: "rax"}
	if r.Key() != "reg:rax" {
		t.Fatalf("unexpected key %q", r.Key())
	}
	m := ResourceAccess{Kind: "mem", Name: "0x1000"}
	if m.Key() != "mem:0x1000" {
		t.Fatalf("unexpected key %q", m.Key())
	}
}
