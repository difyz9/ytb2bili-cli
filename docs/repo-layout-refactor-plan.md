# 仓库目录结构优化方案

> 分析日期：2026-09-02（基于 main 分支实测）
> 目的：将仓库组织方式对齐 Go 社区通用布局（golang-standards/project-layout 思路），为后续渐进式重构铺垫。
> 原则：**每个阶段独立可交付、可回滚、验证手段明确**；不盲从标准布局，凡不采用的条目均给出理由。

---

## 1. 现状盘点

### 1.1 当前结构（实测）

```
ytb2bili-cli/                      # 模块名 github.com/zolagz/ytb2bili-go（与目录名不一致）
├── main.go                        # 主入口在根目录，import internal/cmd
├── cmd/                           # ⚠️ 5 个一次性调试工具（非主入口）
│   ├── check_bitable/  llm-batch-translator/  test_cookies/
│   └── transcribe/     translate-srt/
├── internal/
│   ├── cmd/                       # ⚠️ 真正的 CLI 实现（包名 cmd 却位于 internal）
│   │   └── pipeline.go 等 15 个文件，共 5324 行（pipeline.go 单文件 2087 行）
│   ├── pipeline/  audiosync/  auth/  bili/  cdp/  channel/  config/
│   ├── download/  feishu/  llm/  metadata/  queue/  search/  server/
│   └── storage/  transcriber/  translator/  tts/  workflow/  ytoauth/
├── skills/audio-video-sync/       # 运行时资产，代码以 3 种相对路径查找
├── extension/                     # ⚠️ 完整 TypeScript/WXT 浏览器扩展项目（独立 package.json、3 个 Jenkinsfile）
├── scripts/refresh_youtube_cookies.sh
├── docs/（含 archive/）
├── config.yaml  config_schedule.yaml  cookies.txt  client_tv.json
├── client_web.apps.googleusercontent.com.json   # ⚠️ OAuth 凭证被 git 跟踪
├── deploy.sh  install-dpms-guard.sh             # 部署脚本在根目录
├── 003.srt  111.md  dev001.md  description.md  prd-analysis.md
├── prd-merge-analysis-mac17.md  backend-tech-research.md
├── tech-investigation-report.md  kanban-hermes-agent.md  warp-cli-notes.md
└── Makefile  README.md  AGENTS.md  CLAUDE.md  INSTALL_AGENT.md  LOCAL_DEPLOY.md
```

### 1.2 问题清单（按严重度）

**P0 — 安全（与本方案 Phase 1 直接相关）**

| # | 问题 | 证据 |
|---|------|------|
| 1 | OAuth 客户端凭证、YouTube cookies 被 git 跟踪 | `git ls-files` 含 `cookies.txt`、`client_tv.json`、`client_web.apps.googleusercontent.com.json` |
| 2 | `config.yaml` 被跟踪，仓库副本与部署机真实配置混用 | 同上；`.gitignore` 未排除 |

**P1 — 结构（Phase 2/3 处理）**

| # | 问题 | 说明 |
|---|------|------|
| 3 | 入口语义错乱 | 标准布局 `cmd/<binary>/` 应放主入口；本项目主入口在根 `main.go`，顶层 `cmd/` 反而是 5 个调试工具；真正的 CLI 包叫 `internal/cmd`（包名与位置矛盾） |
| 4 | `internal/cmd` 巨包 | 15 文件 5324 行；`pipeline.go` 单文件 2087 行，混合 search/submit/queue/auto/channel/task/chain/publish/subtitle 等十余个域的命令 |
| 5 | 顶层 `cmd/*` 工具与 `ytb` 子命令重复 | `transcribe`/`translate-srt`/`test_cookies` 功能已由 `ytb transcribe`/`ytb translate`/`ytb cookies test` 覆盖；`check_bitable`、`llm-batch-translator` 为早期调试脚本 |
| 6 | 前端子项目混入 Go 仓库 | `extension/` 自带构建体系（package.json、tailwind、Jenkinsfile×3、release 脚本），与 Go 工具链零共享 |

**P2 — 治理（Phase 1/4/5 处理）**

| # | 问题 | 说明 |
|---|------|------|
| 7 | 根目录 10+ 散落笔记/产物 | `003.srt`、`111.md`、`dev001.md`、`description.md`、4 份 prd/research 报告、`kanban-hermes-agent.md`、`warp-cli-notes.md` |
| 8 | 部署脚本位于根目录 | `deploy.sh`（INSTALL_AGENT.md 引用）、`install-dpms-guard.sh` 应归 `scripts/` |
| 9 | 疑似死文件 | `config_schedule.yaml` 代码零引用（grep 全仓库无 import/读取） |
| 10 | 无 CI | `.github/` 仅有 copilot-instructions.md，无 workflows，重构缺安全网 |
| 11 | auth 能力分散三处 | `internal/auth`、`internal/cdp`、`internal/ytoauth` + `internal/cmd/yt_oauth.go` |
| 12 | `skills/` 路径查找逻辑脆弱 | 代码以 cwd、exe 目录、源码相对路径三处兜底查找（`internal/audiosync/audiosync.go:102-108`），迁移目录会破坏运行时 |

