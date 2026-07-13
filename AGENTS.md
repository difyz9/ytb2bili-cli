# AGENTS.md - AI 智能体使用指南

本文档供所有 AI 智能体（Codex、Claude Code、Hermes Agent、Cursor 等）使用。

> **首次安装项目？** 请先阅读 [INSTALL_AGENT.md](./INSTALL_AGENT.md) 获取完整的逐步骤安装指南（含依赖安装、编译、配置）。

## 项目概述

**ytb2bili-go** 是一个 YouTube → Bilibili 视频搬运工具，使用 Go 语言编写。

### 核心功能

| 功能 | 状态 | 说明 |
|------|------|------|
| YouTube 搜索 | ✅ | InnerTube API，支持过滤器和分页 |
| 搜索直接提交 | ✅ | 搜索后直接提交到 B站 |
| 重复检测 | ✅ | 自动检测已提交的视频，避免重复 |
| YouTube 下载 | ✅ | yt-dlp + cookies 认证 |
| 语音转录 | ✅ | Bcut ASR (必剪) 免费 |
| 批量翻译 | ✅ | DeepSeek LLM 并发 |
| AI 元数据 | ✅ | 自动生成标题/简介/标签 |
| B站投稿 | ✅ | 视频上传 |
| 字幕上传 | ✅ | 异步监听审核，通过后自动上传 |
| 频道监控 | ✅ | RSS 订阅更新 |

## 环境配置

```bash
# 必需的环境变量
export DEEPSEEK_API_KEY=*** export YOUTUBE_COOKIES="/home/ubuntu/ytb2bili-cli/cookies/youtube_cookies.txt"

# 必需的 PATH
export PATH="/home/ubuntu/.deno/bin:$PATH"

# 别名 (推荐)
alias y2b="ytb2bili"
```

## 项目位置

- **代码目录**: `/home/ubuntu/ytb2bili-go`
- **可执行文件**: `/home/ubuntu/ytb2bili-go/ytb2bili`
- **别名**: `y2b`
- **Gitee 仓库**: https://gitee.com/difyz/ytb2bili-go

## 快速命令

### 编译

```bash
cd /home/ubuntu/ytb2bili-go && go build -o ytb2bili .
```

### 搜索视频

```bash
# 基本搜索
y2b search "Flutter tutorial"

# 带过滤器搜索
y2b search --sort view_count --duration long "AI tutorial"

# JSON 输出
y2b search --json --max 5 "Go programming"

# 搜索并直接提交第 1 个视频
y2b search --submit 1 "Flutter tutorial"

# 查看已提交的历史记录
y2b search --history
```

### 完整搬运流程

```bash
y2b submit "https://www.youtube.com/watch?v=VIDEO_ID"
```

### 仅测试（不上传）

```bash
y2b submit --dry-run "https://www.youtube.com/watch?v=VIDEO_ID"
```

### 跳过翻译

```bash
y2b submit --skip-translate "https://www.youtube.com/watch?v=VIDEO_ID"
```

### 字幕管理

```bash
# 查看所有视频的字幕上传状态
y2b subtitle status

# 查看特定视频的字幕状态 (按 videoID 或 BVID)
y2b subtitle status BV1xx123

# 重试上传字幕 (审核通过后，按 videoID 或 BVID)
y2b subtitle retry BV1xx123
```

### 🤖 自主模式（批量自动搬运）

```bash
# 自动搜索本周高价值视频（按观看数排序）并提交前3个
y2b auto "AI tutorial" "programming" "tech news"

# 仅查看搜索结果，不上传
y2b auto --dry-run --max-videos 5 "python tutorial"

# 自定义过滤条件
y2b auto --min-views 5000 --max-duration 600 --date this_month "flutter tutorial"

# 搜索多个关键词，自动去重排序
y2b auto "machine learning" "deep learning" "neural network"

# 跳过翻译（保留原声英文字幕）
y2b auto --skip-translate "music production"
```

字幕采用**异步监听**机制：投稿后立即返回，后台 goroutine 每 30 秒检查一次审核状态，最多等待 24 小时。审核通过后自动用 `SubtitleUploader`（获取 CID → 转换 SRT → 保存草稿）上传字幕。上传状态持久化在 `data/subtitles/` 目录中，重启不丢失。

