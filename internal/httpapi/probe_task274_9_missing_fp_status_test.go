package httpapi

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"task274-replaydivergence/internal/model"
	"task274-replaydivergence/internal/service"
	"task274-replaydivergence/internal/store"
	"task274-replaydivergence/internal/trace"
)

func TestMissingFingerprintMapsConflict(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "probe.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc := service.NewService(st)
	ctx := context.Background()
	b, err := svc.CreateBatch("nofp", "ref", "test", "")
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
	req := httptest.NewRequest(http.MethodPost, "/api/batches/"+strconv.FormatInt(b.ID, 10)+"/compare", nil)
	rec := httptest.NewRecorder()
	NewServer(svc, log.New(io.Discard, "", 0)).Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("missing fingerprint want 409, got %d body=%s", rec.Code, rec.Body.String())
	}
	got, err := svc.Batches().Get(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != model.BatchSyncing {
		t.Fatalf("batch must stay syncing, got %s", got.Status)
	}
}
