package store

import (
	"database/sql"
	"testing"
	"time"

	"task274-replaydivergence/internal/model"
)

// newTestStore 打开一个内存 SQLite 并迁移建表，测试结束自动关闭。
func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// insertEvent 在事务中插入单条事件。
func insertEvent(t *testing.T, s *Store, e *model.ExecEvent) {
	t.Helper()
	if err := s.WithTx(func(tx *sql.Tx) error {
		return NewEventStore(s.DB()).InsertBatch(tx, []*model.ExecEvent{e})
	}); err != nil {
		t.Fatalf("insert event side=%s seq=%d: %v", e.Side, e.Seq, err)
	}
}

// TestListBySideIndependentAcrossQueries 复现并钉死一个回归缺陷：
// 连续两次查询参考/待测轨迹时，先返回的事件列表必须独立于后一次查询，
// 不允许两侧指令（切片与 Values 映射）混在一起。
//
// 旧实现里 scanEvents 返回共享的包级 eventScratch 切片、parseJSONMap 返回
// 共享的 mapScratch 映射，第二次查询会改写第一次查询结果的底层数据，
// 表现为「先拿到的那份事件列表被后一次查询改写，两侧指令混在一起」。
func TestListBySideIndependentAcrossQueries(t *testing.T) {
	s := newTestStore(t)
	es := NewEventStore(s.DB())

	const batchID int64 = 1
	now := time.Now().UTC()

	// 参考轨迹：seq=0，操作码 ADD-REF，写 rax=0xRef。
	// 待测轨迹：seq=0，操作码 ADD-TEST，写 rax=0xTest。
	// 两侧操作码与写入值刻意区分，任何混入都能被立即发现。
	ref := &model.ExecEvent{
		BatchID: batchID, Side: model.TrailReference, Seq: 0,
		PC: "0x0", Opcode: "ADD-REF", Stage: "retire",
		Writes: []model.ResourceAccess{{Kind: "reg", Name: "rax"}},
		Values: map[string]string{"reg:rax": "0xRef"},
		Status: model.EventRaw, CreatedAt: now,
	}
	test := &model.ExecEvent{
		BatchID: batchID, Side: model.TrailUnderTest, Seq: 0,
		PC: "0x0", Opcode: "ADD-TEST", Stage: "retire",
		Writes: []model.ResourceAccess{{Kind: "reg", Name: "rax"}},
		Values: map[string]string{"reg:rax": "0xTest"},
		Status: model.EventRaw, CreatedAt: now,
	}
	insertEvent(t, s, ref)
	insertEvent(t, s, test)

	// 连续两次查询：先参考，后待测。
	// 旧实现下，refEvents 的底层数据会被第二次查询改写。
	refEvents, err := es.ListBySide(batchID, model.TrailReference, "")
	if err != nil {
		t.Fatalf("list reference: %v", err)
	}
	testEvents, err := es.ListBySide(batchID, model.TrailUnderTest, "")
	if err != nil {
		t.Fatalf("list under_test: %v", err)
	}

	if len(refEvents) != 1 || len(testEvents) != 1 {
		t.Fatalf("expected 1 event per side, got ref=%d test=%d", len(refEvents), len(testEvents))
	}

	// 切片独立性：参考列表内容不被待测查询改写。
	if got := refEvents[0].Opcode; got != "ADD-REF" {
		t.Errorf("reference opcode corrupted by under_test query: got %q want ADD-REF", got)
	}
	if got := testEvents[0].Opcode; got != "ADD-TEST" {
		t.Errorf("under_test opcode wrong: got %q want ADD-TEST", got)
	}

	// Values 映射独立性：参考事件的写入值不被待测查询改写。
	// 旧实现下 parseJSONMap 返回共享映射，两侧写入值会混在一起。
	if got := refEvents[0].Values["reg:rax"]; got != "0xRef" {
		t.Errorf("reference value corrupted by under_test query: got %q want 0xRef", got)
	}
	if got := testEvents[0].Values["reg:rax"]; got != "0xTest" {
		t.Errorf("under_test value wrong: got %q want 0xTest", got)
	}

	// 再次确认：参考侧与待测侧 Values 映射不是同一个对象。
	// 往参考侧写入探针键，待测侧不应看到。
	refEvents[0].Values["__probe__"] = "ref-only"
	if _, leaked := testEvents[0].Values["__probe__"]; leaked {
		t.Errorf("reference and under_test share the same Values map (probe leaked)")
	}
}