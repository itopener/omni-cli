# omni-cli 公开发布 v1.0.0 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 omni-cli 以 `github.com/itopener/omni-cli` v1.0.0 形式公开发布：module path 改名、main 目录移动、依赖 CVE 修复、GoReleaser + GitHub Actions 发布流水线、POSIX `install.sh`、LICENSE 与文档同步。

**Architecture:** 全部是构建/打包/文档层改动，不动 `internal/` 运行时逻辑。发布链：push tag `v*` → GitHub Actions → goreleaser-action 交叉编译 6 平台 → Release 附件 + `checksums.txt`。安装链：`install.sh` 探测平台 → 拼 Release 下载 URL → 强制 sha256 校验 → 落盘自检。

**Tech Stack:** Go 1.22（可能升 1.23）、GoReleaser v2、GitHub Actions、POSIX sh、python3 `http.server`（仅测试）。

**Spec:** `docs/superpowers/specs/2026-11-07-public-release-design.md`

## Global Constraints

- 所有 go 命令前缀 `GOTOOLCHAIN=local`。裸跑 `go mod tidy` 会挂死（本机环境坑）。
- install.sh 严格 POSIX sh：不用 `[[ ]]`、数组、`set -o pipefail`（本机有 `/bin/dash` 可验）。
- 无魔法值（Go 代码）；错误 `fmt.Errorf("...: %w", err)`；测试表驱动。
- README / 示例 / 测试 fixture 不得出现真实密码、真实 host、真实 token。
- 每个任务结束 `GOTOOLCHAIN=local go vet ./... && GOTOOLCHAIN=local go test ./...` 全绿才 commit。
- 远端现状：只有 `feat/omni-cli-v1`（= 本地 HEAD `101aaed`），无 `main`。

## Review Focus

spec 未点名、但最可能咬人的输入类；每条已在所属任务里配测试：

1. install.sh 在 dash 下跑（Debian/Ubuntu `/bin/sh`）——不能因 bashism 在第 1 行死掉。→ Task 5 用 `/bin/dash install.sh` 跑功能测试。
2. sha256 校验失败 / 校验工具缺失——必须中止且不留二进制。→ Task 5 失败路径测试。
3. `VERSION=v1.2.3` 带前缀、`--version=1.2.3` 不带——URL 都要拼对。→ Task 5 参数规范化测试。
4. 目录移动后 `-ldflags "-X main.version"` 符号失效风险——必须仍是 `main.version`。→ Task 4 snapshot 构建打印版本验证。
5. `go get` 升级 x/crypto 触发 go directive 升级连带 CI go 版本。→ Task 3 显式检查 go.mod 与 `go-version-file` 联动。

---

### Task 1: 建立 main 分支并推送

**Files:** 无代码改动，纯 git 操作。

**Interfaces:**
- Produces: 远端 `main` = `feat/omni-cli-v1` HEAD（`101aaed`）。后续任务从 main 切 `chore/release-v1.0.0`。

- [ ] **Step 1: 合并并推送**

```bash
git checkout main
git merge --ff-only feat/omni-cli-v1
git push -u origin main
```

Expected: `git ls-remote origin` 出现 `refs/heads/main`，指向 `101aaed`。若 push 需要交互式认证，把命令交给用户执行（`! git push -u origin main`），不要挂死等输入。

- [ ] **Step 2: 切发布分支**

```bash
git checkout -b chore/release-v1.0.0
```

---

### Task 2: module path 改名 + main 目录移动

**Files:**
- Modify: `go.mod`（module 行）、35 处 `"omni-cli/internal/..."` import（`cmd/` 与 `internal/` 全部 .go 文件）
- Rename: `cmd/` → `cmd/omni-cli/`（整目录，两步 git mv）
- Modify: `docs/development-standards.md:37,40`、`docs/design.md` 项目结构块、`README.md` 源码构建一节（`./cmd` → `./cmd/omni-cli`）

**Interfaces:**
- Produces: module path `github.com/itopener/omni-cli`；main 包在 `./cmd/omni-cli`；`go install github.com/itopener/omni-cli/cmd/omni-cli@latest` 产出名为 `omni-cli` 的二进制。

