package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"omni-cli/internal/config"
	"omni-cli/internal/db"
	"omni-cli/internal/output"
)

// newDBCmd 构造 db 子命令：执行 SQL 并输出结果。
func newDBCmd() *cobra.Command {
	var env, key, format, sqlStr, timeout string
	cmd := &cobra.Command{
		Use:   "db [SQL]",
		Short: "执行 SQL 查询并输出结果",
		Long: `按 key/env 从配置解析数据库连接（type=tcp，driver=mysql|postgres），执行一条 SQL 并输出对齐表格（默认）或 JSON。

params 含 ssh_host 时经 ssh 隧道访问目标（host:port 为跳板机视角地址，ssh_port 默认 22）。
SQL 可用 --sql 传入，或作为位置参数给出；两者都省略时报错。
无结果集（非 SELECT）时输出 Executed OK。`,
		Example: `  omni-cli db -k orders -e dev --sql "SELECT count(*) FROM users"
  omni-cli db -k orders -e dev "SELECT 1"
  omni-cli db -k orders -e dev -f json "SELECT 1"`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDB(cmd, env, key, format, sqlStr, timeout, args)
		},
	}
	cmd.Flags().StringVarP(&env, "env", "e", "", "环境名（省略时按 key 自动解析唯一 env）")
	cmd.Flags().StringVarP(&key, "key", "k", "", "连接 key（省略时按 env 自动解析唯一 key）")
	cmd.Flags().StringVarP(&format, "format", "f", formatTable, "输出格式：table | json")
	cmd.Flags().StringVar(&sqlStr, "sql", "", "要执行的 SQL（也可用位置参数传入）")
	cmd.Flags().StringVar(&timeout, "timeout", defaultTimeout, "命令超时（如 30s、1m）")
	cmd.AddCommand(newDBListCmd())
	return cmd
}

// runDB 执行 db 查询主流程：加载配置 → 解析条目 → 解析连接 → 超时上下文 → 查询 → 输出。
func runDB(cmd *cobra.Command, env, key, format, sqlStr, timeout string, args []string) error {
	if sqlStr == "" && len(args) == 1 {
		sqlStr = args[0]
	}
	if sqlStr == "" {
		return fmt.Errorf("缺少 SQL：用 --sql 或位置参数传入，例如 omni-cli db -k orders -e dev \"SELECT 1\"")
	}
	if err := checkFormat(format); err != nil {
		return err
	}
	d, err := time.ParseDuration(timeout)
	if err != nil {
		return fmt.Errorf("解析 --timeout %q: %w", timeout, err)
	}

	path, err := resolveConfigPath()
	if err != nil {
		return err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	entry, err := config.ResolveEntry(cfg.DB, key, env)
	if err != nil {
		return err
	}
	if entry.IsReadOnly() && db.IsWriteStatement(sqlStr) {
		return fmt.Errorf("只读连接禁止写操作；如需写请先执行 config set db --key %s --env %s --readonly=false", entry.Key, entry.Env)
	}

	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()

	tcp, closeConn, err := openConn(ctx, entry)
	if err != nil {
		return err
	}
	defer func() { _ = closeConn() }()

	res, err := db.Query(ctx, db.Driver(entry.Driver), tcp, sqlStr)
	if err != nil {
		return err
	}
	if len(res.Columns) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "Executed OK")
		return nil
	}
	if format == formatJSON {
		fmt.Fprintln(cmd.OutOrStdout(), output.JSON(res))
		return nil
	}
	fmt.Fprintln(cmd.OutOrStdout(), output.Table(res))
	return nil
}

// newDBListCmd 构造 db list 子命令：列出 key/env/driver。
func newDBListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "列出 db 连接条目（key/env/driver）",
		Long:    `列出配置中所有 db 连接条目的 key/env/driver，以对齐表格输出。`,
		Example: `  omni-cli db list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDBList(cmd)
		},
	}
}

// runDBList 加载配置并列出 db 条目；配置为空时报清晰错误。
func runDBList(cmd *cobra.Command) error {
	path, err := resolveConfigPath()
	if err != nil {
		return err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	if len(cfg.DB) == 0 {
		return fmt.Errorf("db 配置为空，未找到任何条目。请先添加：omni-cli config add db ...")
	}
	rows := make([][]any, 0, len(cfg.DB))
	for _, e := range cfg.DB {
		rows = append(rows, []any{e.Key, e.Env, e.Driver})
	}
	fmt.Fprintln(cmd.OutOrStdout(), output.Table(&db.Result{Columns: []string{"KEY", "ENV", "DRIVER"}, Rows: rows}))
	return nil
}

// checkFormat 校验输出格式取值。
func checkFormat(format string) error {
	if format != formatTable && format != formatJSON {
		return fmt.Errorf("未知输出格式 %q。支持：%s, %s", format, formatTable, formatJSON)
	}
	return nil
}
