// Package schema 从代码生成机器可读的配置字段清单（JSON）。
//
// 固定项 key/env/type（db 命令另加 driver）由本包常量拼出；params 字段集
// 遍历 conn.SupportedTypes() 调用 conn.Schema 获得，不手写 JSON。
package schema

import (
	"fmt"

	"omni-cli/internal/conn"
)

// 固定项字段名，集中定义避免魔法值散落。
const (
	fieldKey    = "key"
	fieldEnv    = "env"
	fieldType   = "type"
	fieldDriver = "driver"
)

// Field 描述一个固定项字段。
type Field struct {
	Name     string `json:"name"`
	Desc     string `json:"description"`
	Required bool   `json:"required"`
}

// CommandSchema 是一个命令类型的字段清单：固定项 + 各连接方式的 params 字段。
type CommandSchema struct {
	Fixed        []Field                             `json:"fixed"`
	ParamsByType map[conn.ConnType][]conn.ParamField `json:"params_by_type"`
}

// Build 生成命令类型的字段清单。hasDriver 为 true 时固定项含 driver（db 命令）。
func Build(hasDriver bool) (*CommandSchema, error) {
	fixed := []Field{
		{Name: fieldKey, Desc: "连接标识，与 env 复合唯一定位", Required: true},
		{Name: fieldEnv, Desc: "环境名，与 key 复合唯一定位", Required: true},
		{Name: fieldType, Desc: "连接方式（v1 仅 tcp）", Required: true},
	}
	if hasDriver {
		fixed = append(fixed, Field{Name: fieldDriver, Desc: "数据库驱动（mysql | postgres）", Required: true})
	}

	params := make(map[conn.ConnType][]conn.ParamField, len(conn.SupportedTypes()))
	for _, typ := range conn.SupportedTypes() {
		fields, err := conn.Schema(typ)
		if err != nil {
			return nil, fmt.Errorf("获取 %s 参数清单: %w", typ, err)
		}
		params[typ] = fields
	}

	return &CommandSchema{Fixed: fixed, ParamsByType: params}, nil
}
