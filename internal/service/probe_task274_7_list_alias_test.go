package service

import (
	"context"
	"path/filepath"
	"testing"

	"task274-replaydivergence/internal/model"
	"task274-replaydivergence/internal/store"
	"task274-replaydivergence/internal/trace"
)

func TestListBySideDoesNotAlias(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "probe.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc := NewService(st)
	ctx := context.Background()
	b, err := svc.CreateBatch("alias", "ref", "test", "")
	if err != nil {
		t.Fatal(err)
	}
	ref := []trace.EventInput{
		{Seq: 0, PC: "0x0", Opcode: "mov", Writes: []string{"reg:rax"}, Values: map[string]string{"reg:rax": "0x1"}},
		{Seq: 1, PC: "0x4", Opcode: "nop"},
	}
	test := []trace.EventInput{
		{Seq: 0, PC: "0x0", Opcode: "mov", Writes: []string{"reg:rax"}, Values: map[string]string{"reg:rax": "0x2"}},
		{Seq: 1, PC: "0x4", Opcode: "nop"},
	}
	if _, err := svc.ImportEvents(ctx, b.ID, model.TrailReference, ref); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ImportEvents(ctx, b.ID, model.TrailUnderTest, test); err != nil {
		t.Fatal(err)
	}
	first, err := svc.Events().ListBySide(b.ID, model.TrailReference, "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Events().ListBySide(b.ID, model.TrailUnderTest, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(first) == 0 || len(second) == 0 {
		t.Fatal("both lists must be non-empty")
	}
	if first[0].Side != model.TrailReference {
		t.Fatalf("first list side mutated to %s", first[0].Side)
	}
	if second[0].Side != model.TrailUnderTest {
		t.Fatalf("second list side=%s", second[0].Side)
	}
	if first[0].Values["reg:rax"] != "0x1" {
		t.Fatalf("reference rax overwritten to %q", first[0].Values["reg:rax"])
	}
}
