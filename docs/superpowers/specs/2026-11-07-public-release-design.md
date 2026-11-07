# 公开发布 omni-cli v1.0.0 — 设计

日期：2026-11-07
状态：待批准（已过 builder check，事实性错误已修正）

## 意图与背景

omni-cli 要发给公网上的任意开发者使用，不只是团队内部。

当前状态与目标的差距：

- remote `origin https://github.com/itopener/omni-cli.git` 已配置，但一次都没推过；远端是空仓库（`git ls-remote` 返回 0 refs）。
- `go.mod` 的 module path 是 `omni-cli`，不是可解析的导入路径。
- 没有 LICENSE、README、CI、发布流水线、安装脚本。
- 版本注入机制已就绪：`cmd/root.go` 里 `var version = "0.0.1"`，构建时用 `-ldflags "-X main.version=v1.0.0"` 覆盖，`version` 子命令与 `--version` flag 都读它，且有测试覆盖。
- `golang.org/x/crypto v0.17.0`（2024-01）落后于 CVE-2024-45337（ssh 公钥认证绕过，v0.31.0 修复）与 CVE-2025-22869（ssh DoS，v0.35.0 修复）。v1 二进制带 ssh 隧道功能，这两条都打在自家功能面上。

`docs/design.md` 的「发布（二期，方案已定）」章节已经定了大方向：GoReleaser 交叉编译 + GitHub Releases + Homebrew Tap + install.sh + Windows 补 Scoop/winget。本设计在这个方向上收窄到本轮实际要做的范围。

## 目标

- 陌生开发者能在 60 秒内装上并跑通 `omni-cli --help`。
- 两条安装渠道：GitHub Releases 直接下载二进制；`curl | sh` 一键脚本。
- 平台覆盖 darwin / linux / windows × amd64 / arm64。
- 产物可校验（sha256），且 install.sh 强制校验，不给跳过的余地。
- 打 git tag 即发布，无人工交叉编译和手动上传步骤。

## 非目标

- Homebrew tap。本轮明确不做（需要另建 `itopener/homebrew-tap` 公开仓库）。
- Scoop / winget。
- Docker 镜像。
- 把 `go install` 当作文档推荐的安装方式。module path 改对之后它顺带能用，但 README 不主推。
- macOS 代码签名与公证。见「已知取舍」。

## 反目标（技术上跑通也算失败）

- install.sh 在校验 sha256 失败或校验工具缺失时仍然把二进制落盘。
- README、示例配置、测试 fixture 里出现任何真实密码、真实 host、真实 token。
- 发布流程依赖在本地手动 cross-compile 再手动上传到 Releases。
- 公开发布的二进制带着已知的 ssh CVE。
- 为了发布去改动 `internal/` 的运行时行为。发布这轮只动构建、打包、依赖版本、文档和仓库元数据。

## 约束

来自 `CLAUDE.md` 与 `docs/development-standards.md`，全部适用：

- 无魔法值；错误用 `fmt.Errorf("...: %w", err)` 包裹；测试表驱动；路径用 `filepath`；库代码不 panic。
- 「无子进程、无 shell 依赖」这条只约束 CLI 的 Go 运行时代码。`install.sh` 是给用户的安装入口，不是 CLI 的运行时行为，不受这条约束；CLI 自身仍然不 fork 任何进程。
- 本地 Go 环境有坑：必须带 `GOTOOLCHAIN=local`，裸跑 `go mod tidy` 会挂死。本地 go 版本 1.23.4，`go.mod` 声明 `go 1.22`。升级 `x/crypto` 时版本上限受本地工具链约束：不高于要求 go 1.23 的版本。
- 合并前 `go test ./...` 必须全绿。
- `--help`、错误信息、`config schema` 是产品接口，改动必须同步测试。本轮不改这些，但如果目录移动导致测试里的路径断言失效，要一并修。

## 已定的做法

### 1. module path 改名

`omni-cli` → `github.com/itopener/omni-cli`。

- `go.mod` 的 module 行改。
- 35 处 `"omni-cli/internal/..."` 形式的 import 全部改写（跨 `cmd/` 与各 `internal/` 包的测试文件）。
- 改完 `go build ./... && go test ./...` 必须通过。

### 2. main 包目录移动

`cmd/` → `cmd/omni-cli/`，整目录 `git mv`，`package main` 声明不变，import 不变。

理由：GoReleaser 的 `binary` 字段能控制发布产物名，但控制不了 `go install <module-path>@latest` 生成的二进制名——那个名字由目录名决定。main 包留在 `cmd/`，别人 `go install github.com/itopener/omni-cli/cmd@latest` 会得到一个叫 `cmd` 的可执行文件。

