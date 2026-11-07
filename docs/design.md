# omni-cli 技术方案

## 意图

`omni-cli` 是一个面向 AI 的研发场景 CLI。**用 Go 原生驱动直接连 MySQL / PostgreSQL / Redis**，执行 SQL 或 redis 命令，统一输出。

**不依赖、不调用系统已装的客户端命令行**（`mysql` / `psql` / `redis-cli` 都不是前提）。连接信息由全局配置文件管理，配置与使用全部通过 CLI 完成，AI 靠 `--help` 和 `config schema` 即可上手。

v1 只做 `db`、`redis` 两个命令类型，且只支持直连（`type: tcp`）；tcp 可选 ssh 隧道（经跳板机转发）。

## 目标 / 非目标 / 反目标

### 目标（v1）

- `db`：按 `driver`（mysql / postgres）用原生驱动连库、执行 SQL、输出结果。
- `redis`：用原生驱动执行 redis 命令、输出结果。
- 全局配置文件管理连接信息，结构可扩展。
- 直连（`type: tcp`）场景完整可用；tcp 支持可选 ssh 隧道（密码认证跳板机）。
- **CLI 自文档**：`--help` 说明用途/参数/示例；`config schema` 机器可读。
- **配置全走 CLI**：`config add/set/remove/get/list/show/schema` 管理。
- **连接不泄漏、不浪费**：短连接 + 显式 Close + 池限制 + 超时。
- 跨平台：Linux（glibc/musl）、macOS、Windows 均可跑。

### 非目标（v1，明确不做）

- 不依赖、不下载系统已装的客户端命令行。
- 不做 `platform` / `dsn` / `iam` 等连接方式，只留 `type` 扩展口（ssh 隧道已作为 tcp 的可选参数实现，不是独立 type）。
- 不做 ssh 私钥 / known_hosts 主机密钥校验（v1 只支持密码认证，主机密钥不校验，见「ssh 隧道」节）。
- 不做 `api` / `jira` 等其它命令类型。
- 不做常驻 daemon / 连接复用会话（每次命令独立短连接）。
- 不做密码加密存储（只用 `${VAR}` 环境变量插值 + 文件 `0600`）。

### 反目标（做出来就是失败）

- secret（密码/令牌）落日志、错误信息、测试夹具。
- 连接泄漏（Open 不 Close）。
- 查询卡死无超时，占住连接。
- 硬编码字段导致以后加连接方式要改 schema。
- 字段名/类型名以字符串字面量散落代码（魔法值）。

## 约束

- 语言：Go。`CGO_ENABLED=0` 出单静态二进制，交叉编译零成本。
- 依赖（全部纯 Go，无 cgo）：`cobra`、`gopkg.in/yaml.v3`、`github.com/go-sql-driver/mysql`、`github.com/jackc/pgx/v5`、`github.com/redis/go-redis/v9`、`golang.org/x/crypto`（ssh 隧道）。

## 连接管理（核心）

每次 CLI 调用是**短命进程**，连接生命周期由本工具显式管理：

- 一次命令：Open 连接 → 执行 → `defer Close()`。进程退出也自动断开，双保险。
- **单连接、不浪费**：单次命令只需一条连接，db 设 `SetMaxOpenConns(1)`，redis 设 `PoolSize=1`，不为一次查询开一池连接。
- **超时**：`--timeout`（默认 30s）包成 `context.WithTimeout`，贯穿驱动；超时/取消即中断查询、回收连接。
- **secret 只在内存**：密码进驱动的 DSN / Options 结构（内存中），不打印、不落日志、不进 argv。
- 连接池调优（多连接、空闲回收）留到未来常驻/daemon 模式，v1 不需要。

## ssh 隧道（tcp 可选参数）

`tcp` 的 params 含 `ssh_host` 时启用（`internal/sshtunnel`）：

- **语义**：`host` / `port` 是跳板机视角的目标地址；先与 `ssh_host:ssh_port`（默认 22）建 ssh 连接（密码认证），在本地 `127.0.0.1` 随机端口起转发监听，db / redis 对本地地址建连，流量经隧道直达目标。
- **生命周期**：隧道与命令同生命周期，命令结束（含出错）即关闭监听与 ssh 连接；ssh 拨号与握手受 `--timeout` 的 ctx 约束。
- **secret**：`ssh_password` 只进 `ssh.ClientConfig`（内存）；打印配置时命中打码规则（key 含 pass）。
- **实现**：`golang.org/x/crypto/ssh` 纯 Go，无系统 ssh 客户端依赖；cmd 层 `openConn` 分发——无 `ssh_host` 直连原样返回，有则换本地转发地址，db / redis 包零改动。
- **局限**：主机密钥不校验（`InsecureIgnoreHostKey`，中间人风险自负）；仅密码认证，私钥 / known_hosts 留待扩展。

## 配置结构

默认路径 `~/.omni-cli/config.yaml`，可被 `--config` 或 `OMNI_CLI_CONFIG` 覆盖。文件是可读的 YAML 后台存储，**正常操作一律走 CLI**。

