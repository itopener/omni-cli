# CLAUDE.md

`omni-cli`：面向 AI 的研发场景 CLI 统一入口（Go）。用 Go 原生驱动直连 MySQL/PostgreSQL/Redis，执行 SQL / redis 命令，统一输出。不依赖、不下载系统已装的客户端命令行。

## 核心文档

- `docs/design.md` —— 技术方案：配置结构（固定项 + params）、命令、`type` 扩展机制、发布。
- `docs/development-standards.md` —— 开发与代码规范。

## 强制要求

所有改动（代码、命令、配置、文档、测试）**必须遵守 `docs/development-standards.md`**。其中不可妥协的红线：

- **无魔法值**：字段名只出现在参数结构体的 yaml tag 与 cobra flag 定义两处；`type` / `driver` 用 typed 常量，禁止散落字符串字面量。
- **安全**：secret（密码/令牌）只在内存（驱动的 DSN / Options），不打印、不落日志/错误/测试；打印配置时敏感字段打码。
- **连接生命周期**：Open → 执行 → `defer Close`；单次命令池大小 1（db `SetMaxOpenConns(1)`、redis `PoolSize=1`）；查询用 `context.WithTimeout`（`--timeout` 默认 30s）。
- **AI 友好**：`--help`、错误信息、`config schema` 是产品接口，改动必须同步测试；`config schema` 从代码生成，不手写。
- **跨平台**：路径用 `filepath`（不手拼分隔符）；无子进程、无 shell 依赖。
- **错误处理**：`fmt.Errorf("...: %w", err)` 包裹，`errors.Is` / `errors.As` 判断；库代码不 panic，只在 `main` 层 `os.Exit`。
- **测试**：表驱动；数据库/redis 的端到端走 docker 集成测试，单元测试只测纯逻辑（DSN / Options / 解析），不真连库；每个 bug 修复带回归测试；合并前 `go test ./...` 全绿。

## 当前阶段

v1 只做 `db` + `redis` 两个命令类型，连接方式为直连 tcp + 可选 ssh 隧道（`type: tcp`；params 的 `ssh_host` / `ssh_port` / `ssh_user` / `ssh_password` 非空 `ssh_host` 即启用，仅密码认证、不校验跳板机主机密钥）。

- 配置层级：命令类型 → 条目列表；每条 entry 由 `key` + `env` 复合唯一定位。
- 固定项只有 `key` / `env` / `type`（db 再加 `driver`），其余全部进 `params`，字段集由 `type` 决定。
- 加新命令类型（如 `jira`）或新连接方式（如 `platform`）前，先看 `docs/design.md` 的扩展机制，按 strategy 模式加实现，不改已有 type。