不采用的替代方案「根目录放 main.go + cmd 改成 library package」：`Execute()` 已经导出，`cmd/main.go` 只是 10 行薄入口，替代方案只需改各文件 package 子句加根 main.go，成本不大，但仍然比整目录 `git mv` 大，且收益相同。git mv 是零代码改动。

连带更新：

- `docs/development-standards.md:37` 与 `:40` 里对 `cmd/` 的描述。
- `docs/design.md` 的「项目结构」代码块。

### 3. 版本注入与版本字符串形态

沿用现有机制，不改代码。GoReleaser 的 ldflags 用：

```
-s -w -X main.version={{.Version}}
```

目录改名不改包名，符号仍然是 `main.version`。

版本字符串形态取 `{{.Version}}`（tag 去掉 `v` 前缀）：二进制打印 `omni-cli version 1.0.0`，与产物文件名里的版本段一致。`{{.Tag}}` 会打印 `v1.0.0`，和文件名不一致，且 `VERSION=v1.2.3` 环境变量还要多一层规范化。install.sh 接受 `VERSION` 带或不带 `v` 前缀，内部统一剥掉。README 的输出示例写剥掉后的形态。

### 4. `.goreleaser.yaml`

GoReleaser v2 配置格式（文件首行 `version: 2`）。要点：

- `main: ./cmd/omni-cli`，`binary: omni-cli`
- `goos`: darwin, linux, windows；`goarch`: amd64, arm64
- `checksum`：sha256，且 `name_template: checksums.txt`。GoReleaser v2 默认文件名是 `omni-cli_1.0.0_checksums.txt` 这种带版本号的形式，install.sh 想用稳定 URL 就必须固定成不带版本的 `checksums.txt`。
- `release.github` 指向 `itopener/omni-cli`
- 不配 `brews` / `scoops` / `winget`（非目标）

### 5. CI 与发布 workflow

- `.github/workflows/ci.yml`：push 与 PR 触发，跑 `go vet ./...` 和 `go test ./...`，`go-version-file: go.mod`。这是我加的，用户没点名要求；理由是公开仓库没有 CI 说不过去，且 standards 已经要求合并前全绿，把它自动化成本极低。CI 里跑 `go test` 不需要 docker：现有测试全部是进程内纯逻辑，ssh 测试用 `127.0.0.1:0` 起本地 server。
- `.github/workflows/release.yml`：`on: push: tags: ['v*']`，`permissions: contents: write`，用 `goreleaser/goreleaser-action`，`env: GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}`。checkout 必须带 `fetch-depth: 0`，否则 GoReleaser 生成 changelog 时拿不到 tag 历史会失败。

### 6. `install.sh`

放在仓库根，用户侧命令：

```
curl -fsSL https://raw.githubusercontent.com/itopener/omni-cli/main/install.sh | sh
```

要求：

- 严格 POSIX sh。不依赖任何 bashism，**包括 `set -o pipefail`**（dash 下第一行就报错；Debian/Ubuntu 的 `/bin/sh` 就是 dash）。错误捕获靠显式检查每条命令的退出码。
- 探测平台：`uname -s` / `uname -m` → darwin|linux + amd64|arm64。Windows（含 MSYS/MINGW 特征）不支持，报错并指向 Releases 页面。
- 版本选择：默认走 GitHub API 取 latest release；支持 `VERSION` 环境变量与 `--version` 参数指定，两者都接受带或不带 `v` 前缀，内部剥掉。README 里带参示例必须写成 `curl -fsSL ... | sh -s -- --version=1.2.3` 形式——裸管道传不了参数。
- 下载：release tarball 与 `checksums.txt` 都下到 `mktemp -d` 出来的临时目录，`trap` 负责清理。
- 校验：强制 sha256。优先 `sha256sum`，回退 `shasum -a 256`；两者都没有则**报错退出**，不允许「跳过校验继续装」。校验不匹配同样报错退出并删掉已下载文件。
- 安装位置：`--prefix` 可指定，默认 `/usr/local/bin`；默认路径无写权限时回落到 `$HOME/.local/bin`，并提示用户把该目录加进 PATH。
- 自检：**用安装后的绝对路径**执行 `<prefix>/omni-cli --version`，不看 PATH。回落目录通常不在 PATH 里，且 PATH 里可能残留旧版本二进制，按 PATH 查会自检到错误的程序。
- 失败一律非 0 退出，错误信息说明失败原因，不静默。

### 7. 依赖升级：`golang.org/x/crypto`

从 v0.17.0 升到本地工具链能承受的最高版本（不低于 v0.35.0，同时覆盖 CVE-2024-45337 与 CVE-2025-22869）。

