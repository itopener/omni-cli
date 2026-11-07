package output

import (
	"encoding/json"
	"strings"
	"testing"

	"omni-cli/internal/db"
)

func TestTable(t *testing.T) {
	tests := []struct {
		name   string
		result *db.Result
		want   []string
	}{
		{
			name:   "基本两列两行",
			result: &db.Result{Columns: []string{"id", "name"}, Rows: [][]any{{1, "a"}, {2, "b"}}},
			want:   []string{"id", "name", "1", "a"},
		},
		{
			name:   "nil 渲染为 NULL",
			result: &db.Result{Columns: []string{"c"}, Rows: [][]any{{nil}}},
			want:   []string{"c", "NULL"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Table(tt.result)
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("Table() 输出应包含 %q，实际:\n%s", w, got)
				}
			}
		})
	}
}

func TestJSON(t *testing.T) {
	got := JSON(map[string]any{"A": 1})
	var v map[string]any
	if err := json.Unmarshal([]byte(got), &v); err != nil {
		t.Fatalf("JSON() 输出不是合法 JSON: %v\n%s", err, got)
	}
	if v["A"] != float64(1) {
		t.Errorf("JSON() 输出字段 A 期望 1，实际 %v", v["A"])
	}
}
