# ytb2bili-go 代码审查报告

审查日期: 2025-07-23
审查范围: /home/guan/guan/code/ytb2bili-go (56 个源文件, 12,928 行)
审查维度: 架构设计、代码质量、性能、安全性、错误处理、可维护性

---

## 一、架构总体评价

**整体评分: 7.5/10** — 架构合理，模块化程度高，但存在一些值得改进的结构性问题。

项目采用 Go 标准包结构，`internal/` 下的模块划分清晰：`command`(CLI 入口)、`pipeline`(工作流)、`workflow`(流程引擎)、`search/download/transcriber/translator/bili/metadata`(领域逻辑)、`storage`(持久化)、`config`(配置)、`server`(HTTP)、`feishu`(飞书集成)、`auth`(B站认证)、`audiosync`(音画同步)、`queue`(队列)、`channel`(频道监控)、`cdp`(Chrome DevTools)、`llm`(AI 客户端)。

### 1.1 值得肯定的架构决策

- **Pipeline + Workflow 引擎分离** (`internal/pipeline/` + `internal/workflow/`) — 将流程编排与业务逻辑分离，`workflow.Planner` 接口支持 Adaptive/Agent 两种规划器，扩展性好。
- **Planner 安全边界** — AgentPlanner 的 LLM 决策结果经 AdaptivePlanner 二次校验（dependency expansion + safety flag enforcement），防止 LLM 输出恶意或错误步骤。
- **依赖注入风格** — `Processor` 接受 `*config.Config`、`workflow.Planner`、`Reporter` 等依赖，便于测试。
- **任务链拓扑排序** — AdaptivePlanner 检测循环依赖（`visiting` map），使用 DFS 拓扑展开，避免无限递归。
- **字幕翻译计划** (`translationPlan`) — 对滚动字幕中重复文本只翻译一次再回填，减少 API 调用量。

### 1.2 关键架构问题

**[P0] 混合架构：新旧两套流程并行运行**  
`internal/command/command.go` 中的 `searchCommand` 和 `submitCommand` 使用**旧版**内联流程（task store → download → transcribe → translate → metadata → upload），而 `internal/pipeline/` 是**新版**插件化流程。两者功能重叠但走向不同的代码路径：
- 旧版：冗长的 `searchCommand.Action` 闭包（~230 行），耦合了搜索、历史检查、完整搬运流程。
- 新版：`Processor.Process()` + workflow Executor，可规划/可组合。

**建议**: 将旧版命令逐步迁移到新版 Pipeline。内部方法如 `startServer/stopServer/restartCommand` 也要统一。

**[P1] Config 加载策略不统一**  
`config.LoadYAML("config.yaml")`（YAML）和 `config.Load(path)`（TOML）并存。`config.Default()` 默认值被 `LoadYAML` 和 `Load` 重复定义。`Init()` 方法从环境变量覆盖配置字段——但 `DataDir`、`BiliTid` 等字段没有对应的环境变量回退。

**建议**: 统一为一种格式（推荐 TOML），或至少明确注释哪个是推荐的。统一环境变量覆盖策略。

**[P2] 字段命名不一致**  
- `skip_translate` vs `skipTranslate` vs `SkipTranslate` vs `--skip-translate`  
- `cookies` vs `CookiesPath` vs `cookiesFile`  
- `source_lang` vs `sourceLang` vs `SourceLang`  
- `audio_dir` vs `AudioDir` vs `audio-dir`  

不同层（CLI、HTTP JSON、pipeline.Request、struct field）使用了不同的命名约定。

---

## 二、代码质量详细审查

### 2.1 错误处理

**[P0] 大量忽略的错误返回值**  

文件 | 位置 | 代码
---|---|---
`storage/storage.go:183` | `SetBVID` | `json.Unmarshal(data, &t)` — 忽略反序列化错误
`storage/storage.go:201` | `SetCompleted` | 同上
`storage/storage.go:221` | `List` | `json.Unmarshal(data, &t)` — 静默忽略
`storage/storage.go:241` | `CredentialStore.Save` | `json.MarshalIndent(cred)` 省略 error
`storage/history.go:126` | `save` | `json.MarshalIndent` 省略 error
`auth/auth.go:68,80` | `apiGet`/`apiPost` | `json.Unmarshal(body, &result)` 忽略错误
`download/download.go:111` | `VideoContext` | `json.Unmarshal([]byte(lines[0]), &info)` — 静默忽略
`search/search.go:74` | `SearchWithFilter` | `json.Marshal(payload)` 忽略 error
`bili/bili.go:157` | `requestUpload` | `json.Marshal(payload)` 忽略 error

