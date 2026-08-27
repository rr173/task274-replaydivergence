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

func TestCompareWithoutScanDoesNotPanic(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "probe.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc := NewService(st)
	ctx := context.Background()
	b, err := svc.CreateBatch("nilfp", "ref", "test", "")
	if err != nil {
		t.Fatal(err)
	}
	ev := []trace.EventInput{
		{Seq: 0, PC: "0x0", Opcode: "mov", Writes: []string{"reg:rax"}, Values: map[string]string{"reg:rax": "0x1"}},
		{Seq: 1, PC: "0x4", Opcode: "nop"},
	}
	if _, err := svc.ImportEvents(ctx, b.ID, model.TrailReference, ev); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ImportEvents(ctx, b.ID, model.TrailUnderTest, ev); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddCheckpoint(b.ID, 1, model.CheckpointFull, map[string]string{"reg:rax": "0x1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Sync(b.ID); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("compare panicked: %v", rec)
		}
	}()
	_, err = svc.Compare(ctx, b.ID)
	if err == nil {
		t.Fatal("compare without scan must fail")
	}
	if !errors.Is(err, model.ErrFingerprintMissing) {
		t.Fatalf("want ErrFingerprintMissing, got %v", err)
	}
	got, err := svc.Batches().Get(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != model.BatchSyncing {
		t.Fatalf("batch must stay syncing, got %s", got.Status)
	}
}
