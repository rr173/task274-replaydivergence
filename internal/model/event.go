package model

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// ResourceAccess 描述一条指令对某个资源的读写。
type ResourceAccess struct {
	Kind string `json:"kind"` // reg | mem
	Name string `json:"name"` // 寄存器名或内存地址（十六进制）
}

// Key 返回资源稳定标识（reg:rax / mem:0x1000）。
func (r ResourceAccess) Key() string {
	return fmt.Sprintf("%s:%s", r.Kind, r.Name)
}

// ExecEvent 是一条已采集的执行事件：程序计数器、操作码、读写资源、写入值与阶段。
// 以 (batch_id, side, seq) 为唯一键，seq 幂等防止重复导入。
type ExecEvent struct {
	ID        int64            `json:"id"`
	BatchID   int64            `json:"batch_id"`
	Side      TrailSide        `json:"side"`
	Seq       int64            `json:"seq"`
	PC        string           `json:"pc"`
	Opcode    string           `json:"opcode"`
	Stage     string           `json:"stage"`
	Reads     []ResourceAccess `json:"reads"`
	Writes    []ResourceAccess `json:"writes"`
	Values    map[string]string `json:"values,omitempty"` // 写入资源名 -> 写入值（指纹回放用）
	Status    EventStatus      `json:"status"`
	Note      string           `json:"note,omitempty"`
	CreatedAt time.Time        `json:"created_at"`
}

// ReadsJSON / WritesJSON 返回可存储的 JSON 字节。
func (e *ExecEvent) ReadsJSON() []byte {
	b, _ := json.Marshal(e.Reads)
	return b
}

// WritesJSON 返回写资源 JSON 字节。
func (e *ExecEvent) WritesJSON() []byte {
	b, _ := json.Marshal(e.Writes)
	return b
}

// ValuesJSON 返回写入值字典 JSON 字节。
func (e *ExecEvent) ValuesJSON() []byte {
	b, _ := json.Marshal(e.Values)
	return b
}

// NormalizeReads 排序并去重读资源，保证指纹与依赖判定稳定。
func (e *ExecEvent) NormalizeReads() {
	sort.Slice(e.Reads, func(i, j int) bool { return e.Reads[i].Key() < e.Reads[j].Key() })
}

// NormalizeWrites 排序并去重写资源。
func (e *ExecEvent) NormalizeWrites() {
	sort.Slice(e.Writes, func(i, j int) bool { return e.Writes[i].Key() < e.Writes[j].Key() })
}

// WritesResource 判断事件是否写入指定资源。
func (e *ExecEvent) WritesResource(r ResourceAccess) bool {
	for _, w := range e.Writes {
		if w.Kind == r.Kind && w.Name == r.Name {
			return true
		}
	}
	return false
}

// ReadsResource 判断事件是否读取指定资源。
func (e *ExecEvent) ReadsResource(r ResourceAccess) bool {
	for _, rd := range e.Reads {
		if rd.Kind == r.Kind && rd.Name == r.Name {
			return true
		}
	}
	return false
}

// EventValue 表示指令写入后的资源值（用于指纹计算）。
type EventValue struct {
	Resource ResourceAccess `json:"resource"`
	Value    string         `json:"value"`
}
