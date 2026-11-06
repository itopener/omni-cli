package config

import (
	"strings"
	"testing"
)

func TestResolveExact(t *testing.T) {
	entries := []Entry{
		{Key: "orders", Env: "dev"},
		{Key: "orders", Env: "prod"},
		{Key: "cache", Env: "dev"},
	}
	tests := []struct {
		name    string
		key     string
		env     string
		wantKey string
		wantEnv string
	}{
		{name: "orders/dev", key: "orders", env: "dev", wantKey: "orders", wantEnv: "dev"},
		{name: "orders/prod", key: "orders", env: "prod", wantKey: "orders", wantEnv: "prod"},
		{name: "cache/dev", key: "cache", env: "dev", wantKey: "cache", wantEnv: "dev"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveEntry(entries, tt.key, tt.env)
			if err != nil {
				t.Fatalf("ResolveEntry(%q, %q) 意外错误: %v", tt.key, tt.env, err)
			}
			if got.Key != tt.wantKey || got.Env != tt.wantEnv {
				t.Errorf("ResolveEntry(%q, %q) = %s/%s, 期望 %s/%s",
					tt.key, tt.env, got.Key, got.Env, tt.wantKey, tt.wantEnv)
			}
		})
	}
}

func TestResolveDefaultEnvSingle(t *testing.T) {
	tests := []struct {
		name    string
		entries []Entry
		key     string
		want    Entry
	}{
		{
			name:    "key 仅一条",
			entries: []Entry{{Key: "orders", Env: "dev"}, {Key: "cache", Env: "prod"}},
			key:     "orders",
			want:    Entry{Key: "orders", Env: "dev"},
		},
		{
			name:    "其余 key 同名 env 不影响",
			entries: []Entry{{Key: "orders", Env: "dev"}, {Key: "cache", Env: "dev"}},
			key:     "orders",
			want:    Entry{Key: "orders", Env: "dev"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveEntry(tt.entries, tt.key, "")
			if err != nil {
				t.Fatalf("ResolveEntry(%q, \"\") 意外错误: %v", tt.key, err)
			}
			if got.Key != tt.want.Key || got.Env != tt.want.Env {
				t.Errorf("ResolveEntry(%q, \"\") = %s/%s, 期望 %s/%s",
					tt.key, got.Key, got.Env, tt.want.Key, tt.want.Env)
			}
		})
	}
}

func TestResolveDefaultEnvAmbiguous(t *testing.T) {
	entries := []Entry{
		{Key: "orders", Env: "dev"},
		{Key: "orders", Env: "prod"},
	}
	_, err := ResolveEntry(entries, "orders", "")
	if err == nil {
		t.Fatal("期望多个 env 时报错，实际成功")
	}
	for _, env := range []string{"dev", "prod"} {
		if !strings.Contains(err.Error(), env) {
			t.Errorf("错误信息 %q 应包含 env 名 %q", err.Error(), env)
		}
	}
}

func TestResolveDefaultKeySingle(t *testing.T) {
	tests := []struct {
		name    string
		entries []Entry
		env     string
		want    Entry
	}{
		{
			name:    "env 仅一条",
			entries: []Entry{{Key: "orders", Env: "dev"}, {Key: "cache", Env: "prod"}},
			env:     "dev",
			want:    Entry{Key: "orders", Env: "dev"},
		},
		{
			name:    "其余 env 同名 key 不影响",
			entries: []Entry{{Key: "orders", Env: "dev"}, {Key: "orders", Env: "prod"}},
			env:     "dev",
			want:    Entry{Key: "orders", Env: "dev"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveEntry(tt.entries, "", tt.env)
			if err != nil {
				t.Fatalf("ResolveEntry(\"\", %q) 意外错误: %v", tt.env, err)
			}
			if got.Key != tt.want.Key || got.Env != tt.want.Env {
				t.Errorf("ResolveEntry(\"\", %q) = %s/%s, 期望 %s/%s",
					tt.env, got.Key, got.Env, tt.want.Key, tt.want.Env)
			}
		})
	}
}

func TestResolveDefaultKeyAmbiguous(t *testing.T) {
	entries := []Entry{
		{Key: "orders", Env: "dev"},
		{Key: "cache", Env: "dev"},
	}
	_, err := ResolveEntry(entries, "", "dev")
	if err == nil {
		t.Fatal("期望多个 key 时报错，实际成功")
	}
	for _, key := range []string{"orders", "cache"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("错误信息 %q 应包含 key 名 %q", err.Error(), key)
		}
	}
}

func TestResolveNoMatch(t *testing.T) {
	entries := []Entry{{Key: "orders", Env: "dev"}}
	tests := []struct {
		name string
		key  string
		env  string
	}{
		{name: "key 不存在", key: "cache", env: "dev"},
		{name: "env 不存在", key: "orders", env: "prod"},
		{name: "两者都不存在", key: "cache", env: "prod"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveEntry(entries, tt.key, tt.env)
			if err == nil {
				t.Fatalf("ResolveEntry(%q, %q) 期望报错，实际返回 %+v", tt.key, tt.env, got)
			}
		})
	}
}