- [ ] **Step 1: 改 module 行与全部 import**

```bash
sed -i '' 's|^module omni-cli$|module github.com/itopener/omni-cli|' go.mod
grep -rl '"omni-cli/' --include='*.go' . | xargs sed -i '' 's|"omni-cli/|"github.com/itopener/omni-cli/|g'
grep -rn '"omni-cli/' --include='*.go' .   # 期望：无输出
```

- [ ] **Step 2: 移动 main 目录**（git mv 不能把目录移进自身，两步走）

```bash
git mv cmd cmd-tmp && mkdir cmd && git mv cmd-tmp cmd/omni-cli
```

- [ ] **Step 3: 同步三处文档引用**（development-standards.md 两行、design.md 结构块、README.md `go build -o omni-cli ./cmd` 改 `./cmd/omni-cli`）

- [ ] **Step 4: 验证**

Run: `GOTOOLCHAIN=local go vet ./... && GOTOOLCHAIN=local go test ./... && GOTOOLCHAIN=local go build -o /tmp/omni-cli-test ./cmd/omni-cli && /tmp/omni-cli-test version`
Expected: 全绿；版本输出 `omni-cli version 0.0.1`。

- [ ] **Step 5: Commit（两个，边界干净）**

```bash
git add -A && git commit -m "refactor: rename module path to github.com/itopener/omni-cli"
```

目录移动与文档引用并入同一 commit（移动无独立价值，分开会让中间 commit 构建路径断）。

---

### Task 3: x/crypto 升级（CVE-2024-45337 / CVE-2025-22869）

**Files:**
- Modify: `go.mod`、`go.sum`

**Interfaces:**
- Produces: `golang.org/x/crypto v0.35.0`；`go.mod` 的 `go` 指令允许被 `go get` 联动升到 `1.23.0`（CI 用 `go-version-file` 自动跟随）。

- [ ] **Step 1: 升级并整理**

```bash
GOTOOLCHAIN=local go get golang.org/x/crypto@v0.35.0
GOTOOLCHAIN=local go mod tidy
```

若报 `requires go >= 1.23.0`：接受它对 go 指令的修改（本地工具链 1.23.4 满足），不要手改更高。

- [ ] **Step 2: 验证**

Run: `GOTOOLCHAIN=local go vet ./... && GOTOOLCHAIN=local go test ./... && grep 'x/crypto' go.mod`
Expected: 全绿；输出含 `golang.org/x/crypto v0.35.0`。

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum && git commit -m "fix(deps): bump x/crypto to v0.35.0 (CVE-2024-45337, CVE-2025-22869)"
```

---

### Task 4: LICENSE + .goreleaser.yaml + workflows

**Files:**
- Create: `LICENSE`（MIT，版权行 `Copyright (c) 2026 itopener`）
- Create: `.goreleaser.yaml`
- Create: `.github/workflows/ci.yml`、`.github/workflows/release.yml`

**Interfaces:**
- Produces: 产物名模板 `omni-cli_{{ .Version }}_{{ .Os }}_{{ .Arch }}`（tar.gz，全平台统一）——Task 5 的 install.sh URL 依赖这个精确格式。checksum 文件名固定 `checksums.txt`。Os 取值 goreleaser 内建：`darwin` / `linux` / `windows`；Arch：`amd64` / `arm64`。

- [ ] **Step 1: LICENSE** — 标准 MIT 全文，年份 2026，holder `itopener`。

- [ ] **Step 2: `.goreleaser.yaml` 精确内容**

```yaml
version: 2
project_name: omni-cli
builds:
  - main: ./cmd/omni-cli
    binary: omni-cli
    goos: [darwin, linux, windows]
    goarch: [amd64, arm64]
    ldflags: ["-s -w -X main.version={{.Version}}"]
archives:
  - formats: [tar.gz]
    name_template: "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
checksum:
  name_template: "checksums.txt"
  algorithm: sha256
release:
  github:
    owner: itopener
    name: omni-cli
