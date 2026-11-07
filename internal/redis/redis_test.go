package redis

import (
	"testing"

	"github.com/itopener/omni-cli/internal/conn"
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

func TestIsWriteCommand(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "GET", args: []string{"GET", "k"}, want: false},
		{name: "小写 get", args: []string{"get", "k"}, want: false},
		{name: "SET", args: []string{"SET", "k", "v"}, want: true},
		{name: "DEL", args: []string{"DEL", "k"}, want: true},
		{name: "EVAL", args: []string{"EVAL", "..."}, want: true},
		{name: "空参数", args: []string{}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsWriteCommand(tt.args); got != tt.want {
				t.Errorf("IsWriteCommand(%v) = %v, 期望 %v", tt.args, got, tt.want)
			}
		})
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
