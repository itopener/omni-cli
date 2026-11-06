package redis

import (
	"testing"

	"omni-cli/internal/conn"
)

func TestOptions(t *testing.T) {
	got, err := Options(&conn.TCPParams{Host: "h", Port: 6379, Password: "p", DB: 2})
	if err != nil {
		t.Fatalf("Options 意外错误: %v", err)
	}
	if got.Addr != "h:6379" {
		t.Errorf("Addr = %q, 期望 %q", got.Addr, "h:6379")
	}
	if got.Password != "p" {
		t.Errorf("Password = %q, 期望 %q", got.Password, "p")
	}
	if got.DB != 2 {
		t.Errorf("DB = %d, 期望 %d", got.DB, 2)
	}
	if got.PoolSize != 1 {
		t.Errorf("PoolSize = %d, 期望 %d", got.PoolSize, 1)
	}
}

func TestOptionsEmptyPassword(t *testing.T) {
	got, err := Options(&conn.TCPParams{Host: "h", Port: 6379})
	if err != nil {
		t.Fatalf("Options 意外错误: %v", err)
	}
	if got.Password != "" {
		t.Errorf("Password = %q, 期望空串", got.Password)
	}
}
