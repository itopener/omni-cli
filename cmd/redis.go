package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"omni-cli/internal/config"
	"omni-cli/internal/db"
	"omni-cli/internal/output"
	"omni-cli/internal/redis"
)

// newRedisCmd 构造 redis 子命令：原样透传命令参数并输出结果。
func newRedisCmd() *cobra.Command {
	var env, key, format, timeout string
	cmd := &cobra.Command{
		Use:   "redis [args...]",
		Short: "执行 redis 命令",
		Long: `按 key/env 从配置解析 redis 连接（type=tcp），把位置参数原样透传给 redis 执行并输出结果。

例如 GET / SET / INCR 等命令。默认直接打印返回值，-f json 输出 JSON。`,
		Example: `  omni-cli redis -k cache -e dev GET user:1
  omni-cli redis -k cache -e dev SET k v
  omni-cli redis -k cache -e dev -f json GET user:1`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("缺少 redis 命令参数，例如 omni-cli redis -k cache GET user:1")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRedis(cmd, env, key, format, timeout, args)
		},
	}
	cmd.Flags().StringVarP(&env, "env", "e", "", "环境名（省略时按 key 自动解析唯一 env）")
	cmd.Flags().StringVarP(&key, "key", "k", "", "连接 key（省略时按 env 自动解析唯一 key）")
	cmd.Flags().StringVarP(&format, "format", "f", formatTable, "输出格式：table | json（table 即直接打印返回值）")
	cmd.Flags().StringVar(&timeout, "timeout", defaultTimeout, "命令超时（如 30s、1m）")
	cmd.AddCommand(newRedisListCmd())
	return cmd
}

// runRedis 执行 redis 命令主流程：加载配置 → 解析条目 → 解析连接 → 超时上下文 → 执行 → 输出。
func runRedis(cmd *cobra.Command, env, key, format, timeout string, args []string) error {
	if err := checkFormat(format); err != nil {
		return err
	}
	d, err := time.ParseDuration(timeout)
	if err != nil {
		return fmt.Errorf("解析 --timeout %q: %w", timeout, err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	entry, err := config.ResolveEntry(cfg.Redis, key, env)
	if err != nil {
		return err
	}
	tcp, err := resolveTCP(entry)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()

	val, err := redis.Exec(ctx, tcp, args)
	if err != nil {
		return err
	}
	if format == formatJSON {
		fmt.Fprintln(cmd.OutOrStdout(), output.JSON(val))
		return nil
	}
	fmt.Fprintln(cmd.OutOrStdout(), val)
	return nil
}

// newRedisListCmd 构造 redis list 子命令：列出 key/env。
func newRedisListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "列出 redis 连接条目（key/env）",
		Long:    `列出配置中所有 redis 连接条目的 key/env，以对齐表格输出。`,
		Example: `  omni-cli redis list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRedisList(cmd)
		},
	}
}

// runRedisList 加载配置并列出 redis 条目；配置为空时报清晰错误。
func runRedisList(cmd *cobra.Command) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	if len(cfg.Redis) == 0 {
		return fmt.Errorf("redis 配置为空，未找到任何条目。请先添加：omni-cli config add redis ...")
	}
	rows := make([][]any, 0, len(cfg.Redis))
	for _, e := range cfg.Redis {
		rows = append(rows, []any{e.Key, e.Env})
	}
	fmt.Fprintln(cmd.OutOrStdout(), output.Table(&db.Result{Columns: []string{"KEY", "ENV"}, Rows: rows}))
	return nil
}