**[P1] 全部读取时的 panic 风险**  
多处 `resp.Body.Close()` 在读取前或读取后调用，最佳实践是使用 `defer` 加 `io.ReadAll()` 或 `json.NewDecoder()`。例如 `search/search.go:101` 在 `resp.Body.Close()` 后才使用 body。

**[P2] `fmt.Sscanf` 缺乏错误检查**  
`command/command.go:154` — `fmt.Sscanf(submitIdx, "%d", &idx)` 的返回值被忽略，输入非法字符串时 idx 保持零值。

### 2.2 并发安全

**[P0] `http.DefaultClient` 在多处被使用**  
- `transcriber/transcriber.go:163,207,235,262,288` — 所有 Bcut API 调用都使用 `http.DefaultClient`，没有任何超时配置。
- `search/search.go` 使用独立的 `s.Client`（有超时），但 `SearchPaginated` 没有重试机制。

**[P1] TaskStore 的锁粒度**  
`TaskStore.UpdateStep` 获取任务后释放锁，修改步骤时重新加锁——竞态条件窗口。同样的问题在 `SetBVID`、`SetCompleted` 中存在：读文件 → 释放锁 → 修改 → 加锁 → 写文件。应使用 `atomicWriteFile` + 文件级锁保持原子性。

### 2.3 安全审查

**[P0] 硬编码密钥**  
`internal/auth/auth.go:17-21` — 四个 B站 API 密钥/Secret 硬编码在源代码中：
```go
appKeyTV    = "4409e2ce8ffd12b8"
appSecTV    = "59b43e04ad6965f34319062b478f83dd"
appKey      = "783bbb7264451d82"
appSec      = "2653583c8873dea268ab9386918b1d65"
```
虽然后端密钥通常需要嵌入客户端，但这些值在 git 仓库中是明文。如果这些是敏感的 B站 API 密钥，应通过环境变量注入或以编译时标志传递。

**[P0] 凭证以明文 JSON 存储在磁盘上**  
`storage.NewCredentialStore` 将 B站 Cookie 和 Token 保存为 JSON 文件 (`data/cookies/bilibili.json`)，权限为 0600。虽然没有暴露给其他用户，但至少应该添加 GitHub 仓库的 `.gitignore` 规则。当前仓库根目录**没有 `.gitignore`**（检查确认），data 目录中的凭证可能被意外提交。

**[P1] 潜在路径穿越**  
`TaskStore.path(id)` 使用 `validTaskID` 检查 `..` 和 `/` 字符，但 `HistoryStore`、`SubtitleStore` 没有类似的路径检查。`SubtitleStore.path(videoID)` 直接拼接目录路径，如果 videoID 来自用户输入，可能导致路径穿越。

### 2.4 性能问题

**[P1] Bcut ASR 轮询间隔过高**  
`transcriber/transcriber.go:280-319` — 轮询间隔固定 2 秒，最多 300 次轮询（10 分钟）。对于长视频转录，频繁轮询浪费 API 配额和带宽。

**[P1] 整个文件加载到内存**  
`transcriber/transcriber.go:92` — `os.ReadFile(audioPath)` 将完整音频文件加载到内存，对于大视频（>1GB 音频）可能导致 OOM。应使用流式上传。

**[P2] B站字幕每次上传都获取视频时长**  
`bili/bili.go:143` — `getVideoDurationFromAPI` 在每次字幕上传时都请求 B站 API。对于多语言字幕上传（zh + zh-TW + ja），这意味着多次不必要的 API 调用。

**[P2] SRT 解析后保持全部在内存中**  
长视频（如 2 小时）的 SRT 文件可能有数千条字幕，全部保持在内存中。建议行处理流式解析。

### 2.5 可维护性

