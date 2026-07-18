# ytb2bili-go

YouTube → Bilibili 视频搬运工具 (Go 版本)

## 功能特性

| 功能 | 状态 | 说明 |
|------|------|------|
| **YouTube 搜索** | ✅ | InnerTube API，支持过滤器和分页 |
| **搜索直接提交** | ✅ | 搜索后直接提交到 B站 |
| **重复检测** | ✅ | 自动检测已提交的视频，避免重复 |
| **YouTube 下载** | ✅ | 使用 yt-dlp 下载视频、字幕、封面 |
| **语音转录** | ✅ | 使用 Bcut ASR (必剪) 免费转录 |
| **批量翻译** | ✅ | 使用 DeepSeek/LLM 并发翻译字幕 |
| **AI 元数据** | ✅ | 自动生成标题、简介、标签 |
| **B站投稿** | ✅ | 自动上传到 Bilibili |
| **频道监控** | ✅ | RSS 订阅频道更新 |

## 快速开始

### 安装

```bash
# 克隆项目
git clone https://gitee.com/difyz/ytb2bili-go.git
cd ytb2bili-go

# 编译
go build -o ytb2bili .

# 添加到环境变量 (推荐)
echo 'export PATH="/home/ubuntu/ytb2bili-go:$PATH"' >> ~/.bashrc
echo 'alias y2b="ytb2bili"' >> ~/.bashrc
source ~/.bashrc

# 现在可以使用别名 y2b
y2b --help
```

### 配置

```bash
# 设置环境变量
export DEEPSEEK_API_KEY="your...port YOUTUBE_COOKIES="/path/to/youtube_cookies.txt"
```

### 使用

```bash
# 登录 B站
y2b login

# 搜索 YouTube 视频
y2b search "Flutter tutorial"

# 搜索并直接提交第 1 个视频
y2b search --submit 1 "Flutter tutorial"

# 搜索并提交（跳过翻译）
y2b search --submit 1 --skip-translate "Flutter tutorial"

# 查看已提交的历史记录
y2b search --history

# 搬运视频
y2b submit "https://www.youtube.com/watch?v=VIDEO_ID"

# 仅测试（不上传）
y2b submit --dry-run "https://www.youtube.com/watch?v=VIDEO_ID"

# 添加 YouTube 频道订阅
y2b channel add --title "频道名称" <channel_id>

# 同步频道更新
y2b channel sync

# 查看发现的视频
y2b channel videos
```

## 命令行参数

### search 命令

```bash
y2b search [选项] <关键词>

选项:
  --max          最大结果数 (默认: 10)
  --json         输出 JSON 格式
  --sort         排序方式: relevance, upload_date, view_count, rating
  --date         上传日期: last_hour, today, this_week, this_month, this_year
  --duration     时长过滤: short(<4m), medium(4-20m), long(>20m)
  --type         类型: video, channel, playlist, movie
  --features     功能过滤: live, 4k, hd, subtitles, cc
  --submit       直接提交指定序号的视频 (如: --submit 1)
  --skip-translate 提交时跳过翻译
  --dry-run      提交时仅处理不上传
  --history      显示已提交的历史记录
```

**示例:**

```bash
# 基本搜索
y2b search "AI tutorial"

# 按观看量排序 + 长视频
y2b search --sort view_count --duration long "AI tutorial"

# 本周上传的视频
y2b search --date this_week "coding"

# 功能过滤: 4K + 字幕
y2b search --features 4k --features subtitles "travel"

# JSON 输出
y2b search --json --max 5 "Go programming"

# 搜索并直接提交第 1 个视频
y2b search --submit 1 "Flutter tutorial"

# 搜索并提交（跳过翻译）
y2b search --submit 1 --skip-translate "Flutter tutorial"

# 查看已提交的历史记录
y2b search --history
```

### submit 命令

```bash
y2b submit [选项] <YouTube URL>

选项:
  --source-lang    源语言 (默认: en)
  --target-lang    目标语言 (默认: zh)
  --tid            B站分区ID (默认: 122)
  --dry-run        仅处理不上传
  --skip-translate 跳过翻译
  --chain          自定义任务链，使用逗号或 > 分隔
  --show-plan      只显示规划结果，不执行
  --planner        规划器：adaptive 或 agent
  --goal           交给 Agent 的自然语言任务目标
```

任务链默认由规划器根据参数自动生成：

```bash
# 查看默认规划，不执行
y2b submit --show-plan "https://www.youtube.com/watch?v=VIDEO_ID"

# 用户指定目标步骤；缺少的依赖会自动补齐
# 实际规划为 download → transcribe → translate
y2b submit --chain translate "https://www.youtube.com/watch?v=VIDEO_ID"

# 只下载视频
y2b submit --chain download "https://www.youtube.com/watch?v=VIDEO_ID"

# 自定义完整任务链
y2b submit --chain 'download > transcribe > metadata' "https://www.youtube.com/watch?v=VIDEO_ID"

# 让配置的 LLM Agent 根据目标制定任务链
y2b submit --planner agent --goal "下载视频并生成中文字幕，但不要投稿" \
  --show-plan "https://www.youtube.com/watch?v=VIDEO_ID"
```

