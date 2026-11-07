package main

import (
	"path/filepath"
	"testing"

	"github.com/itopener/omni-cli/internal/config"
)

// TestResolveConfigPathPrecedence 回归：配置路径解析优先级为 --config > OMNI_CLI_CONFIG > 默认路径。
func TestResolveConfigPathPrecedence(t *testing.T) {
	defaultPath, err := config.DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath 意外错误: %v", err)
	}
	flagPath := filepath.Join(t.TempDir(), "flag.yaml")
	envPath := filepath.Join(t.TempDir(), "env.yaml")

	old := configPath
	t.Cleanup(func() { configPath = old })

	// flag 优先于 env。
	configPath = flagPath
	t.Setenv(configEnvVar, envPath)
	if got, err := resolveConfigPath(); err != nil || got != flagPath {
		t.Errorf("resolveConfigPath(flag) = %q, %v; 期望 %q", got, err, flagPath)
	}

	// env 优先于默认路径。
	configPath = ""
	if got, err := resolveConfigPath(); err != nil || got != envPath {
		t.Errorf("resolveConfigPath(env) = %q, %v; 期望 %q", got, err, envPath)
	}

	// 均未设置时用默认路径。
	t.Setenv(configEnvVar, "")
	if got, err := resolveConfigPath(); err != nil || got != defaultPath {
		t.Errorf("resolveConfigPath(default) = %q, %v; 期望 %q", got, err, defaultPath)
	}
}
