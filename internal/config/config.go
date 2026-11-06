// Package config 负责全局配置的加载、校验与 ${VAR} 插值。
//
// 配置层级：命令类型（db / redis）→ 条目列表；每条 entry 由 key + env 复合唯一定位。
// 固定项 key / env / type（db 另加 driver）用 typed 结构体承接，连接字段进 params，
// 字段集由 type 决定。加载时 params 里的字符串值做 ${VAR} 环境变量插值，未设置即报错。
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"omni-cli/internal/conn"
)

// Config 是全局配置文件的结构：版本号 + 各命令类型的条目列表。
type Config struct {
	Version int     `yaml:"version"`
	DB      []Entry `yaml:"db"`
	Redis   []Entry `yaml:"redis"`
}

// Entry 是一条连接配置。Key + Env 复合唯一定位；Type 决定 params 字段集。
type Entry struct {
	Key    string         `yaml:"key"`
	Env    string         `yaml:"env"`
	Type   conn.ConnType  `yaml:"type"`
	Driver string         `yaml:"driver"`
	Params map[string]any `yaml:"params"`
}

// envVarPattern 匹配 ${VAR} 形式的占位符，VAR 为合法环境变量名。
var envVarPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// DefaultPath 返回默认配置文件路径 ~/.omni-cli/config.yaml。
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("获取用户主目录: %w", err)
	}
	return filepath.Join(home, ".omni-cli", "config.yaml"), nil
}

// Load 读取并校验配置文件：解析 YAML → 归一化（默认 type、${VAR} 插值）→ 校验。
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置 %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置 %s: %w", path, err)
	}
	if err := normalize(&cfg); err != nil {
		return nil, fmt.Errorf("配置 %s 归一化失败: %w", path, err)
	}
	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("配置 %s 校验失败: %w", path, err)
	}
	return &cfg, nil
}

// Save 把配置序列化为 YAML 并写入文件，权限 0600。
func Save(path string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("序列化配置: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("写入配置 %s: %w", path, err)
	}
	return nil
}

// normalize 填充默认值并对 params 字符串值做 ${VAR} 插值。
func normalize(cfg *Config) error {
	for i := range cfg.DB {
		if err := normalizeEntry(&cfg.DB[i]); err != nil {
			return fmt.Errorf("db 第 %d 条: %w", i, err)
		}
	}
	for i := range cfg.Redis {
		if err := normalizeEntry(&cfg.Redis[i]); err != nil {
			return fmt.Errorf("redis 第 %d 条: %w", i, err)
		}
	}
	return nil
}

// normalizeEntry 把空 type 置为 conn.ConnTypeTCP，并对 params 字符串值做插值。
func normalizeEntry(e *Entry) error {
	if e.Type == "" {
		e.Type = conn.ConnTypeTCP
	}
	for k, v := range e.Params {
		s, ok := v.(string)
		if !ok {
			continue
		}
		got, err := interpolate(s)
		if err != nil {
			return fmt.Errorf("参数 %s: %w", k, err)
		}
		e.Params[k] = got
	}
	return nil
}

// interpolate 把 s 中的 ${VAR} 替换为环境变量值；变量未设置即报错，不静默换空串。
func interpolate(s string) (string, error) {
	var missing []string
	out := envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		name := match[2 : len(match)-1]
		v, ok := os.LookupEnv(name)
		if !ok {
			missing = append(missing, name)
			return match
		}
		return v
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("环境变量未设置: %s", strings.Join(missing, ", "))
	}
	return out, nil
}

// validate 校验配置：版本、必填项、类型已知、同列表内 key+env 复合唯一。
func validate(cfg *Config) error {
	if cfg.Version != 1 {
		return fmt.Errorf("不支持的配置版本 %d，期望 1", cfg.Version)
	}
	if err := validateEntries("db", cfg.DB); err != nil {
		return err
	}
	if err := validateEntries("redis", cfg.Redis); err != nil {
		return err
	}
	return nil
}

// validateEntries 校验单个命令类型的条目列表。
func validateEntries(name string, entries []Entry) error {
	seen := map[string]bool{}
	for i := range entries {
		e := &entries[i]
		if e.Key == "" {
			return fmt.Errorf("%s 第 %d 条缺少 key", name, i)
		}
		if e.Env == "" {
			return fmt.Errorf("%s 第 %d 条缺少 env", name, i)
		}
		if !conn.IsKnownType(e.Type) {
			return unknownConnTypeError(e.Type)
		}
		compound := e.Key + "\x00" + e.Env
		if seen[compound] {
			return fmt.Errorf("%s 第 %d 条 key+env 重复: %s/%s", name, i, e.Key, e.Env)
		}
		seen[compound] = true
	}
	return nil
}

// unknownConnTypeError 构造面向 AI 的未知类型错误，列出所有支持的类型。
func unknownConnTypeError(typ conn.ConnType) error {
	names := make([]string, 0, len(conn.SupportedTypes()))
	for _, t := range conn.SupportedTypes() {
		names = append(names, string(t))
	}
	return fmt.Errorf("未知连接类型 %q。支持：%s", typ, strings.Join(names, ", "))
}
