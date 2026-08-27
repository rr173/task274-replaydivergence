package service

import (
	"context"
	"path/filepath"
	"testing"

	"task274-replaydivergence/internal/model"
	"task274-replaydivergence/internal/store"
	"task274-replaydivergence/internal/trace"
)

func TestPartialListThenImportStillCompares(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "probe.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc := NewService(st)
	ctx := context.Background()
	b, err := svc.CreateBatch("cache", "ref", "test", "")
	if err != nil {
		t.Fatal(err)
	}
	head := []trace.EventInput{{
		Seq: 0, PC: "0x0", Opcode: "mov", Writes: []string{"reg:rax"}, Values: map[string]string{"reg:rax": "0x1"},
	}}
	if _, err := svc.ImportEvents(ctx, b.ID, model.TrailReference, head); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ImportEvents(ctx, b.ID, model.TrailUnderTest, head); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Events().ListBySide(b.ID, model.TrailReference, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Events().ListBySide(b.ID, model.TrailUnderTest, ""); err != nil {
		t.Fatal(err)
	}
	refTail := []trace.EventInput{
		{Seq: 1, PC: "0x4", Opcode: "add", Reads: []string{"reg:rax"}, Writes: []string{"reg:rbx"}, Values: map[string]string{"reg:rbx": "0x1"}},
		{Seq: 2, PC: "0x8", Opcode: "nop"},
	}
	testTail := []trace.EventInput{
		{Seq: 1, PC: "0x4", Opcode: "add", Reads: []string{"reg:rax"}, Writes: []string{"reg:rbx"}, Values: map[string]string{"reg:rbx": "0x2"}},
		{Seq: 2, PC: "0x8", Opcode: "nop"},
	}
	if _, err := svc.ImportEvents(ctx, b.ID, model.TrailReference, refTail); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ImportEvents(ctx, b.ID, model.TrailUnderTest, testTail); err != nil {
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
	out, err := svc.Compare(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if out.Divergent != 1 {
		t.Fatalf("want 1 divergence after tail import, got %d", out.Divergent)
	}
}
