// config 子命令：管理全局配置文件（init/path/add/set/remove/get/list/show/schema）。
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/itopener/omni-cli/internal/config"
	"github.com/itopener/omni-cli/internal/conn"
	"github.com/itopener/omni-cli/internal/db"
	"github.com/itopener/omni-cli/internal/output"
	"github.com/itopener/omni-cli/internal/schema"
)

// config 子命令的命令类型段名与打码值，集中定义避免魔法值散落。
const (
	sectionDB    = "db"
	sectionRedis = "redis"
	// maskValue 是敏感参数值打码后的占位符。
	maskValue = "***"
)

// configTemplate 是 init 生成的配置模板（带注释，version: 1，空列表）。
const configTemplate = `# omni-cli 全局配置
# 配置层级：命令类型（db / redis）→ 条目列表；每条 entry 由 key + env 复合唯一定位。
# 固定项 key / env / type（db 另加 driver），其余连接字段进 params。
# params 字符串值支持 ${VAR} 环境变量插值。
version: 1
db: []
redis: []
`

// newConfigCmd 构造 config 子命令，挂载各管理子命令。
func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "管理全局配置（~/.omni-cli/config.yaml）",
		Long: `管理 omni-cli 的全局配置文件（默认 ~/.omni-cli/config.yaml，可用 --config 或 OMNI_CLI_CONFIG 覆盖）。

配置层级：命令类型（db / redis）→ 条目列表；每条 entry 由 key + env 复合唯一定位。
固定项 key / env / type（db 另加 driver），其余连接字段进 params。

子命令：
  init     生成带注释的配置模板（已存在则跳过）
  path     打印生效的配置文件路径
  add      新增一条连接配置
  set      覆盖指定条目的字段
  remove   删除一条连接配置
  get      按 key/env 打印单条配置（敏感字段打码）
  list     列出全部连接配置
  show     打印完整配置（敏感字段打码）
  schema   打印某命令类型的字段清单（JSON，机器可读）`,
		Example: `  omni-cli config init
  omni-cli config add db --key orders --env dev --driver mysql \
    --param host=127.0.0.1 --param port=3306 --param user=root --param database=orders
  omni-cli config get db --key orders --env dev
  omni-cli config list
  omni-cli config schema db`,
	}
	cmd.AddCommand(
		newConfigInitCmd(),
		newConfigPathCmd(),
		newConfigAddCmd(),
		newConfigSetCmd(),
		newConfigRemoveCmd(),
		newConfigGetCmd(),
		newConfigListCmd(),
		newConfigShowCmd(),
		newConfigSchemaCmd(),
	)
	return cmd
}

// newConfigInitCmd 构造 config init 子命令。
func newConfigInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "生成配置模板（已存在则跳过）",
		Long: `在配置文件路径生成带注释的模板（version: 1，db/redis 空列表），权限 0600。

文件已存在时跳过，不改写现有配置。`,
		Example: `  omni-cli config init`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigInit(cmd)
		},
	}
}

// runConfigInit 生成配置模板：已存在跳过，否则创建目录并写入 0600 模板。
func runConfigInit(cmd *cobra.Command) error {
	path, err := resolveConfigPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		fmt.Fprintf(cmd.OutOrStdout(), "配置已存在，跳过：%s\n", path)
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("检查配置 %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("创建配置目录 %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(configTemplate), 0600); err != nil {
		return fmt.Errorf("写入配置 %s: %w", path, err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		return fmt.Errorf("设置配置 %s 权限: %w", path, err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "已生成配置：%s\n", path)
	return nil
}

// newConfigPathCmd 构造 config path 子命令。
func newConfigPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "path",
		Short:   "打印配置文件路径",
		Long:    `打印生效的配置文件路径：--config 优先，其次 OMNI_CLI_CONFIG，最后默认路径 ~/.omni-cli/config.yaml。`,
		Example: `  omni-cli config path`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigPath(cmd)
		},
	}
}

// runConfigPath 打印生效的配置路径。
func runConfigPath(cmd *cobra.Command) error {
	path, err := resolveConfigPath()
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), path)
	return nil
}

