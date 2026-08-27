package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"task274-replaydivergence/internal/model"
	"task274-replaydivergence/internal/service"
	"task274-replaydivergence/internal/store"
	"task274-replaydivergence/internal/trace"
)

func TestSealedImportMapsUnprocessable(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "probe.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc := service.NewService(st)
	ctx := context.Background()
	b, err := svc.CreateBatch("sealed", "ref", "test", "")
	if err != nil {
		t.Fatal(err)
	}
	ev := []trace.EventInput{
		{Seq: 0, PC: "0x0", Opcode: "mov", Writes: []string{"reg:rax"}, Values: map[string]string{"reg:rax": "0x1"}},
		{Seq: 1, PC: "0x4", Opcode: "add", Reads: []string{"reg:rax"}, Writes: []string{"reg:rbx"}, Values: map[string]string{"reg:rbx": "0x1"}},
		{Seq: 2, PC: "0x8", Opcode: "nop"},
	}
	test := []trace.EventInput{
		{Seq: 0, PC: "0x0", Opcode: "mov", Writes: []string{"reg:rax"}, Values: map[string]string{"reg:rax": "0x1"}},
		{Seq: 1, PC: "0x4", Opcode: "add", Reads: []string{"reg:rax"}, Writes: []string{"reg:rbx"}, Values: map[string]string{"reg:rbx": "0x2"}},
		{Seq: 2, PC: "0x8", Opcode: "nop"},
	}
	if _, err := svc.ImportEvents(ctx, b.ID, model.TrailReference, ev); err != nil {
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
		t.Fatalf("divergences: %v n=%d", err, len(divs))
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
	body, _ := json.Marshal(map[string]any{
		"side": "reference",
		"events": []map[string]any{{"seq": 9, "pc": "0x24", "opcode": "hlt"}},
	})
	req := httptest.NewRequest(http.MethodPost, fmtBatchEvents(b.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	NewServer(svc, log.New(io.Discard, "", 0)).Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("sealed import want 422, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func fmtBatchEvents(id int64) string {
	return "/api/batches/" + itoa(id) + "/events"
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
