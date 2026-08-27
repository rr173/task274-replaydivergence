package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"task274-replaydivergence/internal/model"
	"task274-replaydivergence/internal/service"
	"task274-replaydivergence/internal/store"
	"task274-replaydivergence/internal/trace"
)

// newTestServer 构造一个临时库上的 HTTP 服务并返回其 handler 与清理函数。
func newTestServer(t *testing.T) (http.Handler, *service.Service, func()) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "http.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	svc := service.NewService(st)
	srv := NewServer(svc, nil)
	return srv.Handler(), svc, func() { _ = st.Close() }
}

// seedSyncedBatch 导入双轨迹并同步至 syncing，但不扫描指纹。
func seedSyncedBatch(t *testing.T, svc *service.Service) *model.ReplayBatch {
	t.Helper()
	b, err := svc.CreateBatch("t", "ref", "test", "")
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}
	ref := []trace.EventInput{
		{Seq: 0, PC: "0x0", Opcode: "mov", Writes: []string{"reg:rax"}, Values: map[string]string{"reg:rax": "0x1"}},
		{Seq: 1, PC: "0x4", Opcode: "add", Reads: []string{"reg:rax"}, Writes: []string{"reg:rbx"}, Values: map[string]string{"reg:rbx": "0x1"}},
	}
	test := []trace.EventInput{
		{Seq: 0, PC: "0x0", Opcode: "mov", Writes: []string{"reg:rax"}, Values: map[string]string{"reg:rax": "0x1"}},
		{Seq: 1, PC: "0x4", Opcode: "add", Reads: []string{"reg:rax"}, Writes: []string{"reg:rbx"}, Values: map[string]string{"reg:rbx": "0x1"}},
	}
	if _, err := svc.ImportEvents(context.Background(), b.ID, model.TrailReference, ref); err != nil {
		t.Fatalf("import ref: %v", err)
	}
	if _, err := svc.ImportEvents(context.Background(), b.ID, model.TrailUnderTest, test); err != nil {
		t.Fatalf("import test: %v", err)
	}
	if _, err := svc.AddCheckpoint(b.ID, 1, model.CheckpointFull, map[string]string{"reg:rbx": "0x1"}); err != nil {
		t.Fatalf("add checkpoint: %v", err)
	}
	if _, err := svc.Sync(b.ID); err != nil {
		t.Fatalf("sync: %v", err)
	}
	return b
}

// TestCompareMissingFingerprintHTTPReturnsConflict 验证未扫描指纹就调比较接口时：
// 返回 409 Conflict（而非 500 内部错误），且批次仍处于 syncing。
func TestCompareMissingFingerprintHTTPReturnsConflict(t *testing.T) {
	handler, svc, cleanup := newTestServer(t)
	defer cleanup()

	b := seedSyncedBatch(t, svc) // syncing，未扫描指纹。

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/batches/"+strconv.FormatInt(b.ID, 10)+"/compare", strings.NewReader("{}"))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict, got %d (body=%s)", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if !strings.Contains(body["error"], "fingerprint missing") {
		t.Fatalf("expected fingerprint missing error, got %q", body["error"])
	}
	cur, err := svc.Batches().Get(b.ID)
	if err != nil {
		t.Fatalf("get batch: %v", err)
	}
	if cur.Status != model.BatchSyncing {
		t.Fatalf("batch must stay syncing, got %s", cur.Status)
	}
}