// newConfigAddCmd 构造 config add 子命令。
func newConfigAddCmd() *cobra.Command {
	var key, env, typ, driver string
	var params []string
	var readonly bool
	cmd := &cobra.Command{
		Use:   "add <db|redis>",
		Short: "新增一条连接配置",
		Long: `向配置中新增一条连接配置，写回文件。

位置参数为命令类型：db 或 redis。--key + --env 在同一类型内复合唯一，重复时报错。
db 需 --driver（mysql | postgres）；redis 不需要 driver。
--type 为连接方式（v1 仅 tcp，默认 tcp）；其余连接字段用可重复的 --param k=v 传入。
tcp 支持可选 ssh 隧道参数：ssh_host / ssh_port（默认 22）/ ssh_user / ssh_password。
--readonly 为只读保护（默认 true，写操作被拦截；设为 false 允许写）。`,
		Example: `  omni-cli config add db --key orders --env dev --driver mysql \
    --param host=127.0.0.1 --param port=3306 --param user=root \
    --param password='${DEV_DB_PASS}' --param database=orders
  omni-cli config add redis --key cache --env dev --param host=127.0.0.1 --param port=6379
  omni-cli config add db --key orders --env prod --driver mysql \
    --param host=10.0.0.7 --param port=3306 --param ssh_host=jump.example.com \
    --param ssh_user=jump --param ssh_password='${JUMP_PASS}'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigAdd(cmd, args[0], key, env, typ, driver, readonly, params)
		},
	}
	cmd.Flags().StringVarP(&key, "key", "k", "", "连接 key（必填）")
	cmd.Flags().StringVarP(&env, "env", "e", "", "环境名（必填）")
	cmd.Flags().StringVar(&typ, "type", string(conn.ConnTypeTCP), "连接方式（v1 仅 tcp）")
	cmd.Flags().StringVar(&driver, "driver", "", "数据库驱动：mysql | postgres（仅 db）")
	cmd.Flags().BoolVar(&readonly, "readonly", true, "只读保护（默认 true；false 允许写）")
	cmd.Flags().StringArrayVar(&params, "param", nil, "连接参数 k=v（可重复）")
	return cmd
}

// runConfigAdd 校验参数后向对应段追加一条 entry 并写回。
func runConfigAdd(cmd *cobra.Command, section, key, env, typ, driver string, readonly bool, params []string) error {
	if key == "" {
		return fmt.Errorf("缺少 --key，例如 omni-cli config add %s --key orders --env dev", section)
	}
	if env == "" {
		return fmt.Errorf("缺少 --env，例如 omni-cli config add %s --key orders --env dev", section)
	}
	connType := conn.ConnType(typ)
	if !conn.IsKnownType(connType) {
		return unknownTypeErr(connType)
	}
	paramMap, err := parseParams(params)
	if err != nil {
		return err
	}
	entry := config.Entry{Key: key, Env: env, Type: connType, Driver: driver, ReadOnly: &readonly, Params: paramMap}
	if err := validateDriverForSection(section, &entry); err != nil {
		return err
	}

	cfg, path, err := loadConfigRaw()
	if err != nil {
		return err
	}
	entries, err := sectionEntries(cfg, section)
	if err != nil {
		return err
	}
	for _, e := range *entries {
		if e.Key == key && e.Env == env {
			return fmt.Errorf("%s 已存在 key=%q env=%q 的条目。可用 config set 修改，或 config remove 删除", section, key, env)
		}
	}
	*entries = append(*entries, entry)
	if err := config.Save(path, cfg); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "已添加 %s 条目：%s/%s\n", section, key, env)
	return nil
}

// newConfigSetCmd 构造 config set 子命令。
func newConfigSetCmd() *cobra.Command {
	var key, env, typ, driver string
	var params []string
	var readonly bool
	cmd := &cobra.Command{
		Use:   "set <db|redis>",
		Short: "覆盖指定条目的字段",
		Long: `按 key+env 定位一条配置，覆盖 --type / --driver / --readonly / --param 指定的字段，写回文件。

未提供的字段保持不变；--param 只覆盖给定的 key，其余 params 保留。
--readonly 仅在显式传入时生效（--readonly=true|false）。`,
		Example: `  omni-cli config set db --key orders --env dev --driver postgres
  omni-cli config set db --key orders --env dev --readonly=false
  omni-cli config set db --key orders --env dev --param password='${NEW_DB_PASS}'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigSet(cmd, args[0], key, env, typ, driver, readonly, params)
		},
	}
	cmd.Flags().StringVarP(&key, "key", "k", "", "连接 key（必填）")
	cmd.Flags().StringVarP(&env, "env", "e", "", "环境名（必填）")
	cmd.Flags().StringVar(&typ, "type", "", "连接方式（v1 仅 tcp；省略则不变）")
	cmd.Flags().StringVar(&driver, "driver", "", "数据库驱动：mysql | postgres（仅 db；省略则不变）")
	cmd.Flags().BoolVar(&readonly, "readonly", true, "只读保护（显式传入 --readonly=true|false 才生效）")
	cmd.Flags().StringArrayVar(&params, "param", nil, "连接参数 k=v（可重复，只覆盖给定 key）")
	return cmd
}

