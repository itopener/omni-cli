package conn

import (
	"strings"
	"testing"
)

func TestResolveTCP(t *testing.T) {
	tests := []struct {
		name   string
		params map[string]any
		want   TCPParams
	}{
		{
			name: "完整字段",
			params: map[string]any{
				"host":     "127.0.0.1",
				"port":     3306,
				"user":     "root",
				"password": "test-password",
				"database": "orders",
			},
			want: TCPParams{
				Host:     "127.0.0.1",
				Port:     3306,
				User:     "root",
				Password: "test-password",
				Database: "orders",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAny, err := Resolve(ConnTypeTCP, tt.params)
			if err != nil {
				t.Fatalf("Resolve(%s) 意外错误: %v", ConnTypeTCP, err)
			}
			got, ok := gotAny.(*TCPParams)
			if !ok {
				t.Fatalf("Resolve(%s) = %T, 期望 *TCPParams", ConnTypeTCP, gotAny)
			}
			if got.Host != tt.want.Host ||
				got.Port != tt.want.Port ||
				got.User != tt.want.User ||
				got.Password != tt.want.Password ||
				got.Database != tt.want.Database ||
				got.DB != tt.want.DB {
				t.Errorf("Resolve(%s) = %+v, 期望 %+v", ConnTypeTCP, *got, tt.want)
			}
		})
	}
}

func TestResolveTCPCoercesStringPort(t *testing.T) {
	gotAny, err := Resolve(ConnTypeTCP, map[string]any{
		"host": "127.0.0.1",
		"port": "3306",
	})
	if err != nil {
		t.Fatalf("Resolve(tcp, port 字符串) 意外错误: %v", err)
	}
	got := gotAny.(*TCPParams)
	if got.Port != 3306 {
		t.Errorf("Port = %d, 期望 3306", got.Port)
	}
}

func TestResolveTCPNonNumericPort(t *testing.T) {
	_, err := Resolve(ConnTypeTCP, map[string]any{
		"host": "127.0.0.1",
		"port": "abc",
	})
	if err == nil {
		t.Fatal("期望非数字 port 报错，实际成功")
	}
	if !strings.Contains(err.Error(), "port") {
		t.Errorf("错误信息 %q 应包含 \"port\"", err.Error())
	}
}

func TestResolveTCPUnknownKeysToExtra(t *testing.T) {
	gotAny, err := Resolve(ConnTypeTCP, map[string]any{
		"host": "127.0.0.1",
		"port": 3306,
		"foo":  "bar",
	})
	if err != nil {
		t.Fatalf("Resolve(tcp, 未知 key) 意外错误: %v", err)
	}
	got := gotAny.(*TCPParams)
	if got.Extra["foo"] != "bar" {
		t.Errorf("Extra[%q] = %q, 期望 %q", "foo", got.Extra["foo"], "bar")
	}
}

func TestResolveUnknownType(t *testing.T) {
	_, err := Resolve("platform", map[string]any{})
	if err == nil {
		t.Fatal("期望未知类型报错，实际成功")
	}
	if !strings.Contains(err.Error(), "tcp") {
		t.Errorf("错误信息 %q 应包含 \"tcp\"", err.Error())
	}
}

func TestSchemaTCP(t *testing.T) {
	fields, err := Schema(ConnTypeTCP)
	if err != nil {
		t.Fatalf("Schema(tcp) 意外错误: %v", err)
	}
	required := map[string]bool{}
	for _, f := range fields {
		required[f.Name] = f.Required
	}
	if !required["host"] {
		t.Errorf("schema 应包含 host 且 Required=true，实际 %v", required["host"])
	}
	if !required["port"] {
		t.Errorf("schema 应包含 port 且 Required=true，实际 %v", required["port"])
	}
}