可用步骤为 `download`、`transcribe`、`translate`、`metadata` 和 `upload`。
`--dry-run` 会强制移除 `upload`，`--skip-translate` 会强制移除 `translate`，即使自定义或 Agent 规划包含这些步骤也不会绕过安全参数。规划器通过 `workflow.Planner` 接口与执行器解耦，可替换为 LLM Agent 规划器；最终计划仍必须经过步骤注册表和依赖校验后才能执行。

HTTP API 使用相同的任务链处理器，可在提交 JSON 中传入：

```json
{
  "url": "https://www.youtube.com/watch?v=VIDEO_ID",
  "chain": ["translate"],
  "planner": "adaptive",
  "goal": "生成中文字幕但不要投稿",
  "dryRun": true,
  "skipTranslate": false
}
```

CLI、队列 worker、HTTP/飞书服务和飞书多维表格现在共享 `internal/pipeline.Processor`；各入口只负责提供请求和展示进度。

任务链实现使用强类型 `PipelineState` 传递下载结果、字幕、元数据和投稿结果。各步骤位于独立实现中，由注册表适配到通用 workflow 执行器。Agent 规划和元数据生成共享 `internal/llm.Client`，请求会继承任务的 context，取消任务时可中断 LLM、yt-dlp、ffmpeg 和字幕翻译请求。

HTTP 提交响应中的 `task_id` 是任务全生命周期的唯一 ID，可用于查询统一的持久化状态：

```bash
curl http://localhost:8096/api/v1/tasks
curl http://localhost:8096/api/v1/tasks/TASK_ID
```

任务在规划完成后才会正式写入；`--show-plan` 和规划失败不会制造临时 pending 任务。HTTP 接口会先持久化 queued 状态，确保客户端收到的任务 ID 可以立即查询。

### channel 命令

```bash
# 添加频道
y2b channel add --title "MKBHD" UCBJycsmduvYEL83R_U4JriQ

# 同步频道（查看最近7天的视频）
y2b channel sync --lookback 7

# 查看发现的视频
y2b channel videos

# 查看统计
y2b channel stats
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
├── go.mod
├── internal/
│   ├── command/              # CLI 命令
│   │   └── command.go
│   ├── config/               # 配置管理
│   │   └── config.go
│   ├── search/               # YouTube 搜索 (InnerTube API)
│   │   ├── search.go         # 搜索逻辑
│   │   └── innertube.go      # InnerTube API 和 protobuf 编码
│   ├── download/             # 视频下载
│   │   └── download.go
│   ├── transcriber/          # 语音转录 (Bcut ASR)
│   │   └── transcriber.go
│   ├── translator/           # 批量翻译
│   │   └── translator.go
│   ├── metadata/             # 元数据生成
│   │   └── metadata.go
│   ├── bili/                 # B站 API
│   │   ├── auth.go
│   │   └── upload.go
│   ├── channel/              # 频道监控
│   │   └── channel.go
│   └── storage/              # 存储管理
│       ├── storage.go        # 任务和凭证存储
│       └── history.go        # 提交历史记录
├── data/
│   ├── cookies/              # 登录凭证
│   ├── downloads/            # 下载文件
│   ├── tasks/                # 任务记录
│   ├── history/              # 提交历史
│   └── subscriptions/        # 频道订阅
└── README.md
```

## 依赖

- **Go** >= 1.22
- **yt-dlp** - YouTube 视频下载
- **ffmpeg** - 音视频处理
- **deno** - JavaScript 运行时 (yt-dlp 需要)

### 安装依赖

```bash
# yt-dlp
sudo curl -L https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp -o /usr/local/bin/yt-dlp
sudo chmod a+rx /usr/local/bin/yt-dlp

# ffmpeg
sudo apt install ffmpeg

# deno
curl -fsSL https://deno.land/install.sh | sh
```

## API 说明

### YouTube InnerTube API

使用 YouTube 内部 InnerTube API 进行搜索，无需 API Key：

- **端点**: `https://www.youtube.com/youtubei/v1/search`
- **方法**: POST (JSON payload)
- **特点**: 支持过滤器、分页、无需认证

### Bcut ASR (必剪语音识别)

使用 Bilibili 必剪 API 进行免费语音转文字：

- **端点**: `https://member.bilibili.com/x/bcut/rubick-interface`
- **模型**: model_id = "8"
- **支持语言**: 中文、英文

### DeepSeek LLM

使用 DeepSeek API 进行批量翻译：

- **端点**: `https://api.deepseek.com`
- **模型**: deepseek-chat
- **特点**: 支持并发、上下文感知、自动重试

## 配置文件

### YouTube Cookies

导出 YouTube cookies 为 Netscape 格式：

1. 安装浏览器扩展 [Get cookies.txt LOCALLY](https://chromewebstore.google.com/detail/get-cookiestxt-locally/cclelndahbckbenkjhflpdbgdldlbecc)
2. 登录 YouTube
3. 导出 cookies 文件
4. 设置环境变量: `export YOUTUBE_COOKIES=/path/to/cookies.txt`

### B站登录

```bash
y2b login
# 扫描二维码完成登录
```

## 许可证

MIT License
