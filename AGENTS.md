# AGENTS.md - AI 智能体使用指南

本文档供所有 AI 智能体（Codex、Claude Code、Hermes Agent、Cursor、Pi 等）使用。

> **首次安装项目？** 请先阅读 [INSTALL_AGENT.md](./INSTALL_AGENT.md) 获取完整的逐步骤安装指南（含依赖安装、编译、配置）。
> **Claude Code 用户？** 项目内置 skills（`.claude/skills/`），可用 `/ytb2bili-pipeline` 直接驱动完整工作流。

## 项目概述

**ytb2bili-go** 是一个 YouTube → Bilibili 视频搬运工具，使用 Go 语言编写。CLI 构建产物为 **`ytb`**（cobra 框架），支持搜索、下载、转录、翻译、TTS 配音、投稿、字幕上传完整流水线。

### 当前运行架构（2026-08 优化后）

```text
systemd 用户服务
├─ ytb-batch-loop.service → /home/guan/.local/bin/ytb daemon   # 主调度（Go 内置循环）
└─ index-tts.service      → IndexTTS2 TTS 服务（配音后端，localhost:18765）
```

- **调度已全部收敛到 `ytb daemon`**（替代旧 batch_loop.sh 的 bash 循环）：搜索→评分→去重→入队→串行处理，无限循环。
- **关键词/搜索参数单一来源 = `config.yaml` 的 `search:` 段**；调度参数在 `daemon:` 段。改关键词 = 改 yaml + `systemctl --user restart ytb`，不要再改任何脚本。
- 失败任务自动重试（默认 3 次）后停止并飞书告警；步骤超时（下载 30min / TTS 60min）自动 kill 重试。
- 心跳文件 `data/daemon/heartbeat.json`（批次/PID/当前任务/队列统计/状态），每 30s 刷新。

### 常用运维命令（Agent 直接执行）

```bash
cd /home/guan/guan/code/ytb2bili-cli

./ytb daemon status            # 查看 daemon 心跳（状态/批次/当前任务/队列统计）
./ytb daemon check-heartbeat   # 心跳新鲜度检查（cron 用，>15min 未更新告警，exit 1）
./ytb queue status             # 队列统计（排队中/处理中/已完成/失败）
./ytb queue list               # 队列明细（含失败原因）
./ytb queue retry-failed       # 手动重排队失败任务（确认已修复根因后再用）
./ytb task list / task show <id>   # 任务详情（失败步骤定位）
./ytb submit <URL>             # 手动提交单个搬运任务
./ytb submit <videoId>         # 续跑已有产物（幂等，跳过已完成步骤）
systemctl --user restart ytb   # 改配置/关键词后重启（ytb 是 ytb-batch-loop.service 的别名，等价）
systemctl --user status ytb     # 查看服务状态
journalctl --user -u ytb-batch-loop -f    # 实时日志
```

**服务别名**：`ytb-batch-loop.service` 已注册别名 `ytb.service`（位于 `~/.config/systemd/user/`，软链到同名单元），
因此 `systemctl --user {start,stop,restart,status} ytb` 均可用，效果与长名完全一致。若需重建别名：
`ln -s ytb-batch-loop.service ~/.config/systemd/user/ytb.service && systemctl --user daemon-reload`。

**注意**：不要手动再起一个 `ytb daemon`（会和 systemd 服务抢队列）；守护进程已由 systemd 管理。失败任务达重试上限后需要人工判断根因（常见：B站上传连接被重置=临时网络、YouTube cookies 过期、TTS 服务挂了），修复后再 `queue retry-failed`。


### 核心功能

| 功能 | 状态 | 说明 |
|------|------|------|
| YouTube 搜索 | ✅ | InnerTube API，支持过滤器和分页 |
| 搜索直接提交 | ✅ | `search --submit N` 直接提交到 B站 |
| 重复检测 | ✅ | 自动检测已提交的视频，避免重复 |
| YouTube 下载 | ✅ | yt-dlp + cookies 认证 |
| 语音转录 | ✅ | Bcut ASR (必剪) 免费 |
| 批量翻译 | ✅ | DeepSeek LLM 并发 |
| TTS 配音 | ✅ | 腾讯云 TTS / IndexTTS2 分段合成 |
| AI 元数据 | ✅ | 自动生成标题/简介/标签 |
| B站投稿 | ✅ | 视频上传 |
| 字幕上传 | ✅ | 异步监听审核，通过后自动上传 |
| 频道监控 | ✅ | RSS 订阅 + 自动入队 |
| 作业队列 | ✅ | 文件状态机，支持批量消费 |
| HTTP 服务 | ✅ | 内置 API Server（start/stop/status） |