### 频道监控

```bash
# 添加频道
y2b channel add --title "频道名称" <channel_id>

# 同步更新
y2b channel sync --lookback 7

# 查看视频
y2b channel videos
```

### B站登录

```bash
y2b login
# 扫描二维码完成登录
```

## 项目结构

```
ytb2bili-go/
├── main.go                    # 入口
├── internal/
│   ├── command/              # CLI 命令
│   │   └── command.go        # 主命令和工作流
│   ├── config/               # 配置管理
│   │   └── config.go
│   ├── search/               # YouTube 搜索 (InnerTube API)
│   │   ├── search.go         # 搜索逻辑
│   │   └── innertube.go      # InnerTube API 和 protobuf 编码
│   ├── download/             # 视频下载
│   │   └── download.go       # yt-dlp 封装
│   ├── transcriber/          # 语音转录
│   │   └── transcriber.go    # Bcut ASR API
│   ├── translator/           # 批量翻译
│   │   └── translator.go     # LLM 翻译
│   ├── metadata/             # 元数据生成
│   │   └── metadata.go       # AI 生成标题/简介
│   ├── bili/                 # B站 API
│   │   └── bili.go           # 上传/字幕/审核
│   ├── channel/              # 频道监控
│   │   └── channel.go        # RSS 订阅
│   └── storage/              # 存储管理
│       ├── storage.go        # 任务和凭证存储
│       └── history.go        # 提交历史记录
├── CLAUDE.md                 # Claude Code 文档
├── AGENTS.md                 # 通用智能体文档
└── README.md                 # 用户文档
```

## 工作流程

```
┌─────────────────────────────────────────────────────────────┐
│                    ytb2bili-go 完整流程                       │
└─────────────────────────────────────────────────────────────┘

Step 0: 搜索视频
   ├── 使用 InnerTube API 搜索 YouTube
   ├── 支持过滤器 (排序/日期/时长/类型/功能)
   ├── 显示已提交状态 (✅已提交)
   └── 支持直接提交 (--submit)

Step 1: 下载视频
   ├── 使用 yt-dlp 下载 YouTube 视频
   ├── 下载 YouTube 官方字幕 (如有)
   └── 下载视频封面

Step 2: 分离音频 + 语音转录
   ├── 使用 ffmpeg 从视频中提取音频 (MP3)
   ├── 上传音频到 Bcut ASR (必剪) API
   └── 生成 SRT 字幕文件 (subtitle.srt)

Step 3: 字幕翻译
   ├── 读取英文字幕
   ├── 使用 DeepSeek LLM 批量翻译
   └── 生成中文字幕 (subtitle.zh.srt)

Step 4: AI 生成元数据
   ├── 使用 LLM 生成标题 (中文)
   ├── 生成视频简介
   └── 生成标签

Step 5: 视频投稿 B站
   ├── 上传视频文件
   ├── 上传封面图片
   ├── 提交投稿信息
   ├── 返回 BVID
   └── 记录到历史 (避免重复)

Step 6: [异步] 监听审核状态
   ├── 每 30 秒检查一次审核状态
   └── 最多等待 24 小时

Step 7: 审核通过后投稿字幕
   ├── 检测到审核通过
   └── 上传中文字幕到视频
```

## 核心模块 API

### 搜索模块 (search)

```go
// 基本搜索
searcher := search.New(10)
videos, err := searcher.Search("Flutter tutorial")

// 带过滤器搜索
filter := &search.SearchFilter{
    SortBy:     "view_count",
    Duration:   "long",
    UploadDate: "this_week",
}
result, err := searcher.SearchPaginated("AI tutorial", filter, "")

// 使用选项函数
result, err := search.New(5).SearchWithOptions("Go",
    search.WithSortBy("view_count"),
    search.WithDuration("long"),
    search.WithFeatures("4k", "subtitles"),
)
```

**搜索过滤器:**

