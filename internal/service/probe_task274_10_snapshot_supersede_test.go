package service

import (
	"context"
	"path/filepath"
	"testing"

	"task274-replaydivergence/internal/model"
	"task274-replaydivergence/internal/store"
	"task274-replaydivergence/internal/trace"
)

func TestPublishSupersedesPrevious(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "probe.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc := NewService(st)
	ctx := context.Background()
	b, err := svc.CreateBatch("two-snap", "ref", "test", "")
	if err != nil {
		t.Fatal(err)
	}
	ref := []trace.EventInput{
		{Seq: 0, PC: "0x0", Opcode: "mov", Writes: []string{"reg:rax"}, Values: map[string]string{"reg:rax": "0x1"}},
		{Seq: 1, PC: "0x4", Opcode: "add", Reads: []string{"reg:rax"}, Writes: []string{"reg:rbx"}, Values: map[string]string{"reg:rbx": "0x1"}},
		{Seq: 2, PC: "0x8", Opcode: "nop"},
	}
	test := []trace.EventInput{
		{Seq: 0, PC: "0x0", Opcode: "mov", Writes: []string{"reg:rax"}, Values: map[string]string{"reg:rax": "0x1"}},
		{Seq: 1, PC: "0x4", Opcode: "add", Reads: []string{"reg:rax"}, Writes: []string{"reg:rbx"}, Values: map[string]string{"reg:rbx": "0x2"}},
		{Seq: 2, PC: "0x8", Opcode: "nop"},
	}
	if _, err := svc.ImportEvents(ctx, b.ID, model.TrailReference, ref); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ImportEvents(ctx, b.ID, model.TrailUnderTest, test); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddCheckpoint(b.ID, 2, model.CheckpointFull, map[string]string{"reg:rax": "0x1", "reg:rbx": "0x1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Sync(b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ScanFingerprints(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Compare(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	divs, err := svc.Divergences().ListByBatch(b.ID)
	if err != nil || len(divs) == 0 {
		t.Fatal(err)
	}
	if _, err := svc.TraceDivergence(ctx, b.ID, divs[0].ID); err != nil {
		t.Fatal(err)
	}
	s1, err := svc.CreateSnapshot(b.ID, "first")
	if err != nil {
		t.Fatal(err)
	}
	s2, err := svc.CreateSnapshot(b.ID, "second")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PublishSnapshot(ctx, b.ID, s1.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PublishSnapshot(ctx, b.ID, s2.ID); err != nil {
		t.Fatal(err)
	}
	all, err := svc.Snapshots().ListByBatch(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	published := 0
	var firstStatus model.SnapshotStatus
	for _, s := range all {
		if s.ID == s1.ID {
			firstStatus = s.Status
		}
		if s.Status == model.SnapPublished {
			published++
			if s.ID != s2.ID {
				t.Fatalf("published snapshot want %d, got %d", s2.ID, s.ID)
			}
		}
	}
	if firstStatus != model.SnapSuperseded {
		t.Fatalf("first snapshot want superseded, got %s", firstStatus)
	}
	if published != 1 {
		t.Fatalf("want exactly 1 published snapshot, got %d", published)
	}
}
