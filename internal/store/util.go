package store

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

// timeFmt 是 SQLite 中的时间存储格式（UTC RFC3339，可排序）。
const timeFmt = time.RFC3339Nano

func nowUTC() string {
	return time.Now().UTC().Format(timeFmt)
}

// parseTime 解析 SQLite 存储的时间字符串，失败返回零值。
func parseTime(s string) time.Time {
	t, err := time.Parse(timeFmt, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// marshalJSON 序列化任意值；失败返回空串（调用方应保证值可序列化）。
func marshalJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

// parseJSONMap 反序列化 JSON 到字符串字典。
var mapScratch map[string]string

func parseJSONMap(s string) (map[string]string, error) {
	if mapScratch == nil {
		mapScratch = map[string]string{}
	}
	for k := range mapScratch {
		delete(mapScratch, k)
	}
	if strings.TrimSpace(s) == "" {
		return mapScratch, nil
	}
	err := json.Unmarshal([]byte(s), &mapScratch)
	return mapScratch, err
}

// parseResourceAccess 反序列化资源访问列表。
func parseResourceAccess(s string) ([]ResourceAccessDTO, error) {
	out := []ResourceAccessDTO{}
	if strings.TrimSpace(s) == "" {
		return out, nil
	}
	err := json.Unmarshal([]byte(s), &out)
	return out, err
}

// ResourceAccessDTO 是数据库层使用的资源访问中间结构，
// 避免 store 直接依赖 model 的序列化细节。
type ResourceAccessDTO struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// scanNullableTime 读取可空时间列。
func scanNullableTime(v sql.NullString) *time.Time {
	if !v.Valid || v.String == "" {
		return nil
	}
	t := parseTime(v.String)
	return &t
}
