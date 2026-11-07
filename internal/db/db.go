// Package db 用 Go 原生 database/sql 直连 MySQL / PostgreSQL，执行 SQL 并统一输出。
//
// v1 只支持 mysql 与 postgres 两个 driver。DSN 由 conn.TCPParams 构造：
// mysql 走 driver 自带 Config.FormatDSN()，postgres 走 URL 编码（密码含 @ : / 等
// 特殊字符不损坏）。密码只存在于 DSN 字符串，不打印、不落日志。
package db

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-sql-driver/mysql"
	// pgx 顶层包不会自动注册 driver，此处空白导入 stdlib 使 sql.Open("pgx", dsn) 可解析。
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/itopener/omni-cli/internal/conn"
)

// Driver 标识一种数据库驱动。
type Driver string

// v1 支持的数据库驱动。
const (
	// DriverMySQL MySQL（原生 go-sql-driver/mysql）。
	DriverMySQL Driver = "mysql"
	// DriverPostgres PostgreSQL（原生 pgx stdlib）。
	DriverPostgres Driver = "postgres"
)

// driverName 返回 database/sql 注册的驱动名。d 未知时返回空串，由调用方先做校验。
func driverName(d Driver) string {
	switch d {
	case DriverMySQL:
		return "mysql"
	case DriverPostgres:
		return "pgx"
	}
	return ""
}

// IsKnownDriver 报告 d 是否为已知驱动。
func IsKnownDriver(d Driver) bool {
	switch d {
	case DriverMySQL, DriverPostgres:
		return true
	}
	return false
}

// readOnlyStatements 是只读 SQL 的首词白名单，集中定义避免魔法值散落。
var readOnlyStatements = map[string]bool{
	"SELECT":   true,
	"SHOW":     true,
	"DESCRIBE": true,
	"DESC":     true,
	"EXPLAIN":  true,
}

// IsWriteStatement 按首词判断 SQL 是否为写操作。
//
// 去掉前导空白后取第一个空白分隔的词并转大写；不在只读白名单内一律视为写
// （含空首词以外的注释、DDL、DML 等），空/纯空白 SQL 返回 false（无可执行内容）。
func IsWriteStatement(sql string) bool {
	first := firstToken(strings.ToUpper(strings.TrimSpace(sql)))
	if first == "" {
		return false
	}
	return !readOnlyStatements[first]
}

// firstToken 返回 s 的第一个空白分隔词；s 为空返回空串。
func firstToken(s string) string {
	if idx := strings.IndexAny(s, " \t\r\n"); idx >= 0 {
		return s[:idx]
	}
	return s
}

// unknownDriverError 构造面向 AI 的未知驱动错误，列出所有合法选项。
func unknownDriverError(d Driver) error {
	return fmt.Errorf("未知 driver %q。支持：%s, %s", d, DriverMySQL, DriverPostgres)
}

// DSN 按 driver 构造连接串。未知 driver 报错并列出合法选项。
func DSN(d Driver, p *conn.TCPParams) (string, error) {
	switch d {
	case DriverMySQL:
		// NewConfig 给驱动默认值（AllowNativePasswords 等）；零值 Config 会
		// 拒绝 mysql_native_password 认证，老账号连不上。
		cfg := mysql.NewConfig()
		cfg.User = p.User
		cfg.Passwd = p.Password
		cfg.Net = "tcp"
		cfg.Addr = net.JoinHostPort(p.Host, strconv.Itoa(p.Port))
		cfg.DBName = p.Database
		return cfg.FormatDSN(), nil
	case DriverPostgres:
		// 密码/用户名必须 URL 编码，避免 @ : / 等字符破坏 DSN。
		u := url.URL{
			Scheme: "postgres",
			User:   url.UserPassword(p.User, p.Password),
			Host:   net.JoinHostPort(p.Host, strconv.Itoa(p.Port)),
			Path:   "/" + p.Database,
		}
		return u.String(), nil
	default:
		return "", unknownDriverError(d)
	}
}

// Open 打开数据库连接池。单次命令池大小固定为 1（SetMaxOpenConns(1)）。
func Open(d Driver, p *conn.TCPParams) (*sql.DB, error) {
	dsn, err := DSN(d, p)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open(driverName(d), dsn)
	if err != nil {
		return nil, fmt.Errorf("打开 %s 连接失败: %w", d, err)
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

// Result 是查询结果的列名与行值（值统一用 any，便于统一输出）。
type Result struct {
	Columns []string
	Rows    [][]any
}

// Query 打开连接执行 query，扫列名与行值到 Result 后关闭连接。
// 无结果集（非 SELECT）时返回空 Result，调用方输出「Executed OK」。
func Query(ctx context.Context, d Driver, p *conn.TCPParams, query string) (*Result, error) {
	db, err := Open(d, p)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("查询失败: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("读取列名失败: %w", err)
	}

	res := &Result{Columns: cols, Rows: [][]any{}}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("扫描行失败: %w", err)
		}
		normalizeRow(vals)
		res.Rows = append(res.Rows, vals)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历结果失败: %w", err)
	}
	return res, nil
}

// normalizeRow 把行值中的 []byte 归一化为 string，nil 保持 nil。
//
// go-sql-driver 把字符串列扫描为 []byte，encoding/json 会把 []byte 编码成
// base64。这里统一转 string，保证 JSON 输出与表格一致。
func normalizeRow(vals []any) {
	for i, v := range vals {
		b, ok := v.([]byte)
		if !ok {
			continue
		}
		if b == nil {
			vals[i] = nil
			continue
		}
		vals[i] = string(b)
	}
}