```

- [ ] **Step 3: `ci.yml`** — `on: [push, pull_request]`；步骤：checkout → `actions/setup-go@v5` 带 `go-version-file: go.mod` → `go vet ./...` → `go test ./...` → `sh scripts/test-install.sh`（Task 5 产出；此 workflow 先提交，Task 5 落地后 CI 才会绿，分支内顺序无碍）。

- [ ] **Step 4: `release.yml`** — `on: push: tags: ['v*']`；`permissions: contents: write`；步骤：checkout 带 `fetch-depth: 0` → setup-go（同上）→ `goreleaser/goreleaser-action@v6`，`env: GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}`。

- [ ] **Step 5: 本地 snapshot 验证（Review Focus #4）**

```bash
brew install goreleaser   # 已装则跳过
goreleaser release --snapshot --clean
```

Expected: `dist/` 下 6 个 `omni-cli_0.0.0-SNAPSHOT-*_darwin_arm64` 等目录、6 个 tar.gz、`checksums.txt`；`dist/omni-cli_0.0.0-SNAPSHOT*/omni-cli`（darwin/arm64 本机那份）执行 `--version` 打印注入的 snapshot 版本而非 `0.0.1`。

- [ ] **Step 6: Commit**

```bash
git add LICENSE .goreleaser.yaml .github/ && git commit -m "build: add goreleaser config, CI and release workflows, MIT license"
```

（`dist/` 已被 `.gitignore` 排除，确认 `git status` 干净。）

---

### Task 5: install.sh + 测试脚本

**Files:**
- Create: `install.sh`
- Create: `scripts/test-install.sh`（测试驱动本体，bash 写；被测脚本一律用 `sh`/`dash` 执行）

**Interfaces:**
- Consumes: Task 4 的产物名模板与 `checksums.txt` 文件名。
- Produces: `install.sh` 的稳定契约（Task 6 的 README 已按此写好）：
  - 默认下载基址 `https://github.com/itopener/omni-cli/releases/download`，latest 查询 `https://api.github.com/repos/itopener/omni-cli/releases/latest`；测试钩子环境变量 `OMNI_CLI_INSTALL_BASE` / `OMNI_CLI_INSTALL_API` 覆盖两者。
  - 参数：`--version=<x>`（接受 `v` 前缀并剥掉）、`--prefix=<dir>`；环境变量 `VERSION` 等价 `--version`。
  - 行为：平台探测（`uname -s/-m`，darwin/linux × amd64/arm64；Windows/MSYS 报错指向 Releases 页）→ 下载 `<base>/<tag>/omni-cli_<ver>_<os>_<arch>.tar.gz` 与 `checksums.txt` 到 `mktemp -d`（`trap` 清理）→ sha256 校验（`sha256sum` 或 `shasum -a 256`；缺失或失败即非 0 退出，不留二进制）→ 解压安装到 `--prefix`（默认 `/usr/local/bin`，无写权限回落 `$HOME/.local/bin` 并提示加 PATH）→ 用**绝对路径**执行 `<prefix>/omni-cli --version` 自检。
  - 内部函数名：`die <msg>`（统一错误出口，非 0 退出）、`normalize_version <v>`、`detect_platform`、`main`。

- [ ] **Step 1: 写失败测试 `scripts/test-install.sh`**

bash 表驱动风格，五个用例，每个用例自建 python3 `http.server` 假 Release 源（临时目录挂 `releases/download/v1.0.0/` 目录结构 + 一个假的 `latest` JSON），造一个可执行的假 `omni-cli`（shell 脚本打印 `omni-cli version 1.0.0`）打 tar.gz、算 sha256 生成 checksums：

1. dash 成功路径：`dash install.sh` + `OMNI_CLI_INSTALL_BASE` + `VERSION=1.0.0` + `--prefix=$TMP/bin` → 退出 0，`$TMP/bin/omni-cli` 存在且 `--version` 输出 `1.0.0`。
2. `VERSION=v1.0.0`（带前缀）与 `--version=1.0.0`（参数式）两条路 URL 拼对（服务端 404 即失败）。
3. 校验失败：篡改 checksums 里一个字符 → 非 0 退出，`$TMP/bin/omni-cli` 不存在。
4. 校验工具缺失：`PATH` 换成只含 `sh`/`curl`/`tar`/`uname`/`mktemp` 等必需品的 stub 目录（无 sha256sum/shasum）→ 非 0 退出。
5. `sh -n install.sh` 语法检查 + `/bin/dash -n install.sh`。