层级：**命令类型 → 条目列表**。每条一个连接，`key` + `env` 复合唯一定位。

```yaml
version: 1

db:
  - key: orders
    env: dev
    type: tcp                 # 连接方式，决定 params 字段集
    driver: mysql             # db 专属：mysql | postgres
    readonly: true            # 只读保护，默认 true；写操作需设为 false
    params:                   # type 决定的连接字段
      host: 127.0.0.1
      port: 3306
      user: root
      password: ${DEV_DB_PASS}
      database: orders

  - key: inner-orders         # 可选 ssh 隧道：经跳板机访问内网地址
    env: prod
    type: tcp
    driver: mysql
    params:
      host: 10.0.0.7          # 跳板机视角的目标地址
      port: 3306
      user: root
      password: ${PROD_DB_PASS}
      database: orders
      ssh_host: jump.example.com   # 非空即启用隧道
      ssh_port: 22                 # 可省略，默认 22
      ssh_user: jump
      ssh_password: ${JUMP_PASS}

  - key: cloud-orders         # 平台连接，字段和 tcp 完全不同
    env: prod
    type: platform
    driver: mysql
    params:
      endpoint: https://db.example.com
      token: ${CLOUD_TOKEN}
      database: orders

redis:
  - key: cache
    env: dev
    type: tcp
    params:
      host: 127.0.0.1
      port: 6379
      password: ""
      db: 0
```

### 固定项 vs params

每条 entry 分两层：

- **固定项**：跨类型统一的字段，typed 结构体承接。
  - 所有类型：`key` / `env` / `type` / `readonly`（bool，**默认 `true`**）
  - 命令类型专属：`db` 有 `driver`（mysql | postgres）；`redis` 无
- **params**：连接字段，**由 `type` 决定字段集**。
  - `tcp`：`host` / `port` / `user` / `password` / `database`（db）或 `db`（redis），加可选 ssh 隧道字段 `ssh_host` / `ssh_port` / `ssh_user` / `ssh_password`
  - `platform`（未来）：`endpoint` / `token` 等，**无** host/port/user/password
  - 每个 `type` 一个 typed 参数结构体，字段名只出现在 yaml tag

### 无魔法值

- 字段名只出现在两处：参数结构体的 yaml/json tag、cobra 的 flag 定义。
- `type` / `driver` 用 typed 常量（`conn.ConnType*`、`db.Driver*`）。
- 每个 `type` 的参数结构体显式声明自己认识的字段；`params` 里未声明的 key 宽松保留在 `Extra`，加字段不改代码。
- 子命令名（`db` / `redis`）由 cobra 注册集中定义。

### 配置规则

- 一级 key = 命令类型，与子命令同名。
- 每条 entry 由 `key` + `env` 定位，二者**复合唯一**，加载时校验。
- `type` 是连接方式判别符，v1 只认 `tcp`（可省略，默认 `tcp`），其它报错并列出支持类型。
- `${VAR}` 在加载时插值；变量未设置报错，不静默换空串。
- `config init` 生成模板并 `chmod 0600`；所有打印配置的命令把 `password` / `token` 打码。

## 只读保护

每条 entry 有 `readonly` 固定项（bool，**默认 `true`**）。只读时拦截写操作：

- **db**：只允许读语句（首关键字白名单 `SELECT` / `SHOW` / `DESCRIBE` / `DESC` / `EXPLAIN`）；其它（INSERT/UPDATE/DELETE/DDL 等）报错「只读连接禁止写操作，需设 `readonly: false`」。
- **redis**：只允许读命令白名单（`GET`/`MGET`/`HGET*`/`LRANGE`/`SMEMBERS`/`ZRANGE*`/`EXISTS`/`TTL`/`SCAN`/`KEYS` 等）；白名单外的命令（`SET`/`DEL`/`EVAL`/`FLUSHALL` 等）拦截。
- **默认拒绝**：未列出的语句/命令一律按写处理，宁可误拦不可漏放。
- **局限**：白名单按首关键字判断，`SELECT ... INTO OUTFILE`、`SELECT nextval(...)`、volatile 函数等服务端写会判为读，靠 DB 账号权限兜底。
- 多语句 SQL 由驱动默认禁用（go-sql-driver `multiStatements=false`、pgx 单语句），不会绕过首关键字检查。

要写数据：`config set db --key orders --env dev --readonly=false`（或 `config add ... --readonly=false`）。

## 命令

```bash
# db：执行 SQL
omni-cli db -e prod -k orders "SELECT count(*) FROM users"
omni-cli db -e prod -k orders --sql "SELECT 1"
omni-cli db -e prod -k orders --format json "SELECT 1"
omni-cli db list [-e prod] [-k orders]

# redis：命令及参数原样传入
omni-cli redis -e prod -k cache GET user:1
omni-cli redis -e prod -k cache SET k v
omni-cli redis list

# 配置管理（全走 CLI）
omni-cli config init
omni-cli config add db --key orders --env dev --type tcp --driver mysql \
  --param host=127.0.0.1 --param port=3306 --param user=root \
  --param password='${DEV_DB_PASS}' --param database=orders
omni-cli config set db --key orders --env dev --param host=10.0.0.2
omni-cli config remove db --key orders --env dev
omni-cli config get db --key orders --env dev
omni-cli config list [<type>]
omni-cli config show
omni-cli config schema [<type>]
omni-cli config path
```

