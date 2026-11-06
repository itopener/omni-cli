package schema

import (
	"encoding/json"
	"testing"

	"omni-cli/internal/conn"
)

// names 提取字段名列表，便于断言 Fixed 的顺序与内容。
func names(fields []Field) []string {
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		out = append(out, f.Name)
	}
	return out
}

func TestBuildFixedWithDriver(t *testing.T) {
	got, err := Build(true)
	if err != nil {
		t.Fatalf("Build(true) 意外错误: %v", err)
	}
	gotNames := names(got.Fixed)
	wantNames := []string{"key", "env", "type", "driver"}
	if len(gotNames) != len(wantNames) {
		t.Fatalf("Build(true).Fixed 名称 = %v, 期望 %v", gotNames, wantNames)
	}
	for i := range wantNames {
		if gotNames[i] != wantNames[i] {
			t.Errorf("Fixed[%d].Name = %q, 期望 %q", i, gotNames[i], wantNames[i])
		}
	}
}

func TestBuildFixedNoDriver(t *testing.T) {
	got, err := Build(false)
	if err != nil {
		t.Fatalf("Build(false) 意外错误: %v", err)
	}
	gotNames := names(got.Fixed)
	wantNames := []string{"key", "env", "type"}
	if len(gotNames) != len(wantNames) {
		t.Fatalf("Build(false).Fixed 名称 = %v, 期望 %v", gotNames, wantNames)
	}
	for i := range wantNames {
		if gotNames[i] != wantNames[i] {
			t.Errorf("Fixed[%d].Name = %q, 期望 %q", i, gotNames[i], wantNames[i])
		}
	}
	for _, name := range gotNames {
		if name == "driver" {
			t.Errorf("Build(false).Fixed 不应包含 driver")
		}
	}
}

func TestCommandSchemaJSONParamKeysLowercase(t *testing.T) {
	s, err := Build(false)
	if err != nil {
		t.Fatalf("Build(false) 意外错误: %v", err)
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("Marshal 意外错误: %v", err)
	}
	var top map[string]any
	if err := json.Unmarshal(data, &top); err != nil {
		t.Fatalf("Unmarshal 意外错误: %v", err)
	}
	pbt, ok := top["params_by_type"].(map[string]any)
	if !ok {
		t.Fatalf("params_by_type 类型 = %T, 期望 map", top["params_by_type"])
	}
	fields, ok := pbt["tcp"].([]any)
	if !ok || len(fields) == 0 {
		t.Fatalf("params_by_type[\"tcp\"] 应为非空数组, 实际 %#v", pbt["tcp"])
	}
	first, ok := fields[0].(map[string]any)
	if !ok {
		t.Fatalf("参数字段类型 = %T, 期望 map", fields[0])
	}
	for _, k := range []string{"name", "required", "description"} {
		if _, ok := first[k]; !ok {
			t.Errorf("参数字段缺少 JSON key %q。实际 keys: %v", k, mapKeys(first))
		}
	}
	for _, k := range []string{"Name", "Required", "Desc"} {
		if _, ok := first[k]; ok {
			t.Errorf("参数字段 JSON key %q 应为小写", k)
		}
	}
}

// mapKeys 提取 map 的 key 列表，便于错误信息定位。
func mapKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestBuildParamsByType(t *testing.T) {
	got, err := Build(false)
	if err != nil {
		t.Fatalf("Build(false) 意外错误: %v", err)
	}
	fields, ok := got.ParamsByType[conn.ConnTypeTCP]
	if !ok {
		t.Fatalf("ParamsByType 缺少 %s", conn.ConnTypeTCP)
	}
	byName := map[string]bool{}
	required := map[string]bool{}
	for _, f := range fields {
		byName[f.Name] = true
		required[f.Name] = f.Required
	}
	for _, name := range []string{"host", "port"} {
		if !byName[name] {
			t.Errorf("ParamsByType[%s] 应包含 %s", conn.ConnTypeTCP, name)
		}
		if !required[name] {
			t.Errorf("ParamsByType[%s] 的 %s 应 Required=true", conn.ConnTypeTCP, name)
		}
	}
}
