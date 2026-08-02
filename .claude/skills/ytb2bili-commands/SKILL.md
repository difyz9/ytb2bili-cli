---
name: ytb2bili-commands
description: Complete command and flag reference for the ytb2bili CLI. Use when you need the exact syntax, subcommands, or flags of any ytb command (search, submit, chain, channel, queue, task, subtitle, cookies, auto, login, download, bcut, translate, tencent-tts, server, init, whoami).
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
| `translate` | Translate an SRT subtitle file |
| `tencent-tts` | Tencent Cloud TTS synthesis from SRT |
| `chain` | Custom task chains (run/plan/list) |
| `channel` | YouTube channel monitoring (add/list/remove/sync/videos) |
| `queue` | Job queue (add/status/work) |
| `task` | Task management (list/show) |
| `subtitle` | Subtitle upload status |
| `cookies` | YouTube cookies (refresh/test) |
| `auto` | Autonomous batch mode (scored search → queue/submit) |
| `start`/`stop`/`restart`/`status` | HTTP API server management |

## Per-command reference

### Global flags
```
--config string   配置文件路径（默认 ./config.yaml 或 $YTB2BILI_CONFIG）
-h, --help
-v, --version
```

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

### `channel`
```
add [--title name] <channel_id>
list
remove <channel_id>
sync [--lookback N] [--queue]
videos
```
`sync --lookback` 默认 7 天；`--queue` 自动将新视频加入处理队列。

### `queue`
```
add <URL>
status
work [--once]
```

### `task`
```
list
show <task_id>
```

### `subtitle`
```
status
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
无参数，直接传文件路径。

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
start      后台守护进程启动 HTTP API
stop       停止服务
restart    重启
status     查看运行状态
```

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
