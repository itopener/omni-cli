package db

import (
	"reflect"
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

// 回归：DSN 解析后必须允许 mysql_native_password 认证。
// 零值 mysql.Config 会静默禁用它，老账号（native password）全部连不上。
func TestDSNMySQLAllowsNativePassword(t *testing.T) {
	tests := []struct {
		name string
		p    *conn.TCPParams
	}{
		{
			name: "基本",
			p:    &conn.TCPParams{Host: "h", Port: 3306, User: "u", Password: "p", Database: "d"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dsn, err := DSN(DriverMySQL, tt.p)
			if err != nil {
				t.Fatalf("DSN(mysql) 意外错误: %v", err)
			}
			cfg, err := mysql.ParseDSN(dsn)
			if err != nil {
				t.Fatalf("mysql.ParseDSN(%q) 意外错误: %v", dsn, err)
			}
			if !cfg.AllowNativePasswords {
				t.Errorf("ParseDSN.AllowNativePasswords = false, 期望 true（DSN=%q）", dsn)
			}
		})
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

func TestIsWriteStatement(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want bool
	}{
		{name: "SELECT", sql: "SELECT 1", want: false},
		{name: "前导空白 select", sql: "  select 1", want: false},
		{name: "SHOW TABLES", sql: "SHOW TABLES", want: false},
		{name: "INSERT", sql: "INSERT INTO t VALUES(1)", want: true},
		{name: "前导空白 update", sql: "  update t set x=1", want: true},
		{name: "DROP", sql: "DROP TABLE t", want: true},
		{name: "空串", sql: "", want: false},
		{name: "纯空白", sql: "   ", want: false},
		{name: "行注释保守判写", sql: "-- comment", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsWriteStatement(tt.sql); got != tt.want {
				t.Errorf("IsWriteStatement(%q) = %v, 期望 %v", tt.sql, got, tt.want)
			}
		})
	}
}

// TestIsWriteStatementMultiStatement 回归：多语句 SQL 只按首关键字分类。
//
// 首关键字 SELECT 判读；尾随的 DROP 由驱动默认禁用多语句兜底
// （go-sql-driver multiStatements=false、pgx 单语句），不会绕过首关键字检查。
func TestIsWriteStatementMultiStatement(t *testing.T) {
	if got := IsWriteStatement("SELECT 1; DROP TABLE t"); got {
		t.Errorf("IsWriteStatement(%q) = true，期望 false（首关键字 SELECT）", "SELECT 1; DROP TABLE t")
	}
}

// TestNormalizeRow 回归：扫描得到的 []byte 归一化为 string，避免 JSON 输出 base64；nil 保持 nil。
func TestNormalizeRow(t *testing.T) {
	tests := []struct {
		name string
		in   []any
		want []any
	}{
		{name: "[]byte 转 string", in: []any{[]byte("hello")}, want: []any{"hello"}},
		{name: "nil 保持 nil", in: []any{nil}, want: []any{nil}},
		{name: "nil []byte 转 nil", in: []any{[]byte(nil)}, want: []any{nil}},
		{name: "混合类型", in: []any{int64(1), []byte("a"), nil, 2.5}, want: []any{int64(1), "a", nil, 2.5}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalizeRow(tt.in)
			if !reflect.DeepEqual(tt.in, tt.want) {
				t.Errorf("normalizeRow() = %#v, 期望 %#v", tt.in, tt.want)
			}
		})
	}
}