// runConfigSet 定位条目并覆盖指定字段后写回。
func runConfigSet(cmd *cobra.Command, section, key, env, typ, driver string, readonly bool, params []string) error {
	if key == "" {
		return fmt.Errorf("缺少 --key，例如 omni-cli config set %s --key orders --env dev", section)
	}
	if env == "" {
		return fmt.Errorf("缺少 --env，例如 omni-cli config set %s --key orders --env dev", section)
	}
	cfg, path, err := loadConfigRaw()
	if err != nil {
		return err
	}
	entries, err := sectionEntries(cfg, section)
	if err != nil {
		return err
	}
	idx := indexOfEntry(*entries, key, env)
	if idx < 0 {
		return fmt.Errorf("未找到 %s 中 key=%q env=%q 的条目", section, key, env)
	}
	e := &(*entries)[idx]
	if typ != "" {
		ct := conn.ConnType(typ)
		if !conn.IsKnownType(ct) {
			return unknownTypeErr(ct)
		}
		e.Type = ct
	}
	if driver != "" {
		e.Driver = driver
	}
	if cmd.Flags().Changed("readonly") {
		e.ReadOnly = &readonly
	}
	if len(params) > 0 {
		pm, err := parseParams(params)
		if err != nil {
			return err
		}
		if e.Params == nil {
			e.Params = map[string]any{}
		}
		for k, v := range pm {
			e.Params[k] = v
		}
	}
	if err := validateDriverForSection(section, e); err != nil {
		return err
	}
	if err := config.Save(path, cfg); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "已更新 %s 条目：%s/%s\n", section, key, env)
	return nil
}

// newConfigRemoveCmd 构造 config remove 子命令。
func newConfigRemoveCmd() *cobra.Command {
	var key, env string
	cmd := &cobra.Command{
		Use:     "remove <db|redis>",
		Short:   "删除一条连接配置",
		Long:    `按 key+env 定位一条配置并删除，写回文件。`,
		Example: `  omni-cli config remove db --key orders --env dev`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigRemove(cmd, args[0], key, env)
		},
	}
	cmd.Flags().StringVarP(&key, "key", "k", "", "连接 key（必填）")
	cmd.Flags().StringVarP(&env, "env", "e", "", "环境名（必填）")
	return cmd
}

// runConfigRemove 定位条目并删除后写回。
func runConfigRemove(cmd *cobra.Command, section, key, env string) error {
	if key == "" {
		return fmt.Errorf("缺少 --key，例如 omni-cli config remove %s --key orders --env dev", section)
	}
	if env == "" {
		return fmt.Errorf("缺少 --env，例如 omni-cli config remove %s --key orders --env dev", section)
	}
	cfg, path, err := loadConfigRaw()
	if err != nil {
		return err
	}
	entries, err := sectionEntries(cfg, section)
	if err != nil {
		return err
	}
	idx := indexOfEntry(*entries, key, env)
	if idx < 0 {
		return fmt.Errorf("未找到 %s 中 key=%q env=%q 的条目", section, key, env)
	}
	*entries = append((*entries)[:idx], (*entries)[idx+1:]...)
	if err := config.Save(path, cfg); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "已删除 %s 条目：%s/%s\n", section, key, env)
	return nil
}

// newConfigGetCmd 构造 config get 子命令。
func newConfigGetCmd() *cobra.Command {
	var key, env string
	cmd := &cobra.Command{
		Use:   "get <db|redis>",
		Short: "打印单条配置（敏感字段打码）",
		Long: `按 key+env 打印一条连接配置（JSON）。

敏感参数（key 含 pass / token / secret，大小写不敏感）值替换为 ***。`,
		Example: `  omni-cli config get db --key orders --env dev`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigGet(cmd, args[0], key, env)
		},
	}
	cmd.Flags().StringVarP(&key, "key", "k", "", "连接 key（必填）")
	cmd.Flags().StringVarP(&env, "env", "e", "", "环境名（必填）")
	return cmd
}

