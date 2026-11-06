package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"omni-cli/internal/config"
)

// setTestConfig 把 configPath 指向临时目录下的 config.yaml，测试后还原。
func setTestConfig(t *testing.T) string {
	t.Helper()
	old := configPath
	path := filepath.Join(t.TempDir(), "config.yaml")
	configPath = path
	t.Cleanup(func() { configPath = old })
	return path
}

// runConfigCmd 执行 config 子命令，返回输出与错误。
func runConfigCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := newConfigCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func TestIsSecretKey(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"password", true},
		{"Password", true},
		{"db_token", true},
		{"apiSecret", true},
		{"api_secret_key", true},
		{"host", false},
		{"port", false},
		{"user", false},
	}
	for _, tt := range tests {
		if got := isSecretKey(tt.key); got != tt.want {
			t.Errorf("isSecretKey(%q) = %v, 期望 %v", tt.key, got, tt.want)
		}
	}
}

func TestParseParams(t *testing.T) {
	got, err := parseParams([]string{"host=127.0.0.1", "port=3306", "password=${DEV_DB_PASS}"})
	if err != nil {
		t.Fatalf("parseParams 意外错误: %v", err)
	}
	if got["host"] != "127.0.0.1" || got["port"] != "3306" || got["password"] != "${DEV_DB_PASS}" {
		t.Errorf("parseParams = %v", got)
	}

	for _, bad := range []string{"no-equals", "=empty-key"} {
		if _, err := parseParams([]string{bad}); err == nil {
			t.Errorf("parseParams(%q) 期望报错，实际成功", bad)
		}
	}
}

func TestMaskParams(t *testing.T) {
	in := map[string]any{"host": "127.0.0.1", "password": "s3cret", "api_token": "tok", "user": "root"}
	got := maskParams(in)
	if got["password"] != maskValue {
		t.Errorf("password = %v, 期望 %q", got["password"], maskValue)
	}
	if got["api_token"] != maskValue {
		t.Errorf("api_token = %v, 期望 %q", got["api_token"], maskValue)
	}
	if got["host"] != "127.0.0.1" || got["user"] != "root" {
		t.Errorf("非敏感字段被改动: %v", got)
	}
}

