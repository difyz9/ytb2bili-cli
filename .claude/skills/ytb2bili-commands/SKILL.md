---
name: ytb2bili-commands
description: Complete command and flag reference for the ytb2bili CLI. Use when you need the exact syntax, subcommands, or flags of any ytb command (search, submit, chain, channel, queue, task, subtitle, cookies, auto, login, download, bcut, whisper, translate, tencent-tts, server, init, whoami).
---

# ytb2bili-commands

Exact reference for every `ytb` subcommand. Use when you need a specific
command's syntax, flags, or an example. For end-to-end workflows instead, invoke
`ytb2bili-pipeline`; for failures, `ytb2bili-debug`.

## Command overview

| Command | Purpose |
|---------|---------|
| `init` | Check/fix environment dependencies |
| `login` / `whoami` | Bilibili QR login / verify account |
| `search` | Search YouTube (filters, JSON, history, direct submit) |
| `submit` | Full pipeline: download→transcribe→translate→metadata→upload→subtitle |
| `download` | Download a single YouTube video |
| `bcut` | Bcut ASR transcribe an audio/video file |
| `whisper` | whisper.cpp local transcribe (default provider) |
| `translate` | Translate an SRT subtitle file |
| `tencent-tts` | Tencent Cloud TTS synthesis from SRT |
| `chain` | Custom task chains (run/plan/list) |
| `audio-sync` | Run audio-sync on existing downloaded artifacts by videoId |
| `channel` | YouTube channel monitoring (add/list/remove/sync/videos) |
| `queue` | Job queue (add/status/work/list/remove/clear/retry-failed) |
| `task` | Task management (list/show/retry) |
| `history` | Submitted submission history (--json) |
| `debug` | Environment/login/stats diagnostics |
| `subtitle` | Subtitle upload status + upload to BVID |
| `publish` | Directly publish a local video to Bilibili |
| `review` | Check Bilibili video review status (--wait to poll) |
| `cookies` | YouTube cookies (refresh/test) |
| `auto` | Autonomous batch mode (scored search → queue/submit) |
| `server` (start/stop/restart/status/run) | HTTP API server management |

## Per-command reference

### Global flags
```
--config string   配置文件路径（默认 ./config.yaml 或 $YTB2BILI_CONFIG）
-h, --help
-v, --version
```

### 参数约定：videoId 或完整路径
以下命令的第一个参数**同时支持** videoId 或完整文件路径：
- 传 videoId → 在下载目录 `download_dir`（默认 `data/downloads`）下定位资源：
  - `whisper`/`bcut` → `<videoId>/<videoId>.mp4`
  - `translate` → `<videoId>/<videoId>.srt`
  - `tencent-tts` → `<videoId>/<videoId>.zh-Hans.srt`（回退 `<videoId>.srt`）
  - `audio-sync` → `<videoId>/` 目录（视频+字幕+voice）
- 传已存在路径 → 直接用

示例（等价）：
```
ytb tencent-tts lVIvZM8zay4
ytb tencent-tts data/downloads/lVIvZM8zay4/lVIvZM8zay4.zh-Hans.srt
```
下载根目录由 `config.yaml` 的 `download_dir` 控制（未配置则 `<data_dir>/downloads`）。

### `init`
```
--update   更新 yt-dlp 到最新版
--pip      自动安装 Python 依赖
```

### `search <query>`
```
--max int          最大结果数 (default 10)
--sort string      排序: relevance / upload_date / view_count / rating
--upload-date string  上传时间: last_hour / today / this_week / this_month / this_year
--duration string  时长: short(<4m) / medium(4-20m) / long(>20m)
--json             以 JSON 输出
--history          查看已提交历史（无需关键词）
--submit int       直接提交第 N 个结果
```

### `submit <YouTube URL>`
```
--source-lang string   源语言 (default "en")
--target-lang string   目标语言（默认读取配置）
--tid int              B站分区ID
--dry-run              仅处理不上传
--skip-translate       跳过翻译
--show-plan            只显示规划不执行
--chain string         自定义任务链 (如 download,transcribe,translate)
```

### `chain`
```
run <step1,step2,...> <URL>  运行自定义任务链 (--dry-run/--skip-translate/--tid)
plan <steps> <URL>           查看任务链规划（不执行）
list                         列出所有可用步骤
```
可用步骤: `download` `transcribe` `translate` `tts` `audio-sync` `metadata` `upload`

### `audio-sync <videoId>`
对 `data/downloads/<videoId>/` 下已有产物（视频 + 译文字幕/源字幕 + `voice/`）做音画同步，
输出 `<videoId>.synced.mp4`。用于失败后续跑。
```
--missing string         缺失配音处理: error(默认) / silence
--no-speed-adjust        不调整配音语速
```
> 幂等续跑：`submit <videoId>` 会跳过所有已有产物步骤（download/transcribe/translate/tts/audio-sync），只做 metadata+upload。

