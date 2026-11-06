package conn

import (
	"fmt"
	"strconv"
)

// tcp 连接方式的参数字段名，集中定义避免魔法值散落。
// 字段名只在此处与 TCPParams 的 yaml tag 出现。
const (
	fieldHost     = "host"
	fieldPort     = "port"
	fieldUser     = "user"
	fieldPassword = "password"
	fieldDatabase = "database"
	fieldDB       = "db"
)

// TCPParams 是 type=tcp 的连接参数。
//
// host/port/user/password 为通用直连字段；database 供 db 命令使用（库名），
// db 供 redis 命令使用（数据库索引）。未识别的 key 宽松保留在 Extra。
type TCPParams struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Database string `yaml:"database"`
	DB       int    `yaml:"db"`
	// Extra 保存 params 中未显式声明的 key，加字段不改代码。
	Extra map[string]string
}

// tcpResolver 实现 type=tcp 的 Resolver。
type tcpResolver struct{}

func init() {
	Register(tcpResolver{})
}

// Type 返回 ConnTypeTCP。
func (tcpResolver) Type() ConnType { return ConnTypeTCP }

// Resolve 提取 tcp 字段；port/db 用 strconv.Atoi 强转，未识别 key 入 Extra。
func (tcpResolver) Resolve(params map[string]any) (any, error) {
	out := &TCPParams{Extra: map[string]string{}}
	for k, v := range params {
		switch k {
		case fieldHost:
			out.Host = asString(v)
		case fieldPort:
			n, err := asInt(v)
			if err != nil {
				return nil, fmt.Errorf("参数 port 必须为数字: %w", err)
			}
			out.Port = n
		case fieldUser:
			out.User = asString(v)
		case fieldPassword:
			out.Password = asString(v)
		case fieldDatabase:
			out.Database = asString(v)
		case fieldDB:
			n, err := asInt(v)
			if err != nil {
				return nil, fmt.Errorf("参数 db 必须为数字: %w", err)
			}
			out.DB = n
		default:
			out.Extra[k] = asString(v)
		}
	}
	return out, nil
}

// Schema 返回 tcp 的参数字段清单。
func (tcpResolver) Schema() []ParamField {
	return []ParamField{
		{Name: fieldHost, Desc: "主机地址", Required: true},
		{Name: fieldPort, Desc: "端口号", Required: true},
		{Name: fieldUser, Desc: "用户名", Required: false},
		{Name: fieldPassword, Desc: "密码", Required: false},
		{Name: fieldDatabase, Desc: "数据库名（db 命令）", Required: false},
		{Name: fieldDB, Desc: "数据库索引（redis 命令）", Required: false},
	}
}

// asString 把任意值转为字符串；nil 转空串。
func asString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// asInt 把任意值强转为 int；字符串用 strconv.Atoi 解析。
func asInt(v any) (int, error) {
	switch n := v.(type) {
	case int:
		return n, nil
	case int64:
		return int(n), nil
	case float64:
		return int(n), nil
	case string:
		return strconv.Atoi(n)
	default:
		return 0, fmt.Errorf("无法将 %T 转为数字", v)
	}
}
