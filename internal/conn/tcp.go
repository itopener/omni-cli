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

	// ssh_ 前缀字段是可选 ssh 隧道参数：ssh_host 存在时经跳板机转发到 host:port。
	fieldSSHHost     = "ssh_host"
	fieldSSHPort     = "ssh_port"
	fieldSSHUser     = "ssh_user"
	fieldSSHPassword = "ssh_password"
)

// DefaultSSHPort 是 ssh_port 省略时的默认端口。
const DefaultSSHPort = 22

// TCPParams 是 type=tcp 的连接参数。
//
// host/port/user/password 为通用直连字段；database 供 db 命令使用（库名），
// db 供 redis 命令使用（数据库索引）。SSHHost/SSHPort/SSHUser/SSHPassword 为
// 可选 ssh 隧道字段：SSHHost 非空时先建立到跳板机的 ssh 连接，再经隧道访问
// host:port；此时 host:port 是跳板机视角的目标地址。未识别的 key 宽松保留在 Extra。
type TCPParams struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Database string `yaml:"database"`
	DB       int    `yaml:"db"`
	// SSHHost 跳板机地址；空串表示不经隧道直连。
	SSHHost string `yaml:"ssh_host"`
	// SSHPort 跳板机 ssh 端口；0 视为默认 22。
	SSHPort int `yaml:"ssh_port"`
	// SSHUser / SSHPassword 跳板机认证信息，只进内存 ssh 配置，不打印。
	SSHUser     string `yaml:"ssh_user"`
	SSHPassword string `yaml:"ssh_password"`
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
		case fieldSSHHost:
			out.SSHHost = asString(v)
		case fieldSSHPort:
			n, err := asInt(v)
			if err != nil {
				return nil, fmt.Errorf("参数 ssh_port 必须为数字: %w", err)
			}
			out.SSHPort = n
		case fieldSSHUser:
			out.SSHUser = asString(v)
		case fieldSSHPassword:
			out.SSHPassword = asString(v)
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
		{Name: fieldSSHHost, Desc: "可选 ssh 跳板机地址，非空时经 ssh 隧道访问 host:port", Required: false},
		{Name: fieldSSHPort, Desc: "ssh 跳板机端口（默认 22）", Required: false},
		{Name: fieldSSHUser, Desc: "ssh 跳板机用户名（ssh_host 非空时必填）", Required: false},
		{Name: fieldSSHPassword, Desc: "ssh 跳板机密码（ssh_host 非空时必填）", Required: false},
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