## 环境配置

```bash
# 必需的环境变量
export DEEPSEEK_API_KEY=***

# 可选：YouTube cookies（防止下载频率限制）
export YOUTUBE_COOKIES="/path/to/youtube_cookies.txt"

# 可选：YouTube 下载专用代理（仅走 yt-dlp 下载/取信息，不影响 B站投稿/翻译）
# 格式: socks5://user:pass@host:port 或 http://user:pass@host:port
# 也可在 config.yaml 配 youtube_proxy:（环境变量优先）。
# 注意：机房 SOCKS5 IP 常被 YouTube 风控；代理 + 完整登录态 cookie（含 SID）缺一不可。
export YOUTUBE_PROXY="socks5://user:pass@host:port"

# 可选：自定义 LLM
export LLM_MODEL="deepseek-v4-flash"
export LLM_BASE_URL="https://api.deepseek.com"

# 可选：字幕翻译服务商（优先级: 环境变量 > config.yaml translation 段）
# 服务商: deepseek / tencent / baidu / ollama（详见 config.yaml translation 段）
export TRANSLATION_PRIMARY="deepseek"          # 主翻译服务
export TRANSLATION_FALLBACKS="tencent,ollama"  # 降级顺序（逗号分隔）
export TRANSLATION_RETRIES="2"                 # 主服务重试次数
# 本地 Ollama（可选，用于 ollama 服务商或兜底）
export OLLAMA_BASE_URL="http://localhost:11434"
export OLLAMA_MODEL="qwen2.5:7b"

# 可选：禁用 yt-dlp 缺失时的自动安装（默认开启，装到 ~/.local/bin 无需 sudo）
# export YTB2BILI_NO_AUTO_INSTALL=1

# 可选：配置文件路径（默认 ~/.ytb/config.yaml，其次当前目录 ./config.yaml）
export YTB2BILI_CONFIG="/path/to/config.yaml"

# 可选：运行时资源定位（skills/、.venv/，非项目根目录运行时用）
export YTB2BILI_PROJECT_DIR="/path/to/project-root"   # 项目根（含 skills/），优先于自动探测
export YTB2BILI_PYTHON="/path/to/python3"            # 指定 .venv 解释器
export YTB2BILI_AUDIO_SYNC_SCRIPT="/path/to/script"  # 指定音画同步脚本（最细粒度）
```

配置文件查找顺序：`--config <path>` → `$YTB2BILI_CONFIG` → `~/.ytb/config.yaml` → 当前目录 `config.yaml`。找不到时使用默认配置（内置 `data_dir=~/.ytb/data`）。

> ⚠️ `config.yaml` 及各类凭证（cookies/ OAuth client）均不入库；首次配置从 `configs/config.example.yaml` 复制脱敏模板。

## 项目位置

- **二进制**: `ytb`（`make build` 或 `go build -o ytb ./cmd/ytb` 生成）
- **配置**: `./config.yaml`（不入库，模板见 `configs/config.example.yaml`）或 `--config` 指定
- **数据目录**: `./data/`（`config.yaml` 的 `data_dir` 字段可改）
- **凭证**: `data/cookies/`（YouTube cookies；默认自动选目录下最新有效 `*.txt`，显式 `youtube_cookies:` 配置优先）/ `client_tv.json` / `client_web.apps.googleusercontent.com.json`（不入库，本地维护）
- **部署脚本**: `scripts/`（deploy.sh / install-dpms-guard.sh / refresh_youtube_cookies.sh）
- **仓库**: https://github.com/zolagz/ytb2bili-go （备选 Gitee: https://gitee.com/difyz/ytb2bili-go ）

## 快速命令

### 编译

```bash
make build          # 产出 ./ytb
# 或 go build -o ytb ./cmd/ytb
```

### 检查环境依赖

```bash
ytb init            # 自动检查 ffmpeg / yt-dlp / Python / deno / audio-sync .venv
ytb init --update   # 同时更新 yt-dlp
ytb init --pip      # 自动安装 Python 依赖
ytb init --venv     # 自动创建 audio-video-sync .venv 并安装配音依赖
```

### 配置与外部服务自检

