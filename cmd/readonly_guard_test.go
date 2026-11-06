package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"omni-cli/internal/config"
	"omni-cli/internal/conn"
)

// TestRunDBReadonlyGuard 回归：db 守卫接线——只读写操作在 resolveTCP/建连前拦截，可写条目放行。
func TestRunDBReadonlyGuard(t *testing.T) {
	path := setTestConfig(t)
	tests := []struct {
		name      string
		ro        bool
		sql       string
		params    map[string]any
		wantGuard bool
	}{
		{
			name:      "只读拦截 DELETE",
			ro:        true,
			sql:       "DELETE FROM x",
			params:    map[string]any{"host": "127.0.0.1", "port": 1},
			wantGuard: true,
		},
		{
			name:      "只读放行 SELECT",
			ro:        true,
			sql:       "SELECT 1",
			params:    map[string]any{"port": "notanumber"},
			wantGuard: false,
		},
		{
			name:      "可写条目放行 DELETE",
			ro:        false,
			sql:       "DELETE FROM x",
			params:    map[string]any{"port": "notanumber"},
			wantGuard: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{Version: 1, DB: []config.Entry{
				{Key: "t", Env: "dev", Type: conn.ConnTypeTCP, Driver: "mysql", ReadOnly: &tt.ro, Params: tt.params},
			}}
			if err := config.Save(path, cfg); err != nil {
				t.Fatalf("Save 意外错误: %v", err)
			}
			cmd := &cobra.Command{}
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)
			err := runDB(cmd, "dev", "t", formatTable, tt.sql, "1s", nil)
			if tt.wantGuard {
				if err == nil {
					t.Fatal("期望只读拦截错误，实际 nil")
				}
				if !strings.Contains(err.Error(), "只读连接禁止写操作") {
					t.Errorf("错误 %q 应包含只读拦截提示", err)
				}
				if strings.Contains(err.Error(), "查询失败") || strings.Contains(err.Error(), "dial") {
					t.Errorf("只读拦截不应尝试建连，实际错误 %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("期望非 nil（resolveTCP 失败），实际 nil")
			}
			if strings.Contains(err.Error(), "只读连接禁止写操作") {
				t.Errorf("不应被只读拦截，实际错误 %v", err)
			}
			if !strings.Contains(err.Error(), "port") {
				t.Errorf("应到达 resolveTCP 校验，实际错误 %v", err)
			}
		})
	}
}

// TestRunRedisReadonlyGuard 回归：redis 守卫接线——只读写命令在 resolveTCP/建连前拦截，可写条目放行。
func TestRunRedisReadonlyGuard(t *testing.T) {
	path := setTestConfig(t)
	tests := []struct {
		name      string
		ro        bool
		args      []string
		params    map[string]any
		wantGuard bool
	}{
		{
			name:      "只读拦截 SET",
			ro:        true,
			args:      []string{"SET", "k", "v"},
			params:    map[string]any{"host": "127.0.0.1", "port": 1},
			wantGuard: true,
		},
		{
			name:      "只读放行 GET",
			ro:        true,
			args:      []string{"GET", "k"},
			params:    map[string]any{"port": "notanumber"},
			wantGuard: false,
		},
		{
			name:      "可写条目放行 SET",
			ro:        false,
			args:      []string{"SET", "k", "v"},
			params:    map[string]any{"port": "notanumber"},
			wantGuard: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{Version: 1, Redis: []config.Entry{
				{Key: "t", Env: "dev", Type: conn.ConnTypeTCP, ReadOnly: &tt.ro, Params: tt.params},
			}}
			if err := config.Save(path, cfg); err != nil {
				t.Fatalf("Save 意外错误: %v", err)
			}
			cmd := &cobra.Command{}
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)
			err := runRedis(cmd, "dev", "t", formatTable, "1s", tt.args)
			if tt.wantGuard {
				if err == nil {
					t.Fatal("期望只读拦截错误，实际 nil")
				}
				if !strings.Contains(err.Error(), "只读连接禁止写操作") {
					t.Errorf("错误 %q 应包含只读拦截提示", err)
				}
				if strings.Contains(err.Error(), "执行 redis 命令失败") || strings.Contains(err.Error(), "dial") {
					t.Errorf("只读拦截不应尝试建连，实际错误 %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("期望非 nil（resolveTCP 失败），实际 nil")
			}
			if strings.Contains(err.Error(), "只读连接禁止写操作") {
				t.Errorf("不应被只读拦截，实际错误 %v", err)
			}
			if !strings.Contains(err.Error(), "port") {
				t.Errorf("应到达 resolveTCP 校验，实际错误 %v", err)
			}
		})
	}
}
