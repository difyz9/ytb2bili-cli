# ytb2bili-go 代码审查报告

> 审查日期: 2026-07-29
> 项目: ytb2bili-go (YouTube → Bilibili 视频搬运工具)
> 代码规模: ~13K 行 Go
> 审查范围: 全部 Go 源文件

---

## 目录

1. [安全相关问题](#1-安全相关问题)
2. [错误处理问题](#2-错误处理问题)
3. [并发与竞态问题](#3-并发与竞态问题)
4. [代码质量与设计问题](#4-代码质量与设计问题)
5. [资源管理问题](#5-资源管理问题)
6. [性能问题](#6-性能问题)
7. [可维护性问题](#7-可维护性问题)
8. [可测试性问题](#8-可测试性问题)
9. [整体建议](#9-整体建议)

---

## 1. 安全相关问题

### 1.1 [严重] 硬编码 API 密钥 (`internal/auth/auth.go:22-26`)

```go
var (
    appKeyTV = "4409e2ce8ffd12b8"
    appSecTV = "59b43e04ad6965f34319062b478f83dd"
    appKey   = "783bbb7264451d82"
    appSec   = "2653583c8873dea268ab9386918b1d65"
)
```

**问题**: B 站 API 的 appKey/appSec 被硬编码在源代码中。虽然 `init()` 会尝试读取环境变量覆盖，但硬编码的 fallback 值仍然可以通过逆向编译后的二进制文件提取。

**建议**: 
- 将这些值完全移出源码，只从环境变量或加密配置文件读取
- 如果必须保留硬编码值，至少加上明确的注释说明其用途和风险
- 考虑对这些敏感值做混淆处理或在 CI 构建时注入

### 1.2 [中等] 临时文件权限 (`internal/auth/auth.go:658`)

```go
os.WriteFile(authFile, []byte(qr.AuthCode), 0644)
```

**问题**: `auth_code.txt` 使用 `0644`（全局可读）权限，但包含临时的二维码认证码。

**建议**: 使用 `0600`（仅所有者可读写）权限。

### 1.3 [中等] Cookie 文件泄露风险 (`internal/download/download.go:73`)

```go
globalCookies = filepath.Join(os.Getenv("HOME"), "guan", "code", "ytb2bili-go", "data", "cookies", "youtube_cookies.txt")
```

**问题**: 硬编码了 HOME 目录下的特定路径作为 cookie 文件 fallback，路径泄露了用户名 `guan`。

**建议**: 不要使用硬编码的绝对路径，使用环境变量配置或可执行文件相对路径。

### 1.4 [信息] 缺少请求限流

所有 HTTP 客户端都没有配置限流（rate limiting），在批量处理大量视频时可能触发 YouTube/B 站 API 的限流策略。

**建议**: 引入 `golang.org/x/time/rate` 或自定义限流器。

---

## 2. 错误处理问题

### 2.1 [严重] 大量忽略的错误

多处代码中 `os.MkdirAll`、`json.Marshal`、`resp.Body.Close()` 等调用的错误被直接忽略。

**典型模式** (`internal/transcriber/transcriber.go:149-176`):

```go
payloadBytes, _ := json.Marshal(payload)  // 错误被忽略
req, _ := http.NewRequestWithContext(...)   // 错误被忽略
```

`internal/storage/storage.go` 中多处：
```go
os.MkdirAll(dir, 0755)  // 返回值忽略
```

**影响**: 
- `json.Marshal` 失败时将提交空 payload，导致 API 调用失败但难以排查
- `http.NewRequest` 失败时使用 nil `*http.Request` 直接 panic
- 目录创建失败导致文件写入异常，错误信息不完整

**建议**: 
- 使用统一的错误处理辅助函数，至少记录警告日志
- 对于 `http.NewRequestWithContext` 和 `json.Marshal` 这样的调用，不要忽略错误
- 考虑添加 lint 规则禁止忽略非 `_` 的错误

### 2.2 [中等] 不安全的类型断言 (`internal/auth/auth.go:143-144`)

```go
return &QRCodeData{
    URL:      data["url"].(string),
    AuthCode: data["auth_code"].(string),
}, nil
```

**问题**: 直接从 `map[string]interface{}` 进行类型断言而未检查断言是否成功。如果 API 返回格式异常的响应（字段缺失或类型不匹配），将产生 panic。

**应该在** `internal/search/search.go` 中存在相同的模式（大量没有检查的 map 断言）。

**建议**: 使用 comma-ok 模式：

```go
url, _ := data["url"].(string)
authCode, _ := data["auth_code"].(string)
if url == "" || authCode == "" {
    return nil, fmt.Errorf("无效的 QR 码响应: url=%q, auth_code=%q", url, authCode)
}
```

### 2.3 [中等] 缺少错误上下文

多处错误信息不包含原始错误，不利于调试。

**例如** (`internal/auth/auth.go:183-190`):
```go
case 86038:
    return nil, fmt.Errorf("二维码已过期")
case -3:
    return nil, fmt.Errorf("API错误")
```

**建议**: 包含状态码等上下文信息。

### 2.4 [中等] 静默吞错误

**例如** (`internal/storage/storage.go:154-157`):
```go
func (s *TaskStore) UpdateStep(id, stepName, status string, errMsg ...string) {
    t, err := s.Get(id)
    if err != nil {
        return  // 静默返回
    }
```

**影响**: 调用方以为状态更新成功，实际未生效。

**建议**: 至少记录日志。

---

## 3. 并发与竞态问题

### 3.1 [严重] `TaskStore` 锁机制不一致

**问题**: `SetBVID()` 和 `SetCompleted()` 内部自己获取锁并完整读写文件，但 `UpdateStep()` 调用 `Get()` 获取锁读取后，返回的 `t` 是值副本，然后重新获取锁进行写入。这期间其他 goroutine 可能已经修改了任务数据，导致更新丢失。

```go
// storage.go UpdateStep:
t, err := s.Get(id)     // ← 锁 A 获取并释放
// ← 竞态窗口：其他 goroutine 可能修改了任务
s.mu.Lock()             // ← 锁 A 重新获取，但 t 是旧数据
```

**建议**: 
- 统一所有写操作使用内部封装的方法，避免先读后写
- 引入乐观锁（版本号）或统一的 update 函数

### 3.2 [中等] Cover 上传 goroutine 泄漏 (`internal/bili/bili.go:63-79`)

```go
coverDone := make(chan string, 1)
go func() {
    url, err := uploadClient.UploadCover(params.CoverPath)
    // ...
    coverDone <- url
}()
select {
case url := <-coverDone:
    coverURL = url
case <-time.After(30 * time.Second):
    log.Printf("⚠ 封面上传超时(忽略)")
}
```

**问题**: 30 秒超时后，goroutine 仍然在后台运行（向已满的 channel 写入会阻塞，但如果其他部分退出了可能泄漏）。UploadCover 的 HTTP 请求没有绑定 context，超时后无法被取消。

**建议**: 
- 通过 context 传递超时
- 使用 `http.Client` 的 Timeout 属性而非手动的 channel+select

### 3.3 [中等] `translateTexts` 中使用无缓冲 context 的 goroutine

`translator.go` 中的 worker goroutine 使用 `select { case <-ctx.Done(): return }`，但只有 task 分发和结果收集时检查 context，`callLLM` 中的 HTTP 请求调用可能已经启动。

**建议**: 确保 HTTP 请求也使用传入的 context。

---

## 4. 代码质量与设计问题

### 4.1 [严重] `internal/command/command.go` 过于庞大

**情况**: 单一文件 1753 行，包含所有 CLI 命令的定义和业务逻辑。

**影响**:
- 难以测试（所有逻辑都包在闭包中）
- 难以维护和审查
- `searchCommand` 函数中包含完整的提交流水线（~150 行），与 `submitCommand` 逻辑重复

**建议**:
- 每个命令拆分为独立文件: `command_search.go`, `command_submit.go`, `command_queue.go` 等
- 将闭包中的业务逻辑提取为方法
- 消除 `searchCommand` 和 `submitCommand` 之间的流水线重复

### 4.2 [严重] YouTube ID 提取逻辑重复四份

| 位置 | 函数 |
|------|------|
| `internal/command/command.go:1289-1309` | `extractYouTubeID` |
| `internal/search/search.go:500-526` | `ExtractVideoID` |
| `internal/queue/queue.go:472-493` | `ExtractVideoID` |
| `internal/pipeline/pipeline.go:193-195` | `ExtractYouTubeID` (直接委托给 search) |

**问题**: 四份实现各有差异，`queue.go` 的实现不支持 `shorts/` 和 `embed/` 格式。

**建议**: 
- 统一为一个 `video ID` 提取函数，放在 `internal/download` 或 `internal/types` 包中
- 在 `extractYouTubeID` 提交时删除过期/不完整的 cookie 文件（`globalCookies` 的硬编码路径）

### 4.3 [中等] 重复的 `min`/`max` 函数

在 `search.go`、`translator.go`、`transcriber.go` 中各自定义了相同的 `min()` 函数。

**建议**: 
- Go 1.21+ 标准库已内置 `min`/`max`，可以直接使用 `cmp.Min`、`cmp.Max`
- 升级 Go 版本或抽到公共 `internal/util` 包

### 4.4 [中等] 两套 LLM 调用体系并存

- `internal/llm/client.go`: `OpenAIClient.Complete()` — 基础 LLM 调用
- `internal/translator/translator.go`: `callLLM()` — 手动构建 HTTP 请求（重复实现）
- `internal/pipeline/pipeline.go`: `LLMDecisionProvider.Decide()` — 使用 `OpenAIClient`

**问题**: 翻译器中重复实现了 LLM 调用逻辑，应该复用 `llm.Client` 接口。

**建议**: 翻译器应该使用 `llm.Client` 接口而非自己调用 HTTP API。

### 4.5 [中等] `search.go:SearchAndPrint` 违反单一职责

```go
func SearchAndPrint(query string, maxResults int) error
```

**问题**: 搜索模块中包含一个带 UI 输出的函数，输出逻辑耦合在数据层中。

**建议**: 移除或标记为 deprecated，由 CLI 命令层负责输出。

### 4.6 [中等] 硬编码的 B 站分区 ID (`internal/command/command.go:268`)

```go
Tid: 122,  // 硬编码
```

**问题**: `searchCommand` 的提交流水线中分区 ID 是硬编码的，而 `submitCommand` 中通过 flag 传入。两者行为不一致。

**建议**: 统一从配置或 flag 中读取。

### 4.7 [中等] `translator.go:SRTContext` 使用 `interface{}` 参数

```go
func SRTContext(ctx context.Context, inputPath, sourceLang, targetLang string, cfg interface{}) (string, error) {
    switch c := cfg.(type) {
    case *Config:
    case *config.Config:
    default:
        return "", fmt.Errorf("unsupported config type")
```

**问题**: 使用 `interface{}` 并类型断言来接受多种配置类型，失去了编译时类型检查。

**建议**: 定义一个接口或统一为一种配置类型。

---

## 5. 资源管理问题

### 5.1 [中等] HTTP 响应 body 关闭不严谨

多处在错误路径上未关闭 `resp.Body`：

```go
body, err := io.ReadAll(resp.Body)
resp.Body.Close()  // 错误路径上上如果 ReadAll 失败，body 未关闭
```

**应该使用**:
```go
body, err := io.ReadAll(resp.Body)
if err != nil {
    resp.Body.Close()
    return nil, err
}
resp.Body.Close()
```

或者更安全的模式:
```go
defer func() {
    if cerr := resp.Body.Close(); cerr != nil {
        // log or handle
    }
}()
```

### 5.2 [中等] `transcriber` 中 `http.DefaultClient` 使用

多处使用 `http.DefaultClient`（`transcriber.go:163, 207, 288`），缺乏超时控制。

**建议**: 创建一个带超时的自定义 client。

### 5.3 [信息] 提取的音频文件可能残留

`transcriber.go:89`:

```go
defer os.Remove(audioPath)
```

**问题**: 如果函数中间 panic 或系统崩溃，临时音频文件不会被清理。

**建议**: 在可接受的情况下接受这个风险，或在启动时清理上次的残缺文件。

---

## 6. 性能问题

### 6.1 [中等] `search.go` 每次请求都 `json.Marshal`

**问题**: 每次搜索请求都重新 marshal payload。

**建议**: 对于大部分相同的 payload，可以 marshal 一次后按需修改。

### 6.2 [中等] `SubtitleStore.ListPendingVideos` 全量加载

```go
func (s *SubtitleStore) ListPendingVideos() []string {
    entries, _ := os.ReadDir(s.dir)
    for _, e := range entries {
        // 读取并反序列化每个 JSON 文件
```

**问题**: 每次调用都读取并反序列化目录下的所有 JSON 文件。

**建议**: 对大量数据使用增量加载或建立索引缓存。

### 6.3 [中等] `transcriber.queryResult` 轮询策略

```go
maxRetries := 300
for i := 0; i < maxRetries; i++ {
    // ...
    time.Sleep(2 * time.Second)
}
```

**问题**: 固定 2 秒间隔轮询，最多等 10 分钟。过于保守。

**建议**: 使用指数退避（exponential backoff），早期轮询间隔更短，后期间隔更长。

---

## 7. 可维护性问题

### 7.1 [中等] 混合的日志/输出方式

代码中混合使用三种输出方式：
- `log.Printf` — 有前缀、时间戳
- `fmt.Printf` — 用户可见输出
- `fmt.Fprintf(os.Stderr, ...)` — 调试信息

**建议**: 
- 定义统一的 log 级别（trace/debug/info/warn/error）
- 运行时输出到 stdout，日志记录到 stderr 或文件
- 使用结构化日志库（可选）

### 7.2 [中等] 配置依赖传递过大

很多模块接收 `*config.Config` 但只使用其中 1-2 个字段：

- `translator.go:SRTContext` 接收整个 Config 但只用 APIKey/BaseURL/Model
- 各个 step 接收 `*config.Config` 但只使用 `DataDir`

**建议**: 使用接口隔离，按需定义小接口：

```go
type LLMConfig interface {
    APIKey() string
    BaseURL() string
    Model() string
}
```

### 7.3 [信息] YAML 解析的语义

`config.go:LoadYAML`:

```go
func LoadYAML(path string) (*Config, error) {
    cfg := Default()
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }
```

**问题**: 文件不存在时返回 error（上层 `main.go` 用 `Default()+Init()` fallback），但文件存在但内容为空时不报错。

**建议**: 明确区分"文件不存在"（使用默认值）和"文件损坏"（报错）两种情况。

### 7.4 [中等] `queue.go:isInHistory` 路径硬编码

```go
func isInHistory(videoID, queuePath string) bool {
    historyPath := filepath.Join(filepath.Dir(filepath.Dir(queuePath)), "history", "history.json")
```

**问题**: 基于文件路径推导历史文件位置，耦合了目录结构。

**建议**: 通过依赖注入传递 HistoryStore 或直接传递历史文件的路径。

---

## 8. 可测试性问题

### 8.1 [严重] CLI 命令逻辑不可测试

`command.go` 中所有命令的 `Action` 使用闭包，内部直接调用 `fmt.Printf`、`os.Exit` 等副作用。无法为命令逻辑编写单元测试。

**建议**: 
- 将命令的 Action 函数提取为可导出的方法
- 输出通过接口注入（如 `io.Writer`）
- 为提交流水线编写中间件式的测试

### 8.2 [中等] 缺少 mock 接口

- `downloader` 没有接口，测试中无法模拟 yt-dlp 调用
- `transcriber` 直接调用 HTTP，不可单元测试
- `bili.Upload` 直接调用真实 API

**建议**: 
- 为外部依赖定义接口
- 使用 [testify/mock](https://github.com/stretchr/testify) 或 [moq](https://github.com/matryer/moq) 生成 mock

### 8.3 [信息] 测试覆盖率低

现有测试文件主要是集成测试（需要真实的 API 密钥和外部服务），缺乏纯单元测试。

**建议**: 遵循测试金字塔：大量单元测试 + 适量集成测试 + 少量端到端测试。

---

## 9. 整体建议

### 9.1 短期修复（高优先级）

| 问题 | 文件 | 难度 |
|------|------|------|
| 修复不安全的 map 类型断言 | `auth.go`, `search.go` | 低 |
| 修复 goroutine 泄漏 | `bili.go:coverUpload` | 中 |
| 统一 YouTube ID 提取 | 3 个文件 | 低 |
| 修复 `TaskStore` 锁不一致 | `storage.go` | 中 |
| 消除 LLM 调用重复 | `translator.go` / `llm/client.go` | 中 |

### 9.2 中期重构

| 重构项 | 说明 |
|--------|------|
| 拆分 `command.go` | 按命令拆分为独立文件 |
| 统一日志系统 | 替换混合的 `log.Printf`/`fmt.Printf`/`fmt.Fprintf` |
| 消灭 `min`/`max` 重复 | 升级 Go 1.21+ 或抽公共 util |
| 接口隔离配置依赖 | 减少模块间耦合 |
| 提取测试接口 | 为外部依赖定义接口以便 mock |

### 9.3 长期架构建议

1. **引入依赖注入框架**或手动 DI：目前通过 `*config.Config` 传递全局依赖，模块间耦合度高
2. **事件驱动架构**：提交流水线目前是线性同步执行，可考虑事件驱动让各步骤更独立
3. **统一状态管理**：`TaskStore`、`HistoryStore`、`Queue`、`SubtitleStore` 四个存储体系可以合并或统一接口
4. **增加 prometheus/metrics** 监控：跟踪每个步骤的成功/失败率、处理时间等
5. **考虑使用 SQLite** 替代 JSON 文件存储：当前每个视频写多个 JSON 文件，大规模使用时有性能瓶颈

### 9.4 亮点

值得肯定的设计和实现：

1. **队列模块设计良好**（`internal/queue/queue.go`）：使用 `flock` + 临时文件 + `rename` 保证 crash-safe 和并发安全
2. **Pipeline 架构清晰**（`internal/pipeline/`）：`workflow` + `planner` + `executor` 的三层分离设计使得任务链可配置、可扩展
3. **安全过滤**（`search.go:ApplySafeSearch`）：搜索结果的去重、黑名单、时长和观看数过滤比较完善
4. **翻译计划**（`translator.go:buildTranslationPlan`）：滚动字幕的去重翻译 + 回填机制设计巧妙
5. **SRT 清理**（`bili/sanitize.go`）：自动截断超出视频时长的 BCC 字幕条目
6. **原子写文件**（`storage` 包多处）：使用临时文件 + `rename` 保证写入原子的模式是正确的

---

## 附录: 各文件主要问题统计

| 文件 | 行数 | 主要问题数 | 关键问题 |
|------|------|-----------|----------|
| `internal/command/command.go` | 1753 | 12 | 文件过大, 逻辑重复, 不可测试 |
| `internal/translator/translator.go` | 967 | 6 | interface{}参数, LLM调用重复 |
| `internal/search/search.go` | 792 | 5 | SearchAndPrint耦合, 类型断言 |
| `internal/storage/storage.go` | 600 | 4 | 锁不一致, 静默吞错误 |
| `internal/bili/bili.go` | 247 | 3 | goroutine泄漏 |
| `internal/auth/auth.go` | 300 | 3 | 硬编码密钥, 类型断言 |
| `internal/transcriber/transcriber.go` | 355 | 2 | DefaultClient使用 |
| `internal/download/download.go` | 233 | 2 | 硬编码路径 |
| `internal/queue/queue.go` | 513 | 2 | 路径硬编码, ExtractVideoID |
| `internal/llm/client.go` | 83 | 1 | 与translator重复 |
| `internal/config/config.go` | 103 | 1 | 空配置不报错 |
| `main.go` | 27 | 0 | — |
