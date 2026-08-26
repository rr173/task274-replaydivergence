// Package compare 实现处理器状态指纹计算与双轨迹状态比较。
package compare

import (
	"fmt"
	"hash/fnv"
	"sort"

	"task274-replaydivergence/internal/model"
)

// Fingerprinter 计算状态指纹：对排序后的 (资源, 值) 对做 FNV-1a 迭代哈希。
type Fingerprinter struct {
	algo string
}

// NewFingerprinter 构造指纹器，algo 标识写入指纹记录。
func NewFingerprinter(algo string) *Fingerprinter {
	if algo == "" {
		algo = "fnv1a-sorted"
	}
	return &Fingerprinter{algo: algo}
}

// Algo 返回指纹算法标识。
func (f *Fingerprinter) Algo() string {
	return f.algo
}

// HashState 计算状态映射的指纹哈希。
// 状态 key 形如 reg:rax / mem:0x1000，值取写入值；排序后迭代哈希保证顺序无关。
func (f *Fingerprinter) HashState(state map[string]string) string {
	h := fnv.New64a()
	keys := make([]string, 0, len(state))
	for k := range state {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		_, _ = h.Write([]byte(k))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(state[k]))
		_, _ = h.Write([]byte{0})
	}
	return fmt.Sprintf("%016x", h.Sum64())
}

// Replayer 按指令序号回放状态：把事件写入值应用到状态映射。
// 用于在检查点处重建某一侧轨迹的寄存器/内存状态。
type Replayer struct{}

// NewReplayer 构造状态回放器。
func NewReplayer() *Replayer { return &Replayer{} }

// Replay 回放事件序列到状态映射。
// events 必须按 seq 升序；返回的映射是调用方提供的 state 的浅拷贝上的变更，
// 因此调用方可传入共享初始状态（如复位后的架构状态）。
func (r *Replayer) Replay(state map[string]string, events []*model.ExecEvent) error {
	for _, e := range events {
		for _, w := range e.Writes {
			val, ok := e.Values[w.Key()]
			if !ok {
				return fmt.Errorf("event seq=%d writes %s without value", e.Seq, w.Key())
			}
			state[w.Key()] = val
		}
	}
	return nil
}

// SelectScope 从状态映射中取出指定范围的子集。
// register 只取 reg:*；memory 只取 mem:*；full 取全部。
func SelectScope(state map[string]string, scope model.FingerprintScope) map[string]string {
	out := map[string]string{}
	for k, v := range state {
		kind := "reg"
		for i := 0; i < len(k); i++ {
			if k[i] == ':' {
				kind = k[:i]
				break
			}
		}
		switch scope {
		case model.ScopeRegister:
			if kind == "reg" {
				out[k] = v
			}
		case model.ScopeMemory:
			if kind == "mem" {
				out[k] = v
			}
		default:
			out[k] = v
		}
	}
	return out
}
