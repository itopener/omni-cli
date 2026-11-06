// 命令入口：cobra 接线集中在本目录，业务逻辑一律放 internal/。
package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"omni-cli/internal/config"
	"omni-cli/internal/conn"
)

// 输出格式取值与超时默认值，集中定义避免魔法值散落。
const (
	formatTable    = "table"
	formatJSON     = "json"
	defaultTimeout = "30s"
)

// configPath 是 --config 持久 flag 绑定的变量，各子命令统一读取。
var configPath string

// rootCmd 是 omni-cli 根命令，挂载持久 flag --config 与 db / redis 子命令。
var rootCmd = &cobra.Command{
	Use:   "omni-cli",
	Short: "面向 AI 的研发场景 CLI 统一入口",
	Long: `omni-cli 用 Go 原生驱动直连 MySQL / PostgreSQL / Redis，执行 SQL 或 redis 命令并统一输出。

v1 提供两个子命令：
  db     按 key/env 解析数据库连接，执行 SQL，输出对齐表格或 JSON
  redis  按 key/env 解析 redis 连接，原样透传命令参数，输出结果

连接信息来自全局配置文件（默认 ~/.omni-cli/config.yaml，可用 --config 覆盖）。
详情见各子命令的 --help。`,
	Example: `  omni-cli db --help
  omni-cli redis --help`,
	SilenceErrors: true,
	SilenceUsage:  true,
}

// Execute 运行根命令，返回错误供 main 决定退出码。
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&configPath, "config", defaultConfigPath(), "配置文件路径（默认 ~/.omni-cli/config.yaml）")
	rootCmd.AddCommand(newDBCmd(), newRedisCmd())
}

// defaultConfigPath 返回默认配置路径；获取主目录失败时返回空串，由 config.Load 报清晰错误。
func defaultConfigPath() string {
	p, err := config.DefaultPath()
	if err != nil {
		return ""
	}
	return p
}

// resolveTCP 按 entry.Type 解析 params，并断言为 *conn.TCPParams。
func resolveTCP(entry *config.Entry) (*conn.TCPParams, error) {
	v, err := conn.Resolve(entry.Type, entry.Params)
	if err != nil {
		return nil, err
	}
	tcp, ok := v.(*conn.TCPParams)
	if !ok {
		return nil, fmt.Errorf("连接类型 %q 解析结果类型错误，期望 *conn.TCPParams", entry.Type)
	}
	return tcp, nil
}