### 1.3 现状中的优点（保留，不动）

- **`internal/` 业务包划分整体健康**：实测依赖关系无循环——底层包（search/download/storage/llm/feishu/queue/workflow）几乎不依赖兄弟包，`pipeline` 负责编排，`cmd`/`server` 是组合根。这是本次重构能低风险推进的基础。
- **所有 internal 包仅被本模块引用**，import path 变更影响面 = 模块内一次 find/replace + 编译验证。
- `data/` 已 gitignore、`docs/` 与 `docs/archive/` 已存在、`scripts/` 目录已就位。

---

## 2. 目标布局

### 2.1 目标目录树

```
ytb2bili-cli/
├── cmd/
│   └── ytb/                        # 主入口（唯一二进制）
│       └── main.go
├── internal/
│   ├── cli/                        # cobra 命令定义（原 internal/cmd 更名 + 按 Phase 3 拆分）
│   │   ├── root.go  json.go  format.go  qrcode.go  init.go
│   │   ├── search.go  submit.go  auto.go  channel.go  task.go
│   │   ├── queue.go  daemon.go  chain.go  history.go
│   │   ├── publish.go  review.go  subtitle.go  bili.go
│   │   ├── tools.go（下载/转录/翻译/TTS 等单步命令，可再拆）
│   │   └── yt_oauth.go  server.go
│   ├── pipeline/  audiosync/  bili/  channel/  config/  download/
│   ├── feishu/  llm/  metadata/  queue/  search/  server/
│   ├── storage/  transcriber/  translator/  tts/  workflow/
│   ├── ytauth/                      # （Phase 3 评估）auth + cdp + ytoauth 合并
├── configs/
│   └── config.example.yaml          # 示例配置入库；真实 config.yaml 本地化不入库
├── scripts/
│   ├── deploy.sh  install-dpms-guard.sh  refresh_youtube_cookies.sh
├── skills/                          # 暂留原位（Phase 4 治理路径后再议迁移）
├── extension/                       # 过渡保留，README 标注边界（Phase 5 决策拆库）
├── docs/（笔记归档至 docs/archive/）
├── .github/workflows/ci.yml         # Phase 0 建立
├── go.mod  go.sum  Makefile
└── README.md  AGENTS.md  CLAUDE.md  INSTALL_AGENT.md  LOCAL_DEPLOY.md
```

### 2.2 设计决策（含"明确不做什么"）

| 决策 | 结论 | 理由 |
|------|------|------|
| 创建 `pkg/` | **不创建** | 本项目是应用程序非库；标准布局自身也标注 pkg/ 可选且常被滥用。当前无外部消费者，全部代码留在 `internal/` 是正确姿势 |
| 创建 `test/` | **不创建** | 单元测试继续与代码同目录（Go 惯例）；e2e/冒烟脚本归 `scripts/` 即可 |
| 创建 `api/` | **不创建** | 无 OpenAPI/proto 契约；`internal/server` 是内部 HTTP 服务，不对外发布接口定义 |
| 模块名 `ytb2bili-go` vs 目录 `ytb2bili-cli` | **不改** | import path 不要求与目录名一致；改名会触发全量 import churn，收益仅观感。列为 Phase 5 可选项并给出触发条件 |
| `cmd/ytb/` 只放薄入口 | 是 | `main.go` 仅做 Version 注入 + Execute，命令实现留在 `internal/cli`，保持可测试性 |
| 根 `main.go` | 删除 | 移入 `cmd/ytb/main.go` 后根目录不再有 Go 源文件 |
| 顶层 `cmd/*` 调试工具 | 删除为主 | 功能已被 `ytb` 子命令覆盖；个别有用的（如 `llm-batch-translator` 的批量能力）评估后并入 `ytb` 子命令，而不是维护平行入口 |
| `extension/` | 短期保留 | 拆库是组织决策（影响 CI/Jenkins/release 流程），不与 Go 侧重构耦合；先加边界说明 |
| `skills/` 位置 | 暂不动 | 被 3 处相对路径查找引用且 systemd 服务依赖 cwd，迁移必须先收敛查找逻辑（Phase 4） |

---

## 3. 分阶段迁移方案

依赖关系：Phase 0 → 1 → 2 → 3 → 4 → 5。Phase 1/2/3 互相独立可暂停，Phase 4 依赖 2。

