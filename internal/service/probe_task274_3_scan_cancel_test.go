package service

import (
	"context"
	"path/filepath"
	"testing"

	"task274-replaydivergence/internal/model"
	"task274-replaydivergence/internal/store"
	"task274-replaydivergence/internal/trace"
)

func TestCancelledScanAndImportDoNotWrite(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "probe.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc := NewService(st)
	ctx := context.Background()
	b, err := svc.CreateBatch("cancel", "ref", "test", "")
	if err != nil {
		t.Fatal(err)
	}
	ref := []trace.EventInput{
		{Seq: 0, PC: "0x0", Opcode: "mov", Writes: []string{"reg:rax"}, Values: map[string]string{"reg:rax": "0x1"}},
		{Seq: 1, PC: "0x4", Opcode: "add", Reads: []string{"reg:rax"}, Writes: []string{"reg:rbx"}, Values: map[string]string{"reg:rbx": "0x1"}},
	}
	test := []trace.EventInput{
		{Seq: 0, PC: "0x0", Opcode: "mov", Writes: []string{"reg:rax"}, Values: map[string]string{"reg:rax": "0x1"}},
		{Seq: 1, PC: "0x4", Opcode: "add", Reads: []string{"reg:rax"}, Writes: []string{"reg:rbx"}, Values: map[string]string{"reg:rbx": "0x2"}},
	}
	if _, err := svc.ImportEvents(ctx, b.ID, model.TrailReference, ref); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ImportEvents(ctx, b.ID, model.TrailUnderTest, test); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddCheckpoint(b.ID, 1, model.CheckpointFull, map[string]string{"reg:rax": "0x1", "reg:rbx": "0x1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Sync(b.ID); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := svc.ScanFingerprints(canceled, b.ID); err == nil {
		t.Fatal("cancelled scan must fail")
	}
	n, err := svc.Fingerprints().CountByBatch(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("cancelled scan must not persist fingerprints, got %d", n)
	}
	if _, err := svc.ImportEvents(canceled, b.ID, model.TrailReference, []trace.EventInput{{
		Seq: 9, PC: "0x24", Opcode: "hlt",
	}}); err == nil {
		t.Fatal("cancelled import must fail")
	}
	events, err := svc.Events().ListBySide(b.ID, model.TrailReference, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		if e.Seq == 9 {
			t.Fatal("cancelled import must not persist seq 9")
		}
	}
}
