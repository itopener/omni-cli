package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"omni-cli/internal/conn"
)

// writeTemp 把 content 写进临时目录下的 config.yaml，返回路径。
func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("写入临时配置失败: %v", err)
	}
	return path
}

func TestLoadParsesYAML(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		want    *Config
		wantErr bool
	}{
		{
			name: "db 两条 + redis 一条",
			yaml: `version: 1
db:
  - key: orders
    env: dev
    type: tcp
    driver: mysql
    params:
      host: 127.0.0.1
      port: 3306
      user: root
      database: orders
  - key: orders
    env: prod
    type: tcp
    driver: mysql
    params:
      host: 10.0.0.2
      port: 3306
      user: root
      database: orders
redis:
  - key: cache
    env: dev
    type: tcp
    params:
      host: 127.0.0.1
      port: 6379
      db: 0
`,
			want: &Config{
				Version: 1,
				DB: []Entry{
					{Key: "orders", Env: "dev", Type: conn.ConnTypeTCP, Driver: "mysql", Params: map[string]any{"host": "127.0.0.1", "port": 3306, "user": "root", "database": "orders"}},
					{Key: "orders", Env: "prod", Type: conn.ConnTypeTCP, Driver: "mysql", Params: map[string]any{"host": "10.0.0.2", "port": 3306, "user": "root", "database": "orders"}},
				},
				Redis: []Entry{
					{Key: "cache", Env: "dev", Type: conn.ConnTypeTCP, Params: map[string]any{"host": "127.0.0.1", "port": 6379, "db": 0}},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTemp(t, tt.yaml)
			got, err := Load(path)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Load(%s) 期望报错，实际成功", path)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load(%s) 意外错误: %v", path, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Load(%s) = %+v, 期望 %+v", path, got, tt.want)
			}
		})
	}
}

func TestLoadDefaultType(t *testing.T) {
	path := writeTemp(t, `version: 1
db:
  - key: orders
    env: dev
    params:
      host: 127.0.0.1
      port: 3306
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load 意外错误: %v", err)
	}
	if got := cfg.DB[0].Type; got != conn.ConnTypeTCP {
		t.Errorf("Type = %q, 期望 %q", got, conn.ConnTypeTCP)
	}
}

func TestLoadInterpolatesEnv(t *testing.T) {
	t.Setenv("FOO", "x")
	path := writeTemp(t, `version: 1
db:
  - key: orders
    env: dev
    type: tcp
    driver: mysql
    params:
      host: 127.0.0.1
      port: 3306
      user: root
      password: ${FOO}
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load 意外错误: %v", err)
	}
	if got := cfg.DB[0].Params["password"]; got != "x" {
		t.Errorf("password = %v, 期望 %q", got, "x")
	}
}

// TestLoadRawSkipsInterpolation 回归：LoadRaw 保留 ${VAR} 字面量，但默认 type 填充仍生效。
func TestLoadRawSkipsInterpolation(t *testing.T) {
	t.Setenv("FOO", "x")
	path := writeTemp(t, `version: 1
db:
  - key: orders
    env: dev
    params:
      host: 127.0.0.1
      port: 3306
      user: root
      password: ${FOO}
`)
	cfg, err := LoadRaw(path)
	if err != nil {
		t.Fatalf("LoadRaw 意外错误: %v", err)
	}
	e := cfg.DB[0]
	if got := e.Params["password"]; got != "${FOO}" {
		t.Errorf("password = %v, 期望字面量 ${FOO}", got)
	}
	if got := e.Type; got != conn.ConnTypeTCP {
		t.Errorf("Type = %q, 期望 %q", got, conn.ConnTypeTCP)
	}
}

func TestLoadUnsetEnvVarErrors(t *testing.T) {
	const name = "OMNI_CLI_UNSET_VAR_FOR_TEST"
	os.Unsetenv(name)
	t.Cleanup(func() { os.Unsetenv(name) })

	path := writeTemp(t, `version: 1
db:
  - key: orders
    env: dev
    type: tcp
    params:
      host: ${`+name+`}
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("期望未设置环境变量报错，实际成功")
	}
	if !strings.Contains(err.Error(), name) {
		t.Errorf("错误信息 %q 应包含变量名 %q", err.Error(), name)
	}
}

func TestValidateDupKeyEnvErrors(t *testing.T) {
	cfg := &Config{
		Version: 1,
		DB: []Entry{
			{Key: "orders", Env: "dev", Type: conn.ConnTypeTCP},
			{Key: "orders", Env: "dev", Type: conn.ConnTypeTCP},
		},
	}
	if err := validate(cfg); err == nil {
		t.Fatal("期望重复 key+env 报错，实际成功")
	}
}

func TestValidateMissingKeyErrors(t *testing.T) {
	cfg := &Config{
		Version: 1,
		DB: []Entry{
			{Env: "dev", Type: conn.ConnTypeTCP},
		},
	}
	if err := validate(cfg); err == nil {
		t.Fatal("期望缺少 key 报错，实际成功")
	}
}

func TestValidateUnknownTypeErrors(t *testing.T) {
	cfg := &Config{
		Version: 1,
		DB: []Entry{
			{Key: "orders", Env: "dev", Type: conn.ConnType("platform")},
		},
	}
	err := validate(cfg)
	if err == nil {
		t.Fatal("期望未知类型报错，实际成功")
	}
	if !strings.Contains(err.Error(), "tcp") {
		t.Errorf("错误信息 %q 应包含 \"tcp\"", err.Error())
	}
}

func TestSaveCreatesParentDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "config.yaml")
	cfg := &Config{Version: 1}
	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save 到不存在目录 %s 意外错误: %v", path, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Save 后文件应存在: %v", err)
	}
}

func TestEntryIsReadOnly(t *testing.T) {
	yes := true
	no := false
	tests := []struct {
		name string
		e    Entry
		want bool
	}{
		{name: "nil 默认只读", e: Entry{ReadOnly: nil}, want: true},
		{name: "显式 false 可写", e: Entry{ReadOnly: &no}, want: false},
		{name: "显式 true 只读", e: Entry{ReadOnly: &yes}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.e.IsReadOnly(); got != tt.want {
				t.Errorf("IsReadOnly() = %v, 期望 %v", got, tt.want)
			}
		})
	}
}

func TestSaveRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	orig := &Config{
		Version: 1,
		DB: []Entry{
			{Key: "orders", Env: "dev", Type: conn.ConnTypeTCP, Driver: "mysql", Params: map[string]any{"host": "127.0.0.1", "port": 3306, "user": "root", "database": "orders"}},
		},
		Redis: []Entry{
			{Key: "cache", Env: "dev", Type: conn.ConnTypeTCP, Params: map[string]any{"host": "127.0.0.1", "port": 6379}},
		},
	}
	if err := Save(path, orig); err != nil {
		t.Fatalf("Save 意外错误: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat 意外错误: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("文件权限 = %o, 期望 0600", perm)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load 意外错误: %v", err)
	}
	if !reflect.DeepEqual(got, orig) {
		t.Errorf("Save→Load 不等:\n got=%+v\nwant=%+v", got, orig)
	}
}