### Phase 0 — 建立安全网（不改任何文件位置）

**目标**：让后续每一步都有机械验证手段。

操作：
1. 建 `.github/workflows/ci.yml`：`go build ./...` + `go vet ./...` + `go test ./...`（跳过 `*_live_test.go` 类网络用例；CI 环境设 `GOPROXY=https://goproxy.cn,direct`，本机实测 proxy.golang.org 不可达）。
2. Makefile 增加 `verify` target（build + vet + test），作为每次迁移后的本地验收命令。
3. 打基线分支 `baseline/pre-refactor`，记录当前 `go test ./...` 结果与 `ytb --help` 输出快照（存 `docs/archive/`）。

验证：CI 全绿。
回滚：无（纯增量）。

### Phase 1 — 根目录卫生（不动 Go 代码，无 import 变更）

**目标**：清除安全隐患与杂物，根目录只剩"项目级"文件。

操作：
1. **凭证出库**：`git rm --cached cookies.txt client_tv.json client_web.apps.googleusercontent.com.json`；`.gitignore` 追加这 4 项（含 `config.yaml`，但先完成第 2 步）。
   ⚠️ 注意：git 历史仍保留这些内容，**需另行轮换 OAuth client secret 与 YouTube cookies**；如需彻底清除历史，单独决策 `git filter-repo`（涉及协作者强制同步，不并入本次）。
2. **配置示例化**：新增 `configs/config.example.yaml`（从当前 `config.yaml` 脱敏而来：密钥字段替换为占位符）；仓库中的 `config.yaml` 处理需区分场景——本仓库 checkout 同时是部署机工作目录（deploy.sh 流程），改为"本地 config.yaml 不入库 + 部署机已存在文件不受影响"，README/INSTALL_AGENT.md 补充说明。
3. **笔记归档**：`003.srt`、`111.md`、`dev001.md`、`description.md`、`prd-*.md`、`backend-tech-research.md`、`tech-investigation-report.md`、`kanban-hermes-agent.md`、`warp-cli-notes.md` → `git mv` 至 `docs/archive/`。
4. **脚本归位**：`git mv deploy.sh install-dpms-guard.sh scripts/`，同步更新 `INSTALL_AGENT.md` 等文档中的引用路径。
5. **死文件确认**：`config_schedule.yaml` 全仓库零引用，与维护者确认后删除。

验证：`make verify`；`ls` 根目录仅剩项目级文件；`grep -r deploy.sh` 文档引用全部指向新路径。
回滚：单 commit `git revert`。

### Phase 2 — 入口规范化（唯一一次全量 import path 变更）

**目标**：`cmd/ytb/` 成为唯一入口语义，`internal/cmd` 更名 `internal/cli`，删除平行调试入口。

操作（一个原子 commit，或拆为 3 个顺序 commit）：
1. `git mv main.go cmd/ytb/main.go`；Makefile `build` 改为 `go build -o ytb ./cmd/ytb`。
2. `git mv internal/cmd internal/cli`，包声明 `package cmd` → `package cli`（包内自引用无需改，仅 main.go 的 import 更新）。
3. 删除顶层 `cmd/` 5 个工具：
   - `transcribe`、`translate-srt`、`test_cookies`：功能已内置于 `ytb transcribe` / `ytb translate` / `ytb cookies test`，直接删；
   - `check_bitable`：无 internal 依赖的独立脚本，确认无使用（systemd/cron/文档均无引用）后删；
   - `llm-batch-translator`：有 `main_test.go` 且依赖 `internal/translator`，评估其批量能力是否值得做成 `ytb translate --batch-dir` 子命令，否则删。
4. 全量替换 import path：`github.com/zolagz/ytb2bili-go/internal/cmd` → `.../internal/cli`（仅 main.go 一处 + 文档中的路径引用）。

验证：`make verify`；冒烟清单逐条执行：
```
./ytb --help && ./ytb search --max 1 "test" && ./ytb queue status && ./ytb daemon status
```
运行时不受影响的依据：二进制名 `ytb` 不变、systemd 服务调用的是 `~/.local/bin/ytb`、config/data/skills 的 cwd 相对查找逻辑一行未动。
回滚：`git revert` 单 commit；或切回基线分支。

### Phase 3 — 拆分 `internal/cli` 巨包（不改 import，风险最低）

**目标**：`pipeline.go`（2087 行）按命令域拆文件，包内重组对外零感知。

操作：
1. 按目标布局 2.1 的文件清单拆分：`search.go`/`submit.go`/`auto.go`/`channel.go`/`task.go`/`queue.go`/`chain.go`/`publish.go`/`subtitle.go`/`review.go`/`history.go`，`pipeline.go` 拆完后删除。
2. 拆分只做"移动 + 重排"，不改逻辑；每个域一个 commit，方便二分定位问题。
3. auth 收敛评估（单独 PR）：`internal/auth` + `internal/cdp` + `internal/ytoauth` 边界审视，若职责重叠则合并为 `internal/ytauth`；此项涉及 import 变更，放在本阶段最后、确认收益后再做。