### `channel`
```
add [--title name] [--lookback N] <channel_id>
list
remove <channel_id>
sync [--lookback N] [--queue]
videos
```
`add` 支持频道(`UC...`)与播放列表(`PL...`)，自动识别类型并获取名称；添加后按
`--lookback`（默认 7 天，`0`=不限）同步并自动将新视频加入任务队列（队列与历史双重去重）。
`sync --lookback` 默认 7 天；`--queue` 自动将新视频加入处理队列。

### `queue`
```
add <URL>
status
list [--json]
remove <videoID>
clear
retry-failed
work [--once]
```

### `task`
```
list [--json]
show <task_id>
retry <task_id> [--dry-run]     # 幂等续跑（跳过已完成步骤，从失败处重试）
```

### `history`
```
history [--json]    查看已提交的投稿历史
```

> 命令别名：`transcribe`=bcut、`tts`=tencent-tts、`upload`=publish

### `subtitle`
```
status                                  # 查看字幕上传状态
upload <bvid> <subtitle.srt> [--lang]   # 上传字幕到已发布视频（默认 lang=zh）
```

### `publish <video-file>`
直接投稿本地视频到 B站（不经 YouTube 流水线）。需要先 `ytb login`。
```
--title string   标题（默认用文件名）
--desc string    简介
--tags string    标签（逗号分隔）
--tid int        B站分区ID（默认读取配置）
--cover string   封面图片路径
--source string  源站 URL
```

### `review <bvid>`
查看投稿审核状态（只读）。需要先 `ytb login`。
```
--wait   持续轮询直到审核通过（每3分钟，最长24小时）
```

### `cookies`
```
refresh   从 Chrome 刷新 YouTube cookies
test      测试 cookies 是否有效
```

### `auto <keyword...>`
```
--max-videos int     最多提交视频数 (default 3)
--dry-run            仅搜索不入队/提交
--submit             入队后直接处理
--min-views int      最低播放量过滤
--duration string    short/medium/long
--upload-date string last_hour/today/this_week/this_month/this_year
--skip-translate     跳过翻译
--scorer string      popular(默认)/fresh/balanced
```

### `download <URL or video ID>`
```
--output string   输出目录
```

### `bcut <audio/video file>`
无参数，直接传文件路径。别名 `transcribe`（Bcut ASR 云服务）。

### `whisper <audio/video file>`
使用本地 whisper.cpp（whisper-cli）听录，输出标准 SRT。
```
--model string    GGML 模型路径（默认取 config transcriber.whisper.model）
-l, --lang string 语言代码 en/zh/auto（默认 auto）
--threads int     推理线程数（默认取 config）
-o, --out string  输出目录（默认与输入同目录）
```
> 转录后端由 `config.yaml` 的 `transcriber.provider` 控制：`whisper`（默认，本地）/ `bcut`（云 ASR）。

### `metadata <videoId or path-to-srt>`
读取字幕内容，调用 LLM 生成 B站中文标题/描述/标签，保存 JSON。
```
-o, --output string  JSON 输出路径（默认 <字幕名>.meta.json）
```
示例：`ytb metadata lVIvZM8zay4` → `data/downloads/lVIvZM8zay4/lVIvZM8zay4.zh-Hans.meta.json`。
别名 `meta`。

### `translate <input.srt>`
```
--source-lang string   源语言 (default "en")
--target-lang string   目标语言（默认读取配置）
```

### `tencent-tts <input.srt>`
```
--output string      音频输出目录 (默认 <srt_dir>/voice)
--concurrency int    并发合成数 (default 3)
--voice int64        音色: 0=亲和女声 1=成熟女声 2=成熟男声 3=亲和男声
--volume float64     音量 0-15
--speed float64      语速 0-2
```
需要环境变量: `TENCENTCLOUD_SECRET_ID` / `TENCENTCLOUD_SECRET_KEY`

### `login` / `whoami`
```
login      B站扫码登录（终端打印二维码，120 秒有效）
whoami     查看当前账号（未登录会提示）
```

### `server` management
```
server start         后台守护进程启动 HTTP API
server stop          停止服务
server restart       重启
server status        查看运行状态
server run [--addr]  前台运行（内部，供后台模式调用）
```
> `start`/`restart` 会额外启动一个带远程调试的 Chrome（独立 `data/chrome-profile`，端口
> 由 `chrome_debug_port` 起始自动避让，记录于 `data/chrome.pid`+`data/chrome.port`）；
> `stop`/`restart` 会同步关闭它。该调试 Chrome 供 `cookies refresh` 连接提取 YouTube cookies，
> 首次需在该 Chrome 窗口登录 YouTube 一次。

## Data layout

```
data/
├── downloads/<videoID>/   # 下载的视频/字幕/封面
├── tasks/                 # 任务记录 (JSON, 用 task list/show 查看)
├── history/               # 提交历史 (history.json)
├── subtitles/             # 字幕上传状态 (持久化)
├── queue/                 # 作业队列
├── subscriptions/         # 频道订阅
├── monitored_videos/      # 频道发现的视频
├── cookies/               # B站登录凭据
└── server.log             # HTTP 服务日志
```
