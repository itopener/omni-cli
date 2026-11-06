package schema

import (
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