**[P1] 函数过长**  
- `searchCommand.Action` — ~230 行闭包，混合搜索、历史、下载、转录、翻译、元数据、上传逻辑。
- `bili.UploadContext` — 100+ 行，混合视频上传、封面上传、投稿提交。
- `search.parseVideoRenderer` — 140+ 行，大量类型断言嵌套。

**[P2] `map[string]interface{}` 滥用**  
搜索响应解析 (`search/innertube.go`、`search/search.go`) 大量使用 `map[string]interface{}` 类型断言解析 InnerTube 响应。这导致了 140+ 行的 `parseVideoRenderer` 函数。建议使用 `gojay` 或 `fastjson` 进行流式解析，或至少定义中间类型。

**[P1] 重复的工具函数**  
- `truncate()` 在 `bili/bili.go:237` 和 `metadata/metadata.go:71` 重复定义
- `min()` 在 `transcriber/transcriber.go:350` 定义（Go 1.21+ 已内置）
- `fileExists()` 在 `bili/bili.go:244` 定义

**[P2] 文件权限不一致**  
- `os.WriteFile(..., 0644)` in `command/command.go:352,367`  
- `os.WriteFile(..., 0600)` in `download/cookies.go:182`  
- `atomicWriteFile` 在 storage 包中传递 `0644` 或 `0600`  

应该统一凭证文件的权限策略。

### 2.6 测试覆盖率

项目有测试文件，但覆盖严重不足：

文件 | 测试内容 | 评价
---|---|---
`pipeline/pipeline_test.go` | 5 个测试 | 很好，覆盖了 ID 提取、plan-only、agent planner 验证
`workflow/workflow_test.go` | 8 个测试 | 很好，覆盖了 planner、executor、safety flags
`storage/*_test.go` | atomic、task、subtitle 测试 | 基础测试
`config/config_test.go` | 配置测试 | 部分覆盖
`download/download_test.go` | 下载测试 | 部分覆盖
`llm/client_test.go` | LLM 客户端测试 | 部分覆盖
`translator/translator_test.go` | 翻译器测试 | 部分覆盖
`audiosync/audiosync_test.go` | 音画同步测试 | 部分覆盖

**未测试的关键路径**:  
- `bili/bili.go` — 上传、字幕、审核检查（无 mock/接口测试）
- `search/search.go` — 搜索逻辑、解析器
- `server/server.go` — HTTP 路由、任务处理
- `auth/auth.go` — 认证流程
- 旧版 command.go 中的 submit/search 工作流

---

## 三、具体优化建议（按优先级）

### P0 — 必须修复

1. **统一两套流程** — 将 `command.go` 中的内联提交流程迁移到 `pipeline.Processor`，消除代码重复。

2. **添加 `.gitignore`** — 在仓库根目录创建 `.gitignore`，排除 `data/`、`*.log`、`*.pid`、`cookies/`、`credentials/` 等。

3. **处理忽略的错误** — 至少修复 `storage.SetBVID/SetCompleted` 和 `CredentialStore.Save` 中的静默错误忽略。

4. **移除 `http.DefaultClient` 依赖** — 在 transcriber 包中为每个 HTTP 调用设置有超时的 `http.Client`。

### P1 — 强烈建议

5. **为 InnerTube 响应定义强类型** — 将 `map[string]interface{}` 嵌套解析替换为生成的或手写的反序列化类型。

6. **为未测试代码路径添加集成测试** — 使用 `httptest.Server` mock 外部 API（B站、Bcut、DeepSeek），覆盖 upload、search、auth 路径。

7. **添加 request context 超时传播** — 部分长操作（Bcut 轮询 10 分钟、B站审核 24 小时）应响应 `ctx.Done()`。

8. **统一配置格式并补充环境变量** — 增加 `DATA_DIR`、`BILI_TID`、`YTB2BILI_BILI_TID` 等环境变量覆盖，确保所有 Config 字段都有环境变量覆盖路径。

### P2 — 建议改进

9. **添加 CI/CD 配置** — 添加 GitHub Actions（Go lint、test、build）和 Gitea CI。

10. **添加结构化日志** — 替换 `fmt.Printf` 和 `log.Printf` 为结构化日志库（如 `slog`、`log/slog`、`zerolog`）。

11. **字幕审核轮询后端优化** — 当前 `bili.WaitForReviewPassed` 使用 3 分钟间隔 × 24 小时。建议改为指数退避（1m→2m→4m→8m...）。