Run: `bash scripts/test-install.sh`
Expected: FAIL（install.sh 不存在）。

- [ ] **Step 2: 实现 `install.sh`**

按 Interfaces 契约写。要点：每条外部命令后显式查退出码（无 pipefail 可用）；API 取 latest 用 `curl -fsSL` + `grep -o '"tag_name": *"[^"]*"'` 提取（不依赖 jq）；失败信息区分「rate limit → 建议 VERSION= 指定」与普通网络错误。

- [ ] **Step 3: 跑测试至全绿**

Run: `bash scripts/test-install.sh`
Expected: 5 用例全 PASS。

- [ ] **Step 4: Commit**

```bash
git add install.sh scripts/test-install.sh && git commit -m "feat: add POSIX install.sh with mandatory sha256 verification"
```

---

### Task 6: 文档同步收口

**Files:**
- Modify: `README.md`（仅核对，不改内容——install/LICENSE 链接此时均已真实存在）
- Modify: `docs/design.md`「发布（二期，方案已定）」章节 → 本轮已落地范围 + Homebrew/Scoop 仍待做
- Modify: `CLAUDE.md`「当前阶段」：v1 含 ssh 隧道（`params` 的 `ssh_*` 字段），与已推送代码对齐

**Interfaces:** 无代码接口。

- [ ] **Step 1: 三处文档更新**（design.md 发布章节改写为：GoReleaser + Releases + install.sh 已落地，Homebrew Tap / Scoop / winget 待做；CLAUDE.md 的「只支持直连」改为「v1 支持直连 tcp + 可选 ssh 隧道」）

- [ ] **Step 2: 验证**

Run: `GOTOOLCHAIN=local go test ./... && grep -c "LICENSE" README.md && ls LICENSE install.sh`
Expected: 测试全绿；README 的 LICENSE 引用有真实文件承接。

- [ ] **Step 3: Commit**

```bash
git add docs/ CLAUDE.md README.md && git commit -m "docs: sync release scope, ssh tunnel status and install paths"
```

---

### Task 7: 合并、打 tag、发布、验证

**Files:** 无新文件。

**Interfaces:**
- Consumes: 前六个任务的全部产出。

- [ ] **Step 1: 合回 main**

```bash
git checkout main && git merge --ff-only chore/release-v1.0.0 && GOTOOLCHAIN=local go test ./...
```

- [ ] **Step 2: 打 tag**

```bash
git tag -a v1.0.0 -m "omni-cli v1.0.0"
```

- [ ] **Step 3: 推送（外发动作，先向用户确认）**

```bash
git push origin main --tags
```

用户确认后执行；若认证交互，转 `!` 前缀交给用户。

- [ ] **Step 4: 验证发布结果**

- Actions 页 `release` workflow 绿。
- Release 页面：6 个 tar.gz + `checksums.txt`。
- 干净环境全链路：`curl -fsSL https://raw.githubusercontent.com/itopener/omni-cli/main/install.sh | sh`（或手动 `VERSION=v1.0.0`）装出真实二进制，`omni-cli --version` 打印 `1.0.0`，`omni-cli config schema db` 有 JSON 输出。
- 任一不符 → 修完重打 `v1.0.1`，不删已发布的 `v1.0.0` tag。

---

## Self-Review 记录

- Spec 覆盖：§1 module 改名（T2）、§2 目录移动（T2）、§3 版本注入（T4/T7 验证）、§4 goreleaser（T4）、§5 CI/release（T4）、§6 install.sh（T5）、§7 x/crypto（T3）+ LICENSE/README/发布范围（T4/T6）、§9 顺序（T1→T7）——无缺口。
- 比例：install.sh 契约以接口块 + 测试用例表达，未抄全文；goreleaser/工作流 YAML 是 spec 钉死值，全文保留合理。
- Review Focus 五条各有归属任务与测试。