// runConfigGet 定位条目并以打码 JSON 输出。
func runConfigGet(cmd *cobra.Command, section, key, env string) error {
	if key == "" {
		return fmt.Errorf("缺少 --key，例如 omni-cli config get %s --key orders --env dev", section)
	}
	if env == "" {
		return fmt.Errorf("缺少 --env，例如 omni-cli config get %s --key orders --env dev", section)
	}
	cfg, _, err := loadConfig()
	if err != nil {
		return err
	}
	entries, err := sectionEntries(cfg, section)
	if err != nil {
		return err
	}
	idx := indexOfEntry(*entries, key, env)
	if idx < 0 {
		return fmt.Errorf("未找到 %s 中 key=%q env=%q 的条目", section, key, env)
	}
	fmt.Fprintln(cmd.OutOrStdout(), output.JSON(entryJSON(section, &(*entries)[idx])))
	return nil
}

// newConfigListCmd 构造 config list 子命令。
func newConfigListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "列出全部连接配置",
		Long:    `列出配置中全部条目（db 与 redis），以对齐表格输出 TYPE/KEY/ENV/DRIVER/READONLY。`,
		Example: `  omni-cli config list`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigList(cmd)
		},
	}
}

// runConfigList 加载配置并列出全部条目。
func runConfigList(cmd *cobra.Command) error {
	cfg, _, err := loadConfig()
	if err != nil {
		return err
	}
	rows := [][]any{}
	for _, e := range cfg.DB {
		rows = append(rows, []any{sectionDB, e.Key, e.Env, e.Driver, e.IsReadOnly()})
	}
	for _, e := range cfg.Redis {
		rows = append(rows, []any{sectionRedis, e.Key, e.Env, "", e.IsReadOnly()})
	}
	if len(rows) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "配置为空。请先添加：omni-cli config add db --key ... --env ...")
		return nil
	}
	fmt.Fprintln(cmd.OutOrStdout(), output.Table(&db.Result{Columns: []string{"TYPE", "KEY", "ENV", "DRIVER", "READONLY"}, Rows: rows}))
	return nil
}

// newConfigShowCmd 构造 config show 子命令。
func newConfigShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "show",
		Short:   "打印完整配置（敏感字段打码）",
		Long:    `打印完整配置（JSON），敏感参数（key 含 pass / token / secret）值替换为 ***。`,
		Example: `  omni-cli config show`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigShow(cmd)
		},
	}
}

// runConfigShow 打印完整配置（打码 JSON）。
func runConfigShow(cmd *cobra.Command) error {
	cfg, _, err := loadConfig()
	if err != nil {
		return err
	}
	show := map[string]any{
		"version": cfg.Version,
		"db":      maskEntries(sectionDB, cfg.DB),
		"redis":   maskEntries(sectionRedis, cfg.Redis),
	}
	fmt.Fprintln(cmd.OutOrStdout(), output.JSON(show))
	return nil
}

// newConfigSchemaCmd 构造 config schema 子命令。
func newConfigSchemaCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "schema <db|redis>",
		Short: "打印命令类型的字段清单（JSON）",
		Long: `从代码生成某命令类型的字段清单（固定项 + 各连接方式的 params 字段），输出缩进 JSON。

db 含 driver 固定项（hasDriver=true），redis 不含（hasDriver=false）。`,
		Example: `  omni-cli config schema db
  omni-cli config schema redis`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigSchema(cmd, args[0])
		},
	}
}

// runConfigSchema 按命令类型生成并输出字段清单 JSON。
func runConfigSchema(cmd *cobra.Command, section string) error {
	var hasDriver bool
	switch section {
	case sectionDB:
		hasDriver = true
	case sectionRedis:
		hasDriver = false
	default:
		return fmt.Errorf("未知命令类型 %q。支持：%s, %s", section, sectionDB, sectionRedis)
	}
	s, err := schema.Build(hasDriver)
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), output.JSON(s))
	return nil
}

// loadConfig 加载配置文件（做 ${VAR} 插值）；文件不存在时返回空的 version:1 配置，便于首次 add 前免 init。
func loadConfig() (*config.Config, string, error) {
	return loadConfigWith(config.Load)
}