```bash
ytb check           # 检查配置有效性 + LLM/DeepSeek key 可用性 + TTS + YouTube 代理连通性
ytb check --json    # 机器可读输出（stdout 仅 JSON，退出码 0=全通过 / 非0=有异常）
```

定位配置问题（如 API key 失效/带空白、IndexTTS 服务没起、代理挂了）优先跑 `ytb check`；
环境依赖是否安装用 `ytb init`。

### 搜索视频

```bash
# 基本搜索
ytb search "Flutter tutorial"

# 带过滤器搜索
ytb search --sort view_count --duration long "AI tutorial"

# 按上传时间过滤
ytb search --upload-date this_week "golang"

# JSON 输出（机器可读）
ytb search --json --max 5 "Go programming"

# 搜索并直接提交第 1 个视频
ytb search --submit 1 "Flutter tutorial"

# 查看已提交的历史记录
ytb search --history
```

### 完整搬运流程

```bash
# 下载 → 转录 → 翻译 → 元数据 → 上传 → 字幕（审核通过后自动）
ytb submit "https://www.youtube.com/watch?v=VIDEO_ID"
# 也支持直接用 videoId（11 位 ID）
ytb submit yn4MSHbKgmo

# 仅测试（不上传）
ytb submit --dry-run "https://www.youtube.com/watch?v=VIDEO_ID"

# 跳过翻译
ytb submit --skip-translate "https://www.youtube.com/watch?v=VIDEO_ID"

# 自定义任务链
ytb chain run download,transcribe,translate "https://www.youtube.com/watch?v=VIDEO_ID"
ytb chain list    # 查看可用步骤
ytb chain plan download,upload "https://www.youtube.com/watch?v=VIDEO_ID"  # 只规划不执行
```

**幂等续跑**：所有步骤会检查 `data/downloads/<videoId>/` 下的已有产物，存在则跳过——
download（视频文件）、transcribe（`.srt`）、translate（`.zh-Hans.srt`）、tts（`voice/` 配音）、audio-sync（`.synced.mp4`）。
重跑 `submit <videoId>` 会自动跳过已完成步骤，只做剩余部分（不会重新下载/转录/翻译/合成配音）。

**作为可组合 CLI 工具**：每个流水线步骤都可独立调用，并支持 `--json` 机器可读输出
（stdout 仅含 JSON，日志转 stderr，退出码 0=成功/非0=失败），可被外部 pipeline/Agent
作为单个工具步骤自由串联，不局限于 `submit`/daemon 一键流程。契约与组合示例见
`.claude/skills/ytb2bili-tool/SKILL.md`（或 `ytb2bili-tool` skill）。

```bash
ytb download --json "<URL>" > d.json      # {"ok":true,"step":"download","video":"..."}
ytb transcribe --json "<videoId>" > t.json
ytb translate --json "<id>.srt" > tr.json
ytb tts --json "<id>.zh-Hans.srt" > tts.json
ytb audio-sync --json "<videoId>" > as.json
ytb submit --json "<URL>"                  # {"ok":true,"step":"submit","bvid":"BV..."}
```

```bash
# 单独执行音画同步（对已有产物）
ytb audio-sync <videoId>
# 输出 data/downloads/<videoId>/<videoId>.synced.mp4，然后可 submit <videoId> 续跑收尾
```

### 任务管理

```bash
ytb task list [--json]           # 列出任务（显示真实步骤进度 [n/7]）
ytb task show <task_id>          # 查看任务详情（各步骤状态/错误）
ytb task retry <task_id>         # 重试失败任务（幂等跳过已完成步骤，从失败处续跑）
ytb task retry --dry-run <id>    # 重试但不投稿
```

### 作业队列（批量搬运）

```bash
ytb queue add "https://www.youtube.com/watch?v=VIDEO_ID"   # 加入队列
ytb queue status                                            # 队列统计
ytb queue list [--json]          # 列出队列中的视频（含失败原因）
ytb queue remove <videoID>       # 从队列移除
ytb queue clear                  # 清空整个队列
ytb queue retry-failed           # 将失败的视频重新排队
ytb queue work --once            # 消费一个视频后退出
ytb queue work                   # 持续消费（Ctrl+C 停止）
```

### 提交历史与诊断

```bash
ytb history [--json]             # 查看已提交的投稿历史
ytb debug                        # 环境/登录/数据统计诊断（排查用）
```

### B站投稿管理

