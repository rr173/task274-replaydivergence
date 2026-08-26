package compare

import (
	"testing"

	"task274-replaydivergence/internal/model"
)

func TestHashStateDeterministic(t *testing.T) {
	fp := NewFingerprinter("fnv1a-sorted")
	state := map[string]string{
		"reg:rax":     "0x1",
		"reg:rbx":     "0x2",
		"mem:0x1000":  "0x2a",
	}
	h1 := fp.HashState(state)
	h2 := fp.HashState(state)
	if h1 != h2 {
		t.Fatalf("hash not deterministic: %s vs %s", h1, h2)
	}
}

func TestHashStateOrderInsensitive(t *testing.T) {
	fp := NewFingerprinter("fnv1a-sorted")
	a := fp.HashState(map[string]string{"reg:rax": "0x1", "reg:rbx": "0x2"})
	b := fp.HashState(map[string]string{"reg:rbx": "0x2", "reg:rax": "0x1"})
	if a != b {
		t.Fatalf("hash should be order-insensitive: %s vs %s", a, b)
	}
}

func TestHashStateSensitiveToValue(t *testing.T) {
	fp := NewFingerprinter("fnv1a-sorted")
	a := fp.HashState(map[string]string{"reg:rbx": "0x1"})
	b := fp.HashState(map[string]string{"reg:rbx": "0x2"})
	if a == b {
		t.Fatal("hash should differ when value differs")
	}
}

func TestSelectScope(t *testing.T) {
	state := map[string]string{"reg:rax": "0x1", "mem:0x1000": "0x2a"}
	regs := SelectScope(state, "register")
	if _, ok := regs["reg:rax"]; !ok {
		t.Fatal("register scope should keep reg keys")
	}
	if _, ok := regs["mem:0x1000"]; ok {
		t.Fatal("register scope should drop mem keys")
	}
	mems := SelectScope(state, "memory")
	if len(mems) != 1 {
		t.Fatalf("memory scope expected 1 key, got %d", len(mems))
	}
	full := SelectScope(state, "full")
	if len(full) != 2 {
		t.Fatalf("full scope expected 2 keys, got %d", len(full))
	}
}

func TestReplayer(t *testing.T) {
	e := &model.ExecEvent{
		Seq:    0,
		Writes: []model.ResourceAccess{{Kind: "reg", Name: "rax"}},
		Values: map[string]string{"reg:rax": "0x1"},
	}
	state := map[string]string{}
	if err := NewReplayer().Replay(state, []*model.ExecEvent{e}); err != nil {
		t.Fatalf("replay: %v", err)
	}
	if state["reg:rax"] != "0x1" {
		t.Fatalf("expected reg:rax=0x1, got %q", state["reg:rax"])
	}
	// 缺写入值必须报错（指纹回放不静默）。
	e2 := &model.ExecEvent{Seq: 1, Writes: []model.ResourceAccess{{Kind: "mem", Name: "0x2000"}}}
	if err := NewReplayer().Replay(state, []*model.ExecEvent{e2}); err == nil {
		t.Fatal("replay without write value should fail")
	}
}