// loadConfigRaw 加载配置文件（不做 ${VAR} 插值，保留字面量）；文件不存在时返回空的 version:1 配置。
//
// add/set/remove 用它，避免把插值后的明文 secret 写回文件。
func loadConfigRaw() (*config.Config, string, error) {
	return loadConfigWith(config.LoadRaw)
}

// loadConfigWith 用 load 加载配置；文件不存在时返回空的 version:1 配置。
func loadConfigWith(load func(string) (*config.Config, error)) (*config.Config, string, error) {
	path, err := resolveConfigPath()
	if err != nil {
		return nil, "", err
	}
	cfg, err := load(path)
	if err == nil {
		return cfg, path, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return &config.Config{Version: 1}, path, nil
	}
	return nil, "", err
}

// sectionEntries 返回 section 对应的条目列表指针；section 非法时报错。
func sectionEntries(cfg *config.Config, section string) (*[]config.Entry, error) {
	switch section {
	case sectionDB:
		return &cfg.DB, nil
	case sectionRedis:
		return &cfg.Redis, nil
	default:
		return nil, fmt.Errorf("未知命令类型 %q。支持：%s, %s", section, sectionDB, sectionRedis)
	}
}

// indexOfEntry 返回 key+env 匹配条目的下标，未找到返回 -1。
func indexOfEntry(entries []config.Entry, key, env string) int {
	for i := range entries {
		if entries[i].Key == key && entries[i].Env == env {
			return i
		}
	}
	return -1
}

// validateDriverForSection 校验 section 的 driver 约束：db 必填且已知，redis 禁止。
func validateDriverForSection(section string, e *config.Entry) error {
	switch section {
	case sectionDB:
		if !db.IsKnownDriver(db.Driver(e.Driver)) {
			return fmt.Errorf("未知 driver %q。支持：%s, %s", e.Driver, db.DriverMySQL, db.DriverPostgres)
		}
	case sectionRedis:
		if e.Driver != "" {
			return fmt.Errorf("redis 不支持 --driver")
		}
	default:
		return fmt.Errorf("未知命令类型 %q。支持：%s, %s", section, sectionDB, sectionRedis)
	}
	return nil
}

// parseParams 把 k=v 列表解析为参数 map；缺 = 或 key 为空时报错。
func parseParams(params []string) (map[string]any, error) {
	out := map[string]any{}
	for _, p := range params {
		k, v, ok := strings.Cut(p, "=")
		if !ok {
			return nil, fmt.Errorf("--param 需为 k=v 形式，实际 %q", p)
		}
		if k == "" {
			return nil, fmt.Errorf("--param 需为 k=v 形式（key 不能为空）")
		}
		out[k] = v
	}
	return out, nil
}

// entryJSON 把 entry 渲染为小写 key 的 map（params 打码）；db 含 driver，均含 readonly。
func entryJSON(section string, e *config.Entry) map[string]any {
	m := map[string]any{
		"key":      e.Key,
		"env":      e.Env,
		"type":     string(e.Type),
		"readonly": e.IsReadOnly(),
		"params":   maskParams(e.Params),
	}
	if section == sectionDB {
		m["driver"] = e.Driver
	}
	return m
}

// maskEntries 把条目列表渲染为打码 map 列表。
func maskEntries(section string, entries []config.Entry) []map[string]any {
	out := make([]map[string]any, 0, len(entries))
	for i := range entries {
		out = append(out, entryJSON(section, &entries[i]))
	}
	return out
}

// maskParams 复制 params，敏感 key（含 pass/token/secret，大小写不敏感）值替换为 ***。
func maskParams(params map[string]any) map[string]any {
	out := make(map[string]any, len(params))
	for k, v := range params {
		if isSecretKey(k) {
			out[k] = maskValue
			continue
		}
		out[k] = v
	}
	return out
}

// isSecretKey 报告 key 是否敏感（含 pass/token/secret，大小写不敏感）。
func isSecretKey(k string) bool {
	lk := strings.ToLower(k)
	return strings.Contains(lk, "pass") || strings.Contains(lk, "token") || strings.Contains(lk, "secret")
}

// unknownTypeErr 构造面向 AI 的未知连接类型错误，列出所有支持的类型。
func unknownTypeErr(typ conn.ConnType) error {
	names := make([]string, 0, len(conn.SupportedTypes()))
	for _, t := range conn.SupportedTypes() {
		names = append(names, string(t))
	}
	return fmt.Errorf("未知连接类型 %q。支持：%s", typ, strings.Join(names, ", "))
}
