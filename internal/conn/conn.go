// Package conn 提供连接方式（type）的类型定义与解析策略注册表。
//
// 每种连接方式对应一个 Resolver，负责把原始 params 解析为 typed 参数结构体。
// v1 只注册直连（tcp）；新增连接方式（如 platform）时实现 Resolver 并 Register，
// 不改动已有类型。
package conn

import (
	"fmt"
	"sort"
	"strings"
)

// ConnType 标识一种连接方式（如 tcp）。
type ConnType string

// v1 支持的连接方式。
const (
	// ConnTypeTCP 直连（host/port 方式）。
	ConnTypeTCP ConnType = "tcp"
)

// ParamField 描述连接方式的一个参数字段。
type ParamField struct {
	// Name 字段名，与 params 的 key 一致。
	Name string
	// Desc 字段说明，用于 --help 与 config schema。
	Desc string
	// Required 是否为必填字段。
	Required bool
}

// Resolver 是连接方式的解析策略，也是本包唯一的策略缝。
// 消费者（db / redis）按 ConnType 分发，不写判断分支。
type Resolver interface {
	// Type 返回本 resolver 支持的连接方式。
	Type() ConnType
	// Resolve 把原始 params 解析为 typed 参数结构体。
	Resolve(params map[string]any) (any, error)
	// Schema 返回本连接方式的参数字段清单。
	Schema() []ParamField
}

// registry 保存已注册的 resolver，按 ConnType 索引。
var registry = map[ConnType]Resolver{}

// Register 注册一个连接方式 resolver。同类型重复注册时后注册者覆盖。
func Register(r Resolver) {
	registry[r.Type()] = r
}

// Resolve 用 typ 对应的 resolver 解析 params，返回 typed 参数结构体。
func Resolve(typ ConnType, params map[string]any) (any, error) {
	r, ok := registry[typ]
	if !ok {
		return nil, unknownTypeError(typ)
	}
	return r.Resolve(params)
}

// Schema 返回 typ 对应的参数字段清单。
func Schema(typ ConnType) ([]ParamField, error) {
	r, ok := registry[typ]
	if !ok {
		return nil, unknownTypeError(typ)
	}
	return r.Schema(), nil
}

// SupportedTypes 返回所有已注册的连接方式（升序，输出确定）。
func SupportedTypes() []ConnType {
	types := make([]ConnType, 0, len(registry))
	for t := range registry {
		types = append(types, t)
	}
	sort.Slice(types, func(i, j int) bool { return types[i] < types[j] })
	return types
}

// IsKnownType 报告 typ 是否为已注册的连接方式。
func IsKnownType(typ ConnType) bool {
	_, ok := registry[typ]
	return ok
}

// unknownTypeError 构造面向 AI 的未知类型错误，列出所有支持的类型。
func unknownTypeError(typ ConnType) error {
	names := make([]string, 0, len(registry))
	for _, t := range SupportedTypes() {
		names = append(names, string(t))
	}
	return fmt.Errorf("未知连接类型 %q。支持：%s", typ, strings.Join(names, ", "))
}
