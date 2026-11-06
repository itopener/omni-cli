// 命令入口：cobra 接线集中在本目录，业务逻辑一律放 internal/。
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"omni-cli/internal/config"
	"omni-cli/internal/conn"
)

// 输出格式取值与超时默认值，集中定义避免魔法值散落。
const (
	formatTable    = "table"
	formatJSON     = "json"
	defaultTimeout = "30s"
	// configEnvVar 是配置文件路径的环境变量覆盖名（等价 --config）。
	configEnvVar = "OMNI_CLI_CONFIG"
)

// configPath 是 --config 持久 flag 绑定的变量，各子命令统一读取。
var configPath string

// version 是当前版本号，默认 dev，构建时可用
// -ldflags "-X main.version=v1.0.0" 注入（main 包符号名固定为 main）。
var version = "dev"

// rootCmd 是 omni-cli 根命令，挂载持久 flag --config 与 db / redis 子命令。
var rootCmd = &cobra.Command{
	Use:   "omni-cli",
	Short: "面向 AI 的研发场景 CLI 统一入口",
	Long: `omni-cli 用 Go 原生驱动直连 MySQL / PostgreSQL / Redis，执行 SQL 或 redis 命令并统一输出。

v1 提供以下子命令：
  config 管理全局配置文件（init/add/get/list/show/schema 等）
  db     按 key/env 解析数据库连接，执行 SQL，输出对齐表格或 JSON
  redis  按 key/env 解析 redis 连接，原样透传命令参数，输出结果
  version 打印版本信息（等价 --version flag）

连接信息来自全局配置文件（默认 ~/.omni-cli/config.yaml，可用 --config 或 OMNI_CLI_CONFIG 覆盖）。
详情见各子命令的 --help。`,
	Example: `  omni-cli config init
  omni-cli config add db --key orders --env dev --driver mysql --param host=127.0.0.1
  omni-cli db --help
  omni-cli redis --help`,
	SilenceErrors: true,
	SilenceUsage:  true,
}

// Execute 运行根命令，返回错误供 main 决定退出码。
// 每次执行前同步 rootCmd.Version，保证构建时注入的 version 生效。
func Execute() error {
	rootCmd.Version = version
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "", "配置文件路径（默认 ~/.omni-cli/config.yaml，可用 OMNI_CLI_CONFIG 覆盖）")
	rootCmd.AddCommand(newDBCmd(), newRedisCmd(), newConfigCmd(), newVersionCmd())
}

// newVersionCmd 返回 version 子命令，输出与 --version flag 一致的版本信息。
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "打印 omni-cli 版本",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s version %s\n", cmd.Root().DisplayName(), version)
			return err
		},
	}
}

// resolveConfigPath 返回生效的配置文件路径：--config 优先，其次 OMNI_CLI_CONFIG，最后默认路径。
func resolveConfigPath() (string, error) {
	if configPath != "" {
		return configPath, nil
	}
	if env := os.Getenv(configEnvVar); env != "" {
		return env, nil
	}
	return config.DefaultPath()
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
