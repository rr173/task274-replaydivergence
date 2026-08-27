package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"task274-replaydivergence/internal/model"
	"task274-replaydivergence/internal/service"
	"task274-replaydivergence/internal/store"
	"task274-replaydivergence/internal/trace"
)

func TestPublishedSnapshotSummaryFrozen(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "probe.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc := service.NewService(st)
	ctx := context.Background()
	b, err := svc.CreateBatch("freeze", "ref", "test", "")
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
		t.Fatalf("divergences: %v", err)
	}
	if _, err := svc.TraceDivergence(ctx, b.ID, divs[0].ID); err != nil {
		t.Fatal(err)
	}
	snap, err := svc.CreateSnapshot(b.ID, "loc")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PublishSnapshot(ctx, b.ID, snap.ID); err != nil {
		t.Fatal(err)
	}
	divs, err = svc.Divergences().ListByBatch(b.ID)
	if err != nil || len(divs) == 0 {
		t.Fatal(err)
	}
	divs[0].FirstDivergentSeq = 99
	divs[0].UpdatedAt = time.Now().UTC()
	if err := svc.Divergences().Update(divs[0]); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/snapshots/"+strconv.FormatInt(snap.ID, 10), nil)
	rec := httptest.NewRecorder()
	NewServer(svc, log.New(io.Discard, "", 0)).Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get snapshot want 200, got %d %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Summary struct {
			FirstDivergentSeq int64 `json:"first_divergent_seq"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Summary.FirstDivergentSeq != 1 {
		t.Fatalf("frozen summary want first_divergent_seq=1, got %d", payload.Summary.FirstDivergentSeq)
	}
}