12. **Bcut ASR 分片上传并行化** — `uploadParts` 目前串行上传分片。对大型音频文件（>50MB），应使用 goroutine 并发上传。

---

## 四、具体文件问题清单

### main.go (33 行)
- ✅ 简洁，无重大问题

### internal/command/command.go (1805 行)
- ❌ **P0**: 旧版大量内联流程代码与 pipeline 重复
- ❌ **P1**: `searchCommand` 闭包过长 (~230 行)
- ❌ **P2**: `fmt.Sscanf` 错误被忽略
- ⚠️ `os.MkdirAll` 调用不完备（无错误检查）

### internal/config/config.go (113 行)
- ❌ **P1**: TOML 和 YAML 两种加载方式并存
- ❌ **P2**: `DataDir`、`BiliTid` 没有环境变量覆盖

### internal/pipeline/pipeline.go (207 行)
- ✅ 架构良好，依赖注入干净
- ⚠️ LLMDecisionProvider 的 `/chat/completions` 路径硬编码

### internal/pipeline/steps.go (207 行)
- ❌ **P1**: `ttsStep` 硬编码 Python 脚本路径和 API URL
- ⚠️ `uploadStep` 中的字幕同步错误被忽略 (`_, _ = ...`)

### internal/workflow/workflow.go (243 行)
- ✅ 设计清晰，Planner/Executor/Registry 分离
- ✅ 安全边界处理得当

### internal/search/search.go (766 行)
- ❌ **P2**: `parseVideoRenderer` 140+ 行嵌套类型断言
- ❌ **P2**: `SearchPaginated` 没有重试逻辑

### internal/bili/bili.go (247 行)
- ❌ **P1**: 封面上传的 goroutine + channel 模式复杂度过高
- ❌ **P2**: 每次字幕上传都调用 `getVideoDurationFromAPI`

### internal/audiosync/audiosync.go (105 行)
- ❌ **P2**: 使用 Python 脚本作为子进程 — 依赖外部运行时
- ✅ script discovery 逻辑考虑周全

### internal/translator/translator.go (967 行)
- ✅ 语义去重翻译计划 (`translationPlan`) 设计精巧
- ✅ 上下文感知的批量翻译
- ⚠️ `callLLM` 在其他文件中定义（需要确认完整性）

### internal/download/cookies.go (186 行)
- ✅ AES-GCM 解密实现正确
- ❌ **P0**: `COOKIES_ENCRYPT_KEY` 从环境变量获取 — 如果有.env 文件也要检查

### internal/storage/storage.go (575 行)
- ❌ **P0**: 多处 `json.Unmarshal` 错误被忽略
- ❌ **P1**: `UpdateStep` 存在读写竞争窗口
- ✅ `atomicWriteFile` 实现正确

### internal/transcriber/transcriber.go (355 行)
- ❌ **P0**: 大量使用 `http.DefaultClient`
- ❌ **P1**: 整个音频文件加载到内存

### internal/auth/auth.go (239 行)
- ❌ **P0**: 硬编码 B站 API 密钥/Secret
- ⚠️ `json.Unmarshal` 错误被忽视

---

## 五、总体体检报告表

| 维度 | 评分 | 主要问题 |
|------|------|----------|
| **架构设计** | 7.5/10 | 新旧流程双轨运行、格式不统一 |
| **代码质量** | 7/10 | 函数过长、错误处理不一致、重复代码 |
| **错误处理** | 5.5/10 | 大量静默忽略错误、Sscanf 无验证 |
| **并发安全** | 7/10 | TaskStore 锁粒度问题、DefaultClient 滥用 |
| **安全性** | 6/10 | 硬编码密钥、明文凭证存储、无 .gitignore |
| **性能** | 7/10 | 大文件内存加载、串行分片上传、固定轮询间隔 |
| **可维护性** | 7/10 | map[string]interface{} 泛滥、重复工具函数 |
| **测试覆盖** | 4/10 | 核心路径未覆盖（B站、搜索、认证、HTTP） |

**总分: 6.4/10** — 功能完整，架构方向正确（pipeline + workflow 是新亮点），但历史遗留代码质量参差，需要系统性清理和补测试。
