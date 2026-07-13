# CLAUDE.md - Claude Code 使用指南

## 项目概述

ytb2bili-go 是一个 YouTube → Bilibili 视频搬运工具，支持搜索、下载、转录、翻译、投稿完整流水线。

## 项目位置

- **代码目录**: `/home/ubuntu/ytb2bili-go`
- **可执行文件**: `/home/ubuntu/ytb2bili-go/ytb2bili`
- **别名**: `y2b` (已配置到 ~/.bashrc)
- **Gitee 仓库**: https://gitee.com/difyz/ytb2bili-go

## 环境配置

```bash
# 必需的环境变量
export DEEPSEEK_API_KEY=*** YOUTUBE_COOKIES="/home/ubuntu/ytb2bili-cli/cookies/youtube_cookies.txt"

# 必需的 PATH
export PATH="/home/ubuntu/.deno/bin:$PATH"

# 别名 (已配置)
alias y2b="ytb2bili"
```

## 快速开始

### 编译项目

```bash
cd /home/ubuntu/ytb2bili-go
go build -o ytb2bili .
```

### 使用别名

```bash
# 使用别名 y2b
y2b --help
y2b search "Flutter tutorial"
y2b submit "https://www.youtube.com/watch?v=VIDEO_ID"
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

### 下载并处理视频

```bash
# 完整流程：下载 → 转录 → 翻译 → 上传
y2b submit "https://www.youtube.com/watch?v=VIDEO_ID"

# 仅测试（不上传）
y2b submit --dry-run "https://www.youtube.com/watch?v=VIDEO_ID"

# 跳过翻译
y2b submit --skip-translate "https://www.youtube.com/watch?v=VIDEO_ID"
```

### 频道监控

```bash
# 添加频道
y2b channel add --title "MKBHD" UCBJycsmduvYEL83R_U4JriQ

# 同步频道更新
y2b channel sync --lookback 7

# 查看发现的视频
y2b channel videos
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

## 核心模块

### 1. 搜索模块 (search)

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

### 2. 历史记录模块 (storage/history)

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

### 3. 下载模块 (download)

```go
result, err := download.Video(url, outputDir, lang, cookiesPath)
// result.VideoPath - 视频文件路径
// result.SubtitlePath - 字幕文件路径
// result.CoverPath - 封面图片路径
```

### 4. 转录模块 (transcriber)

```go
// 使用 Bcut ASR 免费转录
srtPath, err := transcriber.BcutASR(videoPath, outputDir)
```

### 5. 翻译模块 (translator)

```go
trans := translator.New(Config{
    APIKey:     apiKey,
    BaseURL:    "https://api.deepseek.com",
    Model:      "deepseek-chat",
    SourceLang: "en",
    TargetLang: "zh",
})
err := trans.TranslateSRTFile(ctx, inputPath, outputPath)
```

### 6. B站模块 (bili)

```go
// 上传视频
bvid, err := bili.Upload(&cred, &params)

// 异步监听审核状态，审核通过后上传字幕
go watchAndUploadSubtitle(bvid, subtitlePath, &cred, cfg)
```

## 常用操作

### 测试单个模块

```bash
# 测试搜索
y2b search --max 3 "test query"

# 测试转录
go test -v -run TestBcutASR ./internal/transcriber/ -timeout 3m

# 测试翻译
DEEPSEEK_API_KEY=*** go test -v -run TestTranslateSRT ./internal/translator/ -timeout 2m
```

### 查看任务状态

```bash
y2b task list
y2b task show <task_id>
```

### 查看提交历史

```bash
y2b search --history
```

### B站登录

```bash
y2b login
# 扫描二维码完成登录
```

## API 端点

| API | 端点 | 用途 |
|-----|------|------|
| YouTube InnerTube | `https://www.youtube.com/youtubei/v1/search` | 视频搜索 |
| Bcut ASR | `https://member.bilibili.com/x/bcut/rubick-interface` | 语音转文字 |
| DeepSeek | `https://api.deepseek.com` | LLM 翻译 |
| B站投稿 | `https://api.bilibili.com` | 视频上传 |

## 依赖工具

- `yt-dlp` - YouTube 视频下载
- `ffmpeg` - 音视频处理
- `deno` - JavaScript 运行时 (yt-dlp 需要)

## 调试技巧

1. 使用 `--dry-run` 测试完整流程但不上传
2. 检查 `data/downloads/` 查看下载文件
3. 查看 `data/tasks/` 的任务状态
4. 查看 `data/history/` 的提交历史
5. 日志输出到 stderr，可重定向查看