验证：`make verify`（现有 `internal/cmd` 测试随包迁移后应全部通过）；`ytb --help` 子命令清单与拆分前 diff 为空。
回滚：逐域 commit revert。

### Phase 4 — 运行时资产与配置路径治理

**目标**：把 `skills/`、config 的"cwd 相对查找"收敛为显式配置，解除目录位置与运行时行为的耦合，为未来 `assets/` 迁移解锁。

操作：
1. `internal/config` 增加 `skills_dir`（默认 `skills`，支持绝对路径）；`internal/audiosync`、`internal/pipeline/steps.go`、`internal/cmd/init.go` 三处相对路径查找统一改为读配置。
2. systemd unit（`ytb-batch-loop.service`、`index-tts.service`）与 deploy.sh 中的 cwd/路径约定复核，必要时通过 `WorkingDirectory=` + 配置显式化钉死。
3. 全量同步文档：AGENTS.md、CLAUDE.md、INSTALL_AGENT.md、README.md、`.claude/skills/*/SKILL.md`、`.cursorrules` 中所有涉及目录路径的描述（Phase 2 已动过的再复核一遍）。
4. （可选，解锁后）`skills/` → `assets/skills/` 的迁移评估。

验证：`make verify` + 在干净目录（非项目根）执行 `ytb audio-sync <已有 videoId>` 验证路径解析不依赖 cwd。
回滚：配置项有默认值，行为等价于现状，revert 即可。

### Phase 5 — 远期可选项（默认不做，列出触发条件）

| 项 | 触发条件 |
|----|----------|
| `extension/` 拆独立仓库 | Go 侧与扩展侧发布节奏持续互相阻塞，或需要独立 CI 权限/协作者管理 |
| 模块更名 `ytb2bili-go` → `ytb2bili-cli` | 出现真实外部 import 需求或模块名歧义造成实际故障时 |
| git 历史清除凭证 | 完成凭证轮换后仍有合规要求时（`git filter-repo`） |

---

## 4. 风险与缓解

| 风险 | 等级 | 缓解 |
|------|------|------|
| import path 全量变更引入笔误 | 低 | Phase 2 原子提交 + `go build ./...` 机械验证；变更点实测仅 `main.go` 一处 + 文档 |
| systemd 部署机与仓库不同步 | 中 | Phase 2 不改运行时查找逻辑；部署机只依赖编译产物 `ytb`，路径不变；上线前在部署机 `make install` + 冒烟 |
| 凭证仍在 git 历史 | 高（遗留） | 本方案只做"停止跟踪"；轮换凭证另行执行；历史清除单独决策 |
| 拆 `pipeline.go` 时隐式共享状态被破坏 | 低 | 只移动不重写；逐域 commit；包内测试 + CLI 冒烟兜底 |
| `skills/` 迁移破坏 daemon 运行 | 中 | Phase 4 前置条件是路径查找配置化 + 非项目根目录验证；不满足则保持原位 |
| 文档与代码漂移（AGENTS.md 描述的结构已过时） | 中 | 每阶段收尾把"文档同步"列为完成标准之一（见 §6） |

---

## 5. 验收清单

- [ ] Phase 0：CI 全绿；`make verify` 可用
- [ ] Phase 1：根目录无凭证/笔记/部署脚本；`git ls-files` 不含 cookies/client_*.json/config.yaml
- [ ] Phase 2：`cmd/` 下仅 `ytb/`；根目录无 `main.go`；`grep -r "internal/cmd"` 全仓库零命中
- [ ] Phase 3：`internal/cli/` 无单文件超 ~500 行；`ytb --help` 子命令清单与基线 diff 为空；测试全绿
- [ ] Phase 4：非项目根目录下 `ytb audio-sync` 正常工作；AGENTS.md 项目结构图与实际一致
- [ ] 全程：每个 Phase 单独 commit，可独立 revert

## 6. 文档同步清单（每阶段收尾执行）

| 文档 | 需同步内容 |
|------|-----------|
| AGENTS.md | "项目位置"“项目结构”章节的路径 |
| CLAUDE.md / .cursorrules / .github/copilot-instructions.md | 目录结构与命令引用 |
| INSTALL_AGENT.md | deploy.sh 新路径、config.example.yaml 用法 |
| README.md / LOCAL_DEPLOY.md | 构建命令（`go build ./cmd/ytb`）、目录说明 |
| .claude/skills/*/SKILL.md | 步骤产物路径相关描述（如有） |