```bash
# 直接投稿本地视频到 B站（不经 YouTube 流水线）
ytb publish video.mp4 --title "我的视频" --tags "科技,评测" --desc "简介"
ytb publish video.mp4 --tid 122 --cover cover.jpg --source "https://..."

# 查看投稿审核状态
ytb review BV1xx123
ytb review --wait BV1xx123     # 轮询直到审核通过（最长24小时）

# 上传字幕到已发布的视频
ytb subtitle upload BV1xx123 subtitle.zh-Hans.srt
ytb subtitle upload BV1xx123 subtitle.srt --lang en

# 查看所有视频的字幕上传状态
ytb subtitle status
```

### 🤖 自主模式（智能批量搬运）

```bash
# 默认 popular 评分，搜索后入队
ytb auto "Flutter tutorial" "AI programming"

# 均衡评分 + 播放量门槛，查看评分结果
ytb auto --dry-run --scorer balanced --min-views 1000 "python tutorial"

# 时效优先，只取本周发布
ytb auto --dry-run --scorer fresh --upload-date this_week "flutter tutorial"

# 时长过滤（short <4m / medium 4-20m / long >20m）
ytb auto --duration long "machine learning"

# 直接提交处理（入队后立即处理，不走 queue work）
ytb auto --submit --max-videos 5 "golang backend"

# 跳过翻译
ytb auto --submit --skip-translate "music production"

# 多关键词搜索，自动去重排序
ytb auto "machine learning" "deep learning" "neural network"
```

**评分策略:**

| 策略 | 权重 | 适用场景 |
|------|------|---------|
| `popular` (默认) | 播放量 × 0.7 + 时效 × 0.3 | 追求热门内容 |
| `fresh` | 时效 × 0.8 + 播放量 × 0.2 | 快速跟进新内容 |
| `balanced` | 播放量 × 0.34 + 时效 × 0.33 + 时长 × 0.33 | 综合择优 |

**工作模式：**

- **默认**（无 `--submit`）：自动搜索 → 评分筛选 → 接入任务队列（`queue`）。用户后续运行 `queue work` 消费队列，适合批量发现、慢慢处理。
- **`--submit`**：入队记录后直接处理，适合少量紧急搬运。
- **`--dry-run`**：仅展示评分结果，不写入任何数据。

**去重机制：** 自动对多关键词结果全局去重，并跳过已提交历史的视频。

### 频道监控

```bash
# 添加频道
ytb channel add --title "频道名称" <channel_id>
ytb channel list                  # 列出订阅
ytb channel remove <channel_id>   # 移除订阅

# 同步更新（仅处理最近 N 天发布的视频）
ytb channel sync --lookback 7

# 同步并自动入队（闭环频道监控 → 批量搬运）
ytb channel sync --lookback 7 --queue

# 查看发现的视频
ytb channel videos
```

### B站登录

```bash
ytb login         # 扫码登录（终端打印二维码）
ytb whoami        # 查看当前账号
ytb cookies test  # 测试 YouTube cookies 是否有效（自动选 data/cookies/ 最新文件，先剔除已轮换的 PSIDTS 令牌）
ytb cookies refresh  # 从 Chrome 刷新 YouTube cookies（Chrome 不在运行则自动拉起；daemon 也每 6h 自动刷，配置 daemon.cookies_refresh_hours）
```

### HTTP 服务

```bash
ytb server start      # 后台启动 API 服务
ytb server status     # 查看运行状态
ytb server stop       # 停止服务
ytb server restart    # 重启
ytb server run        # 前台运行（内部）
```

## 项目结构