func TestConfigInitCreatesAndSkips(t *testing.T) {
	path := setTestConfig(t)
	out, err := runConfigCmd(t, "init")
	if err != nil {
		t.Fatalf("init 意外错误: %v", err)
	}
	if !strings.Contains(out, path) {
		t.Errorf("init 输出 %q 应包含路径 %q", out, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取配置失败: %v", err)
	}
	if !strings.Contains(string(data), "version: 1") {
		t.Errorf("模板应包含 version: 1，实际 %q", string(data))
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat 失败: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("文件权限 = %o, 期望 0600", perm)
	}

	// 再次 init 应跳过且不改写文件。
	out2, err := runConfigCmd(t, "init")
	if err != nil {
		t.Fatalf("第二次 init 意外错误: %v", err)
	}
	if !strings.Contains(out2, "已存在") {
		t.Errorf("第二次 init 应提示已存在，实际 %q", out2)
	}
}

func TestConfigAddThenGetMasksSecret(t *testing.T) {
	setTestConfig(t)
	if _, err := runConfigCmd(t, "add", "db", "--key", "orders", "--env", "dev",
		"--driver", "mysql",
		"--param", "host=127.0.0.1", "--param", "port=3306",
		"--param", "user=root", "--param", "password=s3cret", "--param", "database=orders"); err != nil {
		t.Fatalf("add 意外错误: %v", err)
	}
	out, err := runConfigCmd(t, "get", "db", "--key", "orders", "--env", "dev")
	if err != nil {
		t.Fatalf("get 意外错误: %v", err)
	}
	if !strings.Contains(out, maskValue) {
		t.Errorf("get 输出应含打码占位符 %q，实际 %q", maskValue, out)
	}
	if strings.Contains(out, "s3cret") {
		t.Errorf("get 输出泄漏密码，实际 %q", out)
	}
	if !strings.Contains(out, "orders") {
		t.Errorf("get 输出应含 key orders，实际 %q", out)
	}
}

func TestConfigAddDuplicateErrors(t *testing.T) {
	setTestConfig(t)
	base := []string{"add", "db", "--key", "orders", "--env", "dev", "--driver", "mysql",
		"--param", "host=127.0.0.1", "--param", "port=3306"}
	if _, err := runConfigCmd(t, base...); err != nil {
		t.Fatalf("首次 add 意外错误: %v", err)
	}
	if _, err := runConfigCmd(t, base...); err == nil {
		t.Fatal("重复 add 期望报错，实际成功")
	}
}

func TestConfigAddRedisRejectsDriver(t *testing.T) {
	setTestConfig(t)
	if _, err := runConfigCmd(t, "add", "redis", "--key", "cache", "--env", "dev", "--driver", "mysql"); err == nil {
		t.Fatal("redis 带 --driver 期望报错，实际成功")
	}
}

func TestConfigSetUpdates(t *testing.T) {
	path := setTestConfig(t)
	if _, err := runConfigCmd(t, "add", "db", "--key", "orders", "--env", "dev", "--driver", "mysql",
		"--param", "host=127.0.0.1", "--param", "password=old"); err != nil {
		t.Fatalf("add 意外错误: %v", err)
	}
	if _, err := runConfigCmd(t, "set", "db", "--key", "orders", "--env", "dev",
		"--driver", "postgres", "--param", "password=new"); err != nil {
		t.Fatalf("set 意外错误: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load 意外错误: %v", err)
	}
	e := cfg.DB[0]
	if e.Driver != "postgres" {
		t.Errorf("Driver = %q, 期望 postgres", e.Driver)
	}
	if e.Params["password"] != "new" {
		t.Errorf("password = %v, 期望 new", e.Params["password"])
	}
	if e.Params["host"] != "127.0.0.1" {
		t.Errorf("未覆盖的 host 应保留，实际 %v", e.Params["host"])
	}
}

func TestConfigRemoveDeletes(t *testing.T) {
	path := setTestConfig(t)
	if _, err := runConfigCmd(t, "add", "db", "--key", "orders", "--env", "dev", "--driver", "mysql",
		"--param", "host=127.0.0.1", "--param", "port=3306"); err != nil {
		t.Fatalf("add 意外错误: %v", err)
	}
	if _, err := runConfigCmd(t, "remove", "db", "--key", "orders", "--env", "dev"); err != nil {
		t.Fatalf("remove 意外错误: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load 意外错误: %v", err)
	}
	if len(cfg.DB) != 0 {
		t.Errorf("remove 后 db 应为空，实际 %d 条", len(cfg.DB))
	}
}

func TestConfigSchemaDBJSON(t *testing.T) {
	setTestConfig(t)
	out, err := runConfigCmd(t, "schema", "db")
	if err != nil {
		t.Fatalf("schema db 意外错误: %v", err)
	}
	var top map[string]any
	if err := json.Unmarshal([]byte(out), &top); err != nil {
		t.Fatalf("schema 输出不是合法 JSON: %v\n输出: %s", err, out)
	}
	fixed, ok := top["fixed"].([]any)
	if !ok {
		t.Fatalf("fixed 应为数组，实际 %T", top["fixed"])
	}
	hasDriver := false
	for _, f := range fixed {
		if m, ok := f.(map[string]any); ok && m["name"] == "driver" {
			hasDriver = true
		}
	}
	if !hasDriver {
		t.Errorf("db schema 的 fixed 应包含 driver")
	}
	pbt := top["params_by_type"].(map[string]any)
	fields := pbt["tcp"].([]any)
	first := fields[0].(map[string]any)
	if _, ok := first["name"]; !ok {
		t.Errorf("参数字段 JSON key 应为小写 name，实际 %v", mapKeysAny(first))
	}
}

func TestConfigSchemaRedisNoDriver(t *testing.T) {
	setTestConfig(t)
	out, err := runConfigCmd(t, "schema", "redis")
	if err != nil {
		t.Fatalf("schema redis 意外错误: %v", err)
	}
	var top map[string]any
	if err := json.Unmarshal([]byte(out), &top); err != nil {
		t.Fatalf("schema 输出不是合法 JSON: %v", err)
	}
	for _, f := range top["fixed"].([]any) {
		if m, ok := f.(map[string]any); ok && m["name"] == "driver" {
			t.Errorf("redis schema 的 fixed 不应包含 driver")
		}
	}
}

func TestConfigListAndShow(t *testing.T) {
	setTestConfig(t)
	if _, err := runConfigCmd(t, "add", "db", "--key", "orders", "--env", "dev", "--driver", "mysql",
		"--param", "host=127.0.0.1", "--param", "password=s3cret"); err != nil {
		t.Fatalf("add db 意外错误: %v", err)
	}
	listOut, err := runConfigCmd(t, "list")
	if err != nil {
		t.Fatalf("list 意外错误: %v", err)
	}
	if !strings.Contains(listOut, "orders") || !strings.Contains(listOut, "db") {
		t.Errorf("list 输出应含 db 与 orders，实际 %q", listOut)
	}
	showOut, err := runConfigCmd(t, "show")
	if err != nil {
		t.Fatalf("show 意外错误: %v", err)
	}
	if strings.Contains(showOut, "s3cret") {
		t.Errorf("show 输出泄漏密码，实际 %q", showOut)
	}
	if !strings.Contains(showOut, maskValue) {
		t.Errorf("show 输出应含 %q，实际 %q", maskValue, showOut)
	}
}

// mapKeysAny 提取 map 的 key 列表，便于错误信息定位。
func mapKeysAny(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
