# omni-cli 开发与代码规范

面向 Go + 本项目特性的统一规范，保证后续开发质量与一致性。所有 PR 必须符合本规范。

## 1. 技术栈与工具链

- Go 1.22+（模块模式，`go.mod` 管理依赖）。
- 依赖（全部纯 Go、无 cgo）：`cobra`、`gopkg.in/yaml.v3`、`github.com/go-sql-driver/mysql`、`github.com/jackc/pgx/v5`、`github.com/redis/go-redis/v9`。新增依赖需说明理由。
- 强制工具：`gofmt` / `goimports`、`go vet`、`golangci-lint`、`staticcheck`。
- CI（GitHub Actions）门禁：`go vet` + `golangci-lint` + `go test ./...` + `go build`。

### golangci-lint 基线（实施时落 `.golangci.yml`）

```yaml
linters:
  enable:
    - govet
    - staticcheck
    - errcheck
    - ineffassign
    - unused
    - gocritic
    - misspell
    - gofmt
    - goimports
```

## 2. 开发流程规范

- 分支：每个改动一个 feature 分支，PR 评审后 squash merge 到 `main`。
- 提交信息：Conventional Commits —— `feat:` `fix:` `refactor:` `docs:` `test:` `chore:` `ci:`。一个提交只做一件事。
- 测试门禁：`go test ./...` 必须全绿才可合并。
- 变更范围克制：一次改动只解决一个问题，不夹带无关重构。

## 3. 目录结构（Go 标准布局）

- `cmd/omni-cli/`：只放 `main` 入口和 cobra 接线，**不含业务逻辑**。
- `internal/`：业务逻辑，不对外暴露 API。
- 包职责单一，禁止循环依赖。
- 新增命令类型（如未来 `jira`）时，`cmd/omni-cli/` 加子命令 + `internal/` 加对应包。

## 4. 代码规范

### 命名

- 惯用 Go 风格：短 receiver（`c`、`e`）、缩写全大写（`URL`、`ID`、`SSH`）、包名短小写单数。
- 所有导出符号必须写 doc comment，格式 `// Name 说明`。
- 变量名能表意，避免 `data` / `tmp` / `x1` 这类空泛命名。

### 错误处理

- 一律用 `fmt.Errorf("上下文: %w", err)` 包裹，保留错误链。
- 判断用 `errors.Is` / `errors.As`；可预期错误用 sentinel error（集中定义在各自包）。
- 库代码**不 panic**；只在 `main` 层 `os.Exit`。
- 面向 AI 的错误信息：说清「发生了什么 + 怎么修 + 合法选项是什么」。例：
  ```
  未知连接类型 "platform"。支持：tcp
  ```
  而不是 `invalid type`。

### 配置与常量（无魔法值）

- 字段名只出现在两处：参数结构体的 yaml tag、cobra 的 flag 定义。禁止其它地方手写 `"host"` / `"port"` 字符串。
- `type` / `driver` 用 typed 常量（`ConnTypeTCP`、`DriverMySQL`…），禁止散落 `"tcp"` / `"mysql"` 字面量。
- 每个 `type` 的参数结构体集中声明自己认识的字段；未声明字段的透传规则也集中在一处。

### 接口

- 接口小而聚焦，**消费者侧定义**。`conn.Resolver` 是唯一策略缝，新连接方式只加 resolver 实现，不加判断分支。

### 安全

- secret（密码/令牌）只在内存（驱动的 DSN / Options），**不打印、不落日志/错误/测试**。
- 任何打印配置/日志的地方，敏感字段（`password` / `token`）必须打码。
- 不把明文 secret 写进日志、错误信息、测试夹具。
- 配置文件写入后 `chmod 0600`。

### 连接生命周期

- 每次命令：Open 连接 → 执行 → `defer Close`；进程退出也自动断开。
- 单次命令池大小 1：db `SetMaxOpenConns(1)`、redis `PoolSize=1`，不为一次查询开一池连接。
- 查询统一走 `context.WithTimeout`（`--timeout` 默认 30s），超时/取消即中断查询、回收连接。
- v1 无业务并发；若引入，goroutine 之间用 channel 通信，避免共享可变状态。

### 测试

- 表驱动测试（table-driven）为默认形式。
- 数据库/redis 端到端走 docker 集成测试；单元测试只测纯逻辑（DSN / Options / 解析），**不真连库**。
- 帮助文本、错误信息、`config schema` 输出是产品 API，必须测试（断言或 golden file）。
- 每个 bug 修复必须带回归测试。

## 5. 跨平台红线

- 路径用 `filepath.Join` / `filepath.Clean`，**不手拼分隔符**。
- 无子进程、无 shell 依赖（原生驱动直连，天然规避 `LookPath` / argv 泄漏问题）。
- 不依赖 shell（不写 `sh -c` / `bash`），不假设文件权限位语义。
- Windows 终端颜色/ANSI 降级纯文本。

## 6. AI 友好红线（本项目核心）

- `--help`、错误信息、`config schema` 是**产品接口**，改动必须同步测试。
- `config schema` 必须从代码生成（不是手写 JSON），保证与代码一致。
- 新字段/新 `type` 必须在 `schema` 与 `--help` 示例里同步体现。
- 命令行为保持可预测：同样的输入永远同样的输出（除依赖外部 DB/Redis 服务的部分）。

## 7. 合并前 checklist

- [ ] `gofmt` / `goimports` 干净
- [ ] `go vet` 干净
- [ ] `golangci-lint` 干净
- [ ] `go test ./...` 全绿
- [ ] 无明文 secret 落日志/错误/测试
- [ ] 新命令/新字段的 `--help` 与 `schema` 已同步
- [ ] 提交信息符合 Conventional Commits
