package snapshot

import (
	"testing"

	"task274-replaydivergence/internal/model"
)

// TestSupersedePreviousRetiresOldPublished 确认同一批次发布新快照时，
// 旧的已发布快照被置为 superseded 并指向新快照 ID；草稿与已替代的快照保持原状。
func TestSupersedePreviousRetiresOldPublished(t *testing.T) {
	p := NewPublisher()
	batchID := int64(7)

	old, err := model.NewLocSnapshot(batchID, "snap-1", model.SnapshotConfig{SeqMin: 0, SeqMax: 4})
	if err != nil {
		t.Fatalf("new old snapshot: %v", err)
	}
	old.ID = 100
	if err := old.SetSummary(model.SnapshotSummary{FirstDivergentSeq: 1}); err != nil {
		t.Fatalf("set summary: %v", err)
	}
	if err := old.Publish(); err != nil {
		t.Fatalf("publish old: %v", err)
	}

	// 草稿不应被替代。
	draft, err := model.NewLocSnapshot(batchID, "snap-draft", model.SnapshotConfig{SeqMin: 0, SeqMax: 4})
	if err != nil {
		t.Fatalf("new draft snapshot: %v", err)
	}
	draft.ID = 101

	// 已被替代的快照也不应再次改动（状态机禁止）。
	ghost, err := model.NewLocSnapshot(batchID, "snap-ghost", model.SnapshotConfig{SeqMin: 0, SeqMax: 4})
	if err != nil {
		t.Fatalf("new ghost snapshot: %v", err)
	}
	ghost.ID = 102
	_ = ghost.SetSummary(model.SnapshotSummary{})
	_ = ghost.Publish()
	_ = ghost.Supersede(1)

	existing := []*model.LocSnapshot{old, draft, ghost}
	const newID int64 = 200
	updated := p.SupersedePrevious(existing, newID)

	if updated[0].Status != model.SnapSuperseded {
		t.Fatalf("old published snapshot should be superseded, got %s", updated[0].Status)
	}
	if updated[0].SupersededBy != newID {
		t.Fatalf("old snapshot should point to new id %d, got %d", newID, updated[0].SupersededBy)
	}
	if updated[1].Status != model.SnapDraft {
		t.Fatalf("draft snapshot should stay draft, got %s", updated[1].Status)
	}
	if updated[2].Status != model.SnapSuperseded || updated[2].SupersededBy != 1 {
		t.Fatalf("ghost snapshot should remain superseded by 1, got %s by=%d", updated[2].Status, updated[2].SupersededBy)
	}
	// 确认原 old 指针确被就地修改（事务持久化依赖此就地写回）。
	if old.Status != model.SnapSuperseded || old.SupersededBy != newID {
		t.Fatalf("old snapshot was not mutated in place: status=%s by=%d", old.Status, old.SupersededBy)
	}
}

// TestSupersedePreviousSkipsNewSnapshot 确认即将发布的新快照自身不被替代。
func TestSupersedePreviousSkipsNewSnapshot(t *testing.T) {
	p := NewPublisher()
	batchID := int64(1)
	snap, err := model.NewLocSnapshot(batchID, "s", model.SnapshotConfig{SeqMin: 0, SeqMax: 1})
	if err != nil {
		t.Fatalf("new snapshot: %v", err)
	}
	snap.ID = 5
	_ = snap.SetSummary(model.SnapshotSummary{})
	_ = snap.Publish()
	// existing 中仅含新快照自身，发布自身不应被替代。
	out := p.SupersedePrevious([]*model.LocSnapshot{snap}, snap.ID)
	if out[0].Status != model.SnapPublished {
		t.Fatalf("new snapshot should remain published, got %s", out[0].Status)
	}
	if out[0].SupersededBy != 0 {
		t.Fatalf("new snapshot should not be superseded, by=%d", out[0].SupersededBy)
	}
}

// TestSupersedePreviousNoOpOnEmpty 确认空输入与非法 newID 不崩溃。
func TestSupersedePreviousNoOpOnEmpty(t *testing.T) {
	p := NewPublisher()
	out := p.SupersedePrevious(nil, 0)
	if len(out) != 0 {
		t.Fatalf("expected empty slice, got %d", len(out))
	}
	// 含 nil 元素应跳过。
	out2 := p.SupersedePrevious([]*model.LocSnapshot{nil}, 1)
	if len(out2) != 1 || out2[0] != nil {
		t.Fatalf("nil element should be preserved untouched, got %v", out2)
	}
}