这是我加的，用户没点名。理由：v1 的 ssh 隧道功能直接用 `x/crypto/ssh`，公开分发带已知认证绕过 CVE 的版本说不过去。升级属于依赖版本变更，不算「改运行时行为」的反目标；反目标那条针对的是逻辑改动。升级后 `go test ./...` 全绿才算完。

### 8. 仓库元数据文件与发布范围

- `LICENSE`：MIT。版权行 holder 取 `itopener`。**这是我替用户取的值**，若要写真实姓名或公司主体，批准时说一声。
- `README.md`：一句话定位（面向 AI 的研发场景 CLI 统一入口，Go 原生驱动直连 MySQL/PostgreSQL/Redis）、安装三种方式（install.sh / Releases 手动下载 / 源码构建）、quickstart 最小 `config.yaml`（密码一律写 `${ENV_VAR}` 形式）、命令清单、`config schema` 用法、macOS Gatekeeper 提示（只针对浏览器下载的压缩包；`curl | sh` 装的不带 quarantine 属性，不会被拦——提示写窄了会吓退用户，写宽了会误导）、License。示例中不得出现真实凭据或真实内网地址。
- `.gitignore`：已建好（二进制产物 / `dist/` / `.idea/` / `.vscode/` / OS 垃圾 / `*.local.yaml` / `.env`）。
- **公开发布范围**：仓库里现有的 `CLAUDE.md`、`docs/`（含 `docs/superpowers/plans/`——已被 git 跟踪）、本设计文档所在的 `docs/superpowers/specs/`，全部随 main 公开。`.idea/` 由 `.gitignore` 排除，`.superpowers/` 自带 `.gitignore` 排除。这是我的取值；若设计过程文档不想公开，批准时说一声，把 `docs/superpowers/specs/` 从提交范围里去掉并加进 `.gitignore`。
- `docs/design.md` 的「发布（二期，方案已定）」章节：更新为本轮实际落地范围，并明确 Homebrew Tap 与 Scoop/winget 仍然待做。

### 9. 发布顺序（顺序不可打乱）

当前分支 `feat/omni-cli-v1` 领先 `main` 13 个 commit，另有 12 个文件未提交（`internal/sshtunnel/` 新增、`cmd/conn_open_test.go` 新增等）。

1. 先把在途工作提交完（范围见上节的发布范围），`GOTOOLCHAIN=local go test ./...` 全绿，合并到 `main`。
2. 推送 `main` 到已配置好的 `origin`（首次推送）。
3. 本设计涉及的发布改动（module 改名、目录移动、`x/crypto` 升级、goreleaser、CI、install.sh、README、LICENSE、docs 更新）走单独分支，合回 `main`。
4. 在 `main` 上打 `v1.0.0` tag，推 tag，触发 release workflow。
5. 验证：Releases 页面有 6 个平台产物 + `checksums.txt`；在干净环境完整跑一遍 install.sh；装完 `omni-cli --help` 与 `omni-cli config schema` 正常输出。

第 3 步的 module 改名会牵动所有 import，所以第 1 步必须先做完，避免两件事搅在同一个 diff 里。

## 已知取舍

- **未做 macOS 签名与公证**：浏览器下载的压缩包首次运行会被 Gatekeeper 拦（「无法验证开发者」），需要 `xattr -d com.apple.quarantine <binary>` 或在访达里右键打开。`curl | sh` 安装路径不受影响（curl 不设置 quarantine 属性）。README 把这两条分开写。彻底解决需要 Apple Developer 账号（99 美元/年），本轮不做。
- **未做 Homebrew**：Mac 用户走 install.sh。
- **install.sh 依赖 raw.githubusercontent.com 与仓库名**：仓库改私有、改名或改默认分支名，脚本 URL 立即失效。
- **GitHub API 取 latest 有匿名 rate limit**：CI 环境或共享出口 IP 下可能触发。install.sh 需要在这种情况下给出明确提示（告诉用户改用 `VERSION=` 指定版本），而不是抛一个裸的 curl 错误。
- **`x/crypto` 升级版本受本地 go 1.23.4 工具链约束**：够修掉两个 CVE，但追不了最新大版本。工具链升级不在本轮。

## 留给实现者决定

- archives 的命名模板与格式（windows 用 zip、unix 用 tar.gz 是常见做法，具体模板自定）。
- `ci.yml` 是否额外加 golangci-lint（standards 没有强制某个 lint 工具）。
- install.sh 取 latest 的具体 GitHub API 端点，以及 JSON 解析方式（不能假设用户机器上有 `jq`）。
- README 的措辞与章节顺序。
- GoReleaser 版本在 action 里的固定方式（pin 具体版本 vs 跟随 latest）。
- `x/crypto` 的具体目标版本（下限 v0.35.0，上限由本地工具链决定）。