```
ytb2bili-go/
├── cmd/
│   └── ytb/main.go           # 主入口（唯一二进制 ytb）
├── internal/
│   ├── cli/                  # CLI 命令 (cobra，按命令域拆分)
│   │   ├── root.go           # 根命令 + 配置加载
│   │   ├── submit.go         # submit（完整流水线一键投稿）
│   │   ├── search.go         # search + history
│   │   ├── auto.go           # auto 自主模式（搜索→评分→入队）
│   │   ├── daemon.go         # daemon 守护进程
│   │   ├── queue.go          # queue 作业队列
│   │   ├── task.go           # task 任务管理
│   │   ├── chain.go          # chain 任务链（可组合单步）
│   │   ├── channel.go        # channel 频道监控
│   │   ├── account.go        # login/whoami/accounts
│   │   ├── download.go       # download 单步
│   │   ├── transcribe.go     # transcribe/bcut/whisper 单步
│   │   ├── translate.go      # translate 单步
│   │   ├── tts.go            # tts 单步
│   │   ├── metadata.go       # metadata 单步
│   │   ├── audio_sync.go     # audio-sync 单步
│   │   ├── subtitle.go       # subtitle 字幕上传
│   │   ├── cookies.go        # cookies test/refresh
│   │   ├── bili.go           # publish/review B站投稿管理
│   │   ├── yt_oauth.go       # yt-oauth 订阅
│   │   ├── server.go         # server HTTP 服务
│   │   ├── chrome_debug.go   # Chrome 调试浏览器生命周期
│   │   ├── debug.go          # debug 诊断
│   │   ├── init.go           # 环境依赖检查
│   │   ├── format.go         # 输出格式化辅助
│   │   └── qrcode.go         # 二维码生成
│   ├── pipeline/              # 流水线处理器
│   │   ├── pipeline.go        # Processor (流程编排)
│   │   └── steps.go           # 各步骤实现
│   ├── workflow/              # 任务链规划/执行引擎
│   ├── queue/                 # 作业队列 (文件状态机)
│   ├── search/                # YouTube 搜索 (InnerTube API)
│   │   ├── search.go          # 搜索逻辑
│   │   └── innertube.go       # InnerTube API 和 protobuf 编码
│   ├── download/              # 视频下载 (yt-dlp 封装)
│   ├── transcriber/           # 语音转录 (Bcut ASR)
│   ├── translator/            # 批量翻译 (DeepSeek LLM)
│   ├── metadata/              # AI 生成标题/简介
│   ├── bili/                  # B站 API (上传/字幕/审核)
│   ├── channel/               # 频道监控 (RSS 订阅)
│   ├── cdp/                   # Chrome DevTools (cookies 刷新)
│   ├── tts/                   # 腾讯云 TTS
│   ├── audiosync/             # 音画同步
│   ├── server/                # HTTP API 服务
│   ├── resource/              # 运行时资源定位（skills/、.venv/ 统一入口，支持 skills_dir 配置）
│   ├── storage/               # 任务/凭证/历史/字幕存储
│   └── config/                # 配置管理
├── skills/
│   └── audio-video-sync/      # IndexTTS2 配音技能（运行时被 pipeline 引用）
├── .claude/skills/            # Claude Code 项目技能
├── CLAUDE.md                  # Claude Code 文档
├── AGENTS.md                  # 通用智能体文档
├── INSTALL_AGENT.md           # 安装指南
└── README.md                  # 用户文档
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

### 添加新的 CLI 命令或参数

1. 编辑 `internal/cli/` 下的文件（按命令域：`submit.go` / `queue.go` / `transcribe.go` 等，见项目结构图）
2. 新建命令用 `&cobra.Command{Use: "...", Short: "...", RunE: func(...) error {...}}` 包裹
3. 添加 flag 用 `cmd.Flags().String(...)` / `cmd.Flags().Bool(...)` / `cmd.Flags().Int(...)`
4. 在 `root.go` 的 `root.AddCommand(...)` 中注册
5. 读取 flag 用 `cmd.Flags().GetString("name")`

## 测试

```bash
# 测试搜索
ytb search --max 3 "test query"

# 运行单元测试
go test ./...

# 只测试某个包
go test ./internal/cli/ -v
```

## 调试技巧

1. 使用 `submit --dry-run` 测试完整流程但不上传
2. 检查 `data/downloads/` 查看下载文件
3. 运行 `task list` / `task show <id>` 查看任务状态和失败步骤
4. 查看 `data/history/` 的提交历史
5. 查看 `data/subtitles/` 的字幕上传状态（持久化，重启不丢失）
6. 检查登录态：`ytb whoami`；检查 cookies：`ytb cookies test`
7. 日志输出到 stderr，可重定向查看
8. HTTP 服务日志在 `data/server.log`

## 依赖工具

- `yt-dlp` - YouTube 视频下载（download/info 步骤发现未安装时会**自动安装**到 `~/.local/bin`，无需 sudo；
  依次尝试 curl 官方二进制 → `pip install --user yt-dlp[default,curl-cffi]` → `brew install yt-dlp`，
  失败后 10 分钟内冷却不再重试；设 `YTB2BILI_NO_AUTO_INSTALL=1` 可禁用退回直接报错）
- `ffmpeg` - 音视频处理
- `deno` - JavaScript 运行时 (yt-dlp 需要；缺失时 YouTube JS challenge 无法解，部分格式缺失/被拦。macOS: `brew install deno`)
