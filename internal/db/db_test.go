package db

import (
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"

	"omni-cli/internal/conn"
)

func TestDSNMySQL(t *testing.T) {
	tests := []struct {
		name string
		p    *conn.TCPParams
		want string
	}{
		{
			name: "基本",
			p:    &conn.TCPParams{Host: "h", Port: 3306, User: "u", Password: "p", Database: "d"},
			want: "u:p@tcp(h:3306)/d",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dsn, err := DSN(DriverMySQL, tt.p)
			if err != nil {
				t.Fatalf("DSN(mysql, %+v) 意外错误: %v", *tt.p, err)
			}
			if !strings.Contains(dsn, tt.want) {
				t.Errorf("DSN(mysql) = %q, 期望包含 %q", dsn, tt.want)
			}
		})
	}
}

func TestDSNMySQLEscapesSpecialChars(t *testing.T) {
	p := &conn.TCPParams{Host: "h", Port: 3306, User: "u", Password: "p@ss:/x", Database: "d"}
	dsn, err := DSN(DriverMySQL, p)
	if err != nil {
		t.Fatalf("DSN(mysql) 意外错误: %v", err)
	}
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("mysql.ParseDSN(%q) 意外错误: %v", dsn, err)
	}
	if cfg.Passwd != "p@ss:/x" {
		t.Errorf("ParseDSN.Passwd = %q, 期望 %q", cfg.Passwd, "p@ss:/x")
	}
}

func TestDSNPostgres(t *testing.T) {
	tests := []struct {
		name string
		p    *conn.TCPParams
		want string
	}{
		{
			name: "基本",
			p:    &conn.TCPParams{Host: "h", Port: 3306, User: "u", Password: "p", Database: "d"},
			want: "postgres://u:p@h:3306/d",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dsn, err := DSN(DriverPostgres, tt.p)
			if err != nil {
				t.Fatalf("DSN(postgres, %+v) 意外错误: %v", *tt.p, err)
			}
			if !strings.Contains(dsn, tt.want) {
				t.Errorf("DSN(postgres) = %q, 期望包含 %q", dsn, tt.want)
			}
		})
	}
}

// TestDSNPostgresSpecialCharPassword 验证含 @ : / ? = 的密码经 URL 编码后可被 pgx 回读（R3）。
func TestDSNPostgresSpecialCharPassword(t *testing.T) {
	p := &conn.TCPParams{Host: "h", Port: 5432, User: "u", Password: "p@ss:/x?y=z", Database: "d"}
	dsn, err := DSN(DriverPostgres, p)
	if err != nil {
		t.Fatalf("DSN(postgres) 意外错误: %v", err)
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("pgx.ParseConfig(%q) 意外错误: %v", dsn, err)
	}
	if cfg.Password != p.Password {
		t.Errorf("ParseConfig.Password = %q, 期望 %q", cfg.Password, p.Password)
	}
}

func TestOpenPoolLimited(t *testing.T) {
	p := &conn.TCPParams{Host: "h", Port: 3306, User: "u", Password: "p", Database: "d"}
	db, err := Open(DriverMySQL, p)
	if err != nil {
		t.Fatalf("Open(mysql) 意外错误: %v", err)
	}
	defer db.Close()
	if got := db.Stats().MaxOpenConnections; got != 1 {
		t.Errorf("MaxOpenConnections = %d, 期望 1", got)
	}
}

func TestUnknownDriver(t *testing.T) {
	p := &conn.TCPParams{Host: "h", Port: 3306, User: "u", Password: "p", Database: "d"}
	_, err := DSN(Driver("oracle"), p)
	if err == nil {
		t.Fatal("期望未知 driver 报错，实际成功")
	}
	if !strings.Contains(err.Error(), "mysql") && !strings.Contains(err.Error(), "postgres") {
		t.Errorf("错误信息 %q 应包含 mysql 或 postgres", err.Error())
	}
}

func TestIsKnownDriver(t *testing.T) {
	tests := []struct {
		d    Driver
		want bool
	}{
		{DriverMySQL, true},
		{DriverPostgres, true},
		{Driver("oracle"), false},
		{Driver(""), false},
	}
	for _, tt := range tests {
		if got := IsKnownDriver(tt.d); got != tt.want {
			t.Errorf("IsKnownDriver(%q) = %v, 期望 %v", tt.d, got, tt.want)
		}
	}
}
