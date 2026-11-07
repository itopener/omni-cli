# omni-cli

> Unified CLI entry for AI-driven dev workflows — talk to MySQL / PostgreSQL / Redis with native Go drivers, no local client tools.

面向 AI 的研发场景 CLI 统一入口。Go 原生驱动直连 MySQL / PostgreSQL / Redis：执行 SQL 与 redis 命令，统一输出对齐表格或 JSON。不依赖、不调用系统已装的 `mysql` / `psql` / `redis-cli`，单二进制开箱即用。

## 特性

- 单二进制，无运行时依赖，不 fork 任何客户端进程
- MySQL / PostgreSQL 直连执行 SQL；Redis 命令原样透传
- 统一输出：对齐 `table` 与机器可读 `json`（`--format`）
- `key` + `env` 复合定位连接；全局配置文件集中管理
- 密码等敏感字段支持 `${ENV_VAR}` 环境变量插值，不落明文
- 默认只读保护（`readonly: true`），写操作需显式放开
- 可选 ssh 跳板隧道，经跳板机访问内网地址
- `--timeout` 控制单条命令时长（默认 30s）
- AI 友好：`config schema` 输出机器可读的字段清单，`--help` 措辞稳定

## 安装

### macOS / Linux（一键脚本）

```sh
curl -fsSL https://raw.githubusercontent.com/itopener/omni-cli/main/install.sh | sh
```

指定版本：

```sh
curl -fsSL https://raw.githubusercontent.com/itopener/omni-cli/main/install.sh | sh -s -- --version=1.0.0
```

脚本会下载对应平台的压缩包、校验 sha256 后安装到 `/usr/local/bin`（无写权限时回落到 `~/.local/bin` 并提示加入 PATH）。Windows 用户走下面的手动下载。

### 手动下载

[Releases](https://github.com/itopener/omni-cli/releases) 提供 darwin / linux / windows × amd64 / arm64 的压缩包。下载解压后把二进制放进 PATH，建议校验：

```sh
shasum -a 256 -c checksums.txt   # Linux 上用 sha256sum -c checksums.txt
```

> **macOS 提示**：浏览器下载的压缩包首次运行可能被 Gatekeeper 拦截（「无法验证开发者」），执行 `xattr -d com.apple.quarantine omni-cli` 或在访达中右键打开即可。一键脚本安装路径不受影响（curl 下载不带 quarantine 属性）。

### 源码构建

```sh
git clone https://github.com/itopener/omni-cli.git
cd omni-cli
go build -o omni-cli ./cmd
```

## 快速开始

```sh
# 1. 生成配置模板（~/.omni-cli/config.yaml）
omni-cli config init

# 2. 添加一条 MySQL 连接（密码用 ${VAR} 引用环境变量，单引号防 shell 展开）
omni-cli config add db --key orders --env dev --driver mysql \
  --param host=127.0.0.1 --param port=3306 --param user=root \
  --param 'password=${ORDERS_DB_PASS}' --param database=orders

# 3. 执行 SQL
omni-cli db -k orders -e dev --sql "SELECT count(*) FROM users"
omni-cli db -k orders -e dev --format json --sql "SHOW TABLES"

# 4. Redis 同理
omni-cli config add redis --key cache --env dev \
  --param host=127.0.0.1 --param port=6379 --param db=0
omni-cli redis -k cache -e dev GET user:1
```

`-k`（key）与 `-e`（env）可以只给一个：另一个唯一时自动解析。

## 配置

配置默认在 `~/.omni-cli/config.yaml`，可用 `--config <path>` 或环境变量 `OMNI_CLI_CONFIG` 覆盖。条目由 `key` + `env` 复合唯一定位，固定项只有 `key` / `env` / `type`（db 再加 `driver`），其余连接字段全部在 `params` 里，字段集由 `type` 决定：

```yaml
version: 1

db:
  - key: orders
    env: dev
    type: tcp                 # v1 连接方式仅 tcp
    driver: mysql             # mysql | postgres
    readonly: true            # 只读保护，默认 true；写操作需显式设为 false
    params:
      host: 127.0.0.1
      port: 3306
      user: root
      password: ${ORDERS_DB_PASS}   # 加载时从环境变量插值，配置文件不落明文
      database: orders

  - key: inner-orders         # 可选 ssh 隧道：经跳板机访问内网地址
    env: prod
    type: tcp
    driver: postgres
    params:
      host: 10.0.0.7          # 跳板机视角的目标地址
      port: 5432
      user: app
      password: ${PROD_DB_PASS}
      database: orders
      ssh_host: jump.example.com    # 非空即启用隧道
      ssh_port: 22                  # 可省略，默认 22
      ssh_user: jump
      ssh_password: ${JUMP_PASS}

redis:
  - key: cache
    env: dev
    type: tcp
    params:
      host: 127.0.0.1
      port: 6379
      db: 0
```

各连接方式支持哪些字段，机器可读清单：

```sh
omni-cli config schema db
omni-cli config schema redis
```

打印配置时敏感字段一律打码（`config get` / `config show`）。

### 只读保护

`readonly: true`（默认）会拦截非只读 SQL（`INSERT` / `UPDATE` / `DELETE` / `DDL` 等），防止误写。需要写入时显式放开：

```sh
omni-cli config set db --key orders --env dev --readonly=false
```

已知限制：白名单式拦截无法覆盖 `SELECT ... INTO OUTFILE`、含副作用的函数调用等语句；生产环境慎开写权限。

### ssh 隧道（实验性）

`params` 里出现 `ssh_host` 即启用：CLI 先与跳板机建 ssh 连接，再把目标 `host:port` 转发到本地临时端口。当前限制：仅密码认证，不校验跳板机主机密钥（后续版本补密钥认证与 known_hosts）。

## 命令一览

| 命令 | 说明 |
| --- | --- |
| `omni-cli db -k <key> -e <env> [--sql "\<SQL\>"]` | 执行 SQL（SQL 也可用位置参数） |
| `omni-cli db list` | 列出 db 连接条目 |
| `omni-cli redis -k <key> -e <env> \<CMD\> [args...]` | 执行 redis 命令 |
| `omni-cli redis list` | 列出 redis 连接条目 |
| `omni-cli config init` | 生成配置模板（已存在则跳过） |
| `omni-cli config add <db\|redis> --key K --env E [--param k=v]...` | 新增连接条目 |
| `omni-cli config set <db\|redis> --key K --env E [字段 flags]` | 覆盖条目字段 |
| `omni-cli config remove <db\|redis> --key K --env E` | 删除条目 |
| `omni-cli config get <db\|redis> --key K --env E` | 打印单条配置（打码） |
| `omni-cli config list` / `config show` | 列出条目 / 打印完整配置（打码） |
| `omni-cli config path` | 打印配置文件路径 |
| `omni-cli config schema <db\|redis>` | 机器可读字段清单（JSON） |
| `omni-cli version` | 打印版本（等价 `--version`） |

通用 flag：`--config <path>`（配置文件路径）、`--format table\|json`、`--timeout 30s`。

## 开发

```sh
go build ./... && go test ./...
```

设计文档见 [docs/design.md](docs/design.md)，开发规范见 [docs/development-standards.md](docs/development-standards.md)。

## License

[MIT](LICENSE) © itopener