| 过滤器 | 选项 |
|--------|------|
| `SortBy` | `relevance`, `upload_date`, `view_count`, `rating` |
| `UploadDate` | `last_hour`, `today`, `this_week`, `this_month`, `this_year` |
| `Duration` | `short` (<4m), `medium` (4-20m), `long` (>20m) |
| `Type` | `video`, `channel`, `playlist`, `movie` |
| `Features` | `live`, `4k`, `hd`, `subtitles`, `cc` |

### 历史记录模块 (storage/history)

```go
history := storage.NewHistoryStore(historyDir)

// 检查视频是否已提交
if history.IsSubmitted(youtubeID) {
    // 已提交，跳过
}

// 记录已提交的视频
history.Add(&storage.SubmittedVideo{
    YouTubeID: videoID,
    BVID:      bvid,
    Title:     title,
    Channel:   channel,
})

// 列出已提交的视频
videos, err := history.List()
```

### 下载模块 (download)

```go
result, err := download.Video(url, outputDir, lang, cookiesPath)
// result.VideoPath - 视频文件路径
// result.SubtitlePath - 字幕文件路径
// result.CoverPath - 封面图片路径
```

### 转录模块 (transcriber)

```go
srtPath, err := transcriber.BcutASR(videoPath, outputDir)
// 返回 SRT 字幕文件路径
```

### 翻译模块 (translator)

```go
trans := translator.New(Config{
    APIKey:     apiKey,
    BaseURL:    "https://api.deepseek.com",
    Model:      "deepseek-chat",
    SourceLang: "en",
    TargetLang: "zh",
    BatchSize:  25,
    MaxWorkers: 3,
})
err := trans.TranslateSRTFile(ctx, inputPath, outputPath)
```

### B站模块 (bili)

```go
// 上传视频
bvid, err := bili.Upload(&cred, &params)

// 上传字幕（使用 SubtitleUploader：获取CID → 转换SRT → 保存草稿）
err := bili.UploadSubtitle(cred, bvid, subtitlePath, "zh")

// 检查审核状态
status, err := bili.CheckReviewStatus(cred, bvid)

// 等待审核通过（30秒轮询，最长24小时）
status, err := bili.WaitForReviewPassed(cred, bvid)
```

## API 端点参考

| API | 端点 | 用途 |
|-----|------|------|
| YouTube InnerTube | `https://www.youtube.com/youtubei/v1/search` | 视频搜索 |
| Bcut ASR | `https://member.bilibili.com/x/bcut/rubick-interface` | 语音转文字 |
| DeepSeek | `https://api.deepseek.com` | LLM 翻译 |
| Bilibili | `https://api.bilibili.com` | 视频上传 |

## 常见任务

### 修改搜索逻辑

编辑 `internal/search/search.go` 或 `internal/search/innertube.go`

### 修改历史记录逻辑

编辑 `internal/storage/history.go`

### 修改下载逻辑

编辑 `internal/download/download.go`

### 修改转录逻辑

编辑 `internal/transcriber/transcriber.go`

### 修改翻译逻辑

编辑 `internal/translator/translator.go`

### 添加新的 CLI 参数

1. 编辑 `internal/command/command.go`
2. 在对应命令中添加 flag
3. 使用 `c.Bool("flag")` 或 `c.String("flag")` 获取

## 测试

```bash
# 测试搜索
y2b search --max 3 "test query"

# 测试转录
go test -v -run TestBcutASR ./internal/transcriber/ -timeout 3m

# 测试翻译
DEEPSEEK_API_KEY=*** go test -v -run TestTranslateSRT ./internal/translator/ -timeout 2m
```

## 调试技巧

1. 使用 `--dry-run` 测试完整流程但不上传
2. 检查 `data/downloads/` 查看下载文件
3. 查看 `data/tasks/` 的任务状态
4. 查看 `data/history/` 的提交历史
5. 查看 `data/subtitles/` 的字幕上传状态（持久化，重启不丢失）
6. 日志输出到 stderr

## 依赖工具

- `yt-dlp` - YouTube 视频下载
- `ffmpeg` - 音视频处理
- `deno` - JavaScript 运行时 (yt-dlp 需要)