### flag 约定

- `-e, --env`：环境。省略时：key 只有一个 env → 直接用；多个 → 报错并列出。
- `-k, --key`：连接 key。省略时：该类型只有一个 key → 直接用；多个 → 报错并列出。
- `--config`：配置路径覆盖（等价 `OMNI_CLI_CONFIG`）。
- `-f, --format`：`table`（默认，对齐表格）| `json`。
- `--timeout`：命令超时，默认 `30s`。

### config 子命令说明

- 固定项走显式 flag：`--key` / `--env` / `--type` / `--driver` / `--readonly`（bool，默认 `true`）。
- 连接字段走可重复的 `--param k=v`（不同 `type` 字段不同，flag 无法静态枚举）。
- `--param password='${VAR}'`：单引号防 shell 展开；存 `${VAR}`，加载时插值。
- `get` / `list` / `show`：只读，敏感项打码。
- `schema [<type>]`：输出固定字段 + 各 `type` 的 params 字段清单（JSON）。

## 底层驱动映射

| 类型 | driver | Go 驱动 | 连接构造 |
|---|---|---|---|
| db | mysql | `github.com/go-sql-driver/mysql` | `mysql.Config{...}.FormatDSN()` |
| db | postgres | `github.com/jackc/pgx/v5`（stdlib） | `postgres://user:pass@host:port/db` |
| redis | — | `github.com/redis/go-redis/v9` | `redis.Options{Addr, Password, DB}` |

关键点：

- **DSN 用驱动自带 Config 构造**（`mysql.Config.FormatDSN`、pgx 连接串），不手拼字符串，避免密码含 `@`/`:`/`/` 时转义 bug。
- 密码只进驱动连接配置（内存），不打印。

## 连接方式扩展（strategy）

`type` 是连接方式判别符，每个 `type` 一个参数结构体 + 一个 resolver：

```go
type TCPParams struct {
    Host     string `yaml:"host"`
    Port     int    `yaml:"port"`
    User     string `yaml:"user"`
    Password string `yaml:"password"`
    Database string `yaml:"database"`
    DB       int    `yaml:"db"`
    Extra    map[string]string
}

type Resolver interface {
    Type() ConnType
    Resolve(params map[string]any) (any, error)  // 返回 typed params
    Schema() []ParamField
}

// registry: map[ConnType]Resolver{ ConnTypeTCP: tcpResolver{} }
```

- `conn.Resolve` 产出 typed params（`*TCPParams`），`db` / `redis` 消费它建原生连接。
- v1 只注册 `tcp`。加 `platform` 时定义 `PlatformParams` + 对应 resolver，注册进 map，db/redis 按类型分发。

## 项目结构

```
omni-cli/
  cmd/
    omni-cli/
      root.go            # cobra root，全局 flag（--config）
      db.go              # db 子命令
      redis.go           # redis 子命令
      config.go          # config 管理子命令
      main.go
  internal/
    config/            # 配置加载、校验、${VAR} 插值、读改写、条目解析
    conn/              # ConnType + params 结构 + resolver 注册表（叶子包）
    db/                # DSN 构造 + database/sql 查询 + 结果集
    redis/             # go-redis 执行
    sshtunnel/         # tcp 可选 ssh 隧道（本地转发）
    output/            # table/json 渲染
    schema/            # 机器可读字段清单（JSON）
  go.mod
  go.sum
```

## 输出

- db：`database/sql` 返回结构化 rows（列名 + 值），`output` 渲染为对齐表格（默认）或 JSON。
- redis：`go-redis` 返回 Go 原生值，打印值（默认）或 JSON。
- 无「透传底层客户端文本」——结构化输出是原生驱动的直接收益。

## 跨平台要点

- 路径用 `filepath.Join`，不手拼分隔符。
- 无子进程、无 shell 依赖，天然规避 `LookPath` / argv 泄漏 / 终端差异问题。
- Windows 终端颜色/ANSI 降级纯文本。

## 发布（二期，方案已定）

- GoReleaser：交叉编译全平台二进制 + 自动发 GitHub Releases + 生成校验和。
- Homebrew Tap + 一键安装脚本（`curl -fsSL .../install.sh | sh`）。
- Windows 补 Scoop / winget。

## 留待实现者决定

- DSN 里超时/SSL 等参数的默认值（v1 关 SSL，后续按 `params` 扩展）。
- 非 SELECT 语句的结果展示（v1：无列时输出「Executed OK」；受影响的函数二期再细化）。
- `schema` JSON 具体形状、错误信息措辞、`db list` 输出格式。
