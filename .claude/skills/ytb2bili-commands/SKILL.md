---
name: ytb2bili-commands
description: Complete command and flag reference for the ytb2bili CLI. Use when you need the exact syntax, subcommands, or flags of any ytb command (search, submit, chain, channel, queue, task, subtitle, cookies, auto, login, download, transcribe, translate, tts, server, init, whoami).
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
`ytb --help` 按 5 个逻辑分组展示：**核心流程 / 频道与订阅 / 流水线步骤 / B站管理 / 系统与工具**。

| 命令 | 说明 |
|------|------|
| **核心流程** | |
| `submit` | 一键搬运：download→transcribe→translate→metadata→upload→subtitle |
| `auto` | 自主批量：评分搜索 → 队列/直提（--scorer popular/fresh/balanced/nowcast） |
| `search` | 搜索 YouTube（过滤器、JSON、--submit 直提） |
| `queue` | 作业队列（add/status/work/list/remove/clear/retry-failed） |
| **频道与订阅** | |
| `channel` | 频道监控（add/list/remove/sync/videos/rank） |
| `yt-oauth` | YouTube OAuth 授权 + 订阅同步（login/sync/watch/status/logout） |
| `cookies` | YouTube cookies（refresh/test） |
| **流水线步骤** | |
| `transcribe` | 听录生成字幕（默认本地 whisper，`--provider bcut` 用云） |
| `download` | 下载单个视频 |
| `translate` | 翻译 SRT 字幕 |
| `metadata` | 根据字幕生成B站标题/描述/标签（JSON） |
| `tencent-tts` | 腾讯云 TTS 合成 |
| `audio-sync` | 音画同步（videoId/路径） |
| `bcut` / `whisper` | 旧转录命令，已由 `transcribe --provider` 取代（隐藏但可直接调用） |
| **B站管理** | |
| `login` / `whoami` | B站扫码登录 / 查看账号 |
| `publish` | 直接投稿本地视频 |
| `review` | 查看审核状态（--wait 轮询） |
| `subtitle` | 字幕上传状态 + 投稿字幕 |
| `history` | 提交历史（--json） |
| `task` | 任务管理（list/show/retry） |
| **系统与工具** | |
| `init` | 检查/修复环境依赖 |
| `server` | HTTP API 服务器（start/stop/restart/status/run） |
| `chain` | 自定义任务链（run/plan/list） |
| `debug` | 环境/登录/状态诊断 |

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
频道/订阅管理的唯一入口（含 OAuth 授权）。
```
add [--title name] [--lookback N] <channel_id>   手动添加频道/播放列表
import                                           从 YouTube 账号导入订阅（OAuth）
login / status / logout                          授权登录 / 状态 / 清除凭证
list / remove <channel_id>
sync [--lookback N] [--queue]                    同步 RSS，可入队
watch [--interval 24h] [--once]                  常驻定时检测（自动入队）
videos [--top N] [--status new]                  发现的视频（按表现分排序）
rank [--window N] [--top N] [--prune-below N] [--keywords "ai,go"]
```
`add` 支持频道(`UC...`)与播放列表(`PL...`)，自动识别类型并获取名称；添加后按
`--lookback`（默认 7 天，`0`=不限）同步并自动将新视频加入任务队列（队列与历史双重去重）。
`import` 只导入频道不入队；`sync --queue` 是唯一入队入口，`watch` 为常驻版。
`rank` 按 ytsubs 式基线评分（活跃度/基线触达/基线健康/播放稳定/内容契合，仅 RSS 无需 OAuth），
评分缓存 `data/channel_scores.json`；`--prune-below N` 移除低于 N 分的频道。
`videos` 按"播放量 vs 频道基线"的表现分排序，播放量取观测快照
（`data/observations/`，需先跑过 `channel sync` 才有真实数据）。

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

### `channel` OAuth 子命令（授权与导入）
YouTube OAuth 授权（设备码流程）与订阅导入，已并入 `channel` 家族。
```
channel login           发起设备码授权登录（浏览器打开 URL 输码）
channel status          查看授权状态
channel logout          清除凭证
channel import          从 YouTube 账号导入订阅频道（只导入，不入队）
channel watch [--interval 24h] [--once]   常驻定时检测 / 单次检测（自动入队）
```
> 前置：config.yaml `youtube_oauth.client_id/secret`（或环境变量
> `GOOGLE_CLIENT_ID`/`GOOGLE_CLIENT_SECRET`）。OAuth 客户端**必须**是
> **"TV and Limited Input devices"** 类型——Web/Desktop 类型设备码端点会拒绝
> （`invalid_client`）。token 存 `data/yt_oauth_token.json`。
> 导入后入队请用 `channel sync --queue`。

### `auto <keyword...>`
```
--max-videos int     最多提交视频数 (default 3)
--dry-run            仅搜索不入队/提交
--submit             入队后直接处理
--min-views int      最低播放量过滤
--duration string    short/medium/long
--upload-date string last_hour/today/this_week/this_month/this_year
--skip-translate     跳过翻译
--scorer string      popular(默认)/fresh/balanced/nowcast
```
> `nowcast` 为 ytsubs 式评分：播放 vs 频道基线（`data/channel_scores.json`），无基线时用候选集中位数参照；
> 先跑 `channel rank` 生成基线缓存效果最佳。

### `download <URL or video ID>`
```
--output string   输出目录
```

### `transcribe <audio/video file>`
统一转录命令：默认本地 whisper.cpp，`--provider bcut` 用云 ASR。参数支持 videoId/路径。
```
--provider string  转录后端: whisper(默认)/bcut（空=取 config transcriber.provider）
--model string     whisper GGML 模型路径（覆盖 config）
-l, --lang string  whisper 语言 en/zh/auto（默认 auto）
--threads int      whisper 推理线程数（覆盖 config）
-o, --out string   输出目录（默认与输入同目录）
```
> 旧 `bcut`/`whisper` 命令仍可直接调用（已从 `--help` 隐藏）：`ytb bcut <file>` = `ytb transcribe --provider bcut <file>`，
> `ytb whisper <file>` = `ytb transcribe --provider whisper <file>`。

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

### `tts <input.srt>`（旧名 `tencent-tts` 仍可用）
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
