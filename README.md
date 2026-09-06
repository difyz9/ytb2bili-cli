# ytb2bili-go

YouTube → Bilibili 视频搬运工具。Go 编写的 CLI（`ytb`），覆盖 **搜索 → 下载 → 转录 → 翻译 → 配音 → 音画同步 → 投稿 → 字幕上传** 全流水线，并提供作业队列、频道监控、自主调度守护进程（daemon）、HTTP API 等自动化能力。

- 仓库：https://github.com/zolagz/ytb2bili-go （备选镜像：https://gitee.com/difyz/ytb2bili-go ）
- 许可证：MIT

## 目录

- [功能特性](#功能特性)
- [流水线全景](#流水线全景)
- [快速开始](#快速开始)
- [环境变量](#环境变量)
- [配置文件](#配置文件)
- [CLI 命令总览](#cli-命令总览)
- [使用指南](#使用指南)
  - [搜索 YouTube](#搜索-youtube)
  - [一键搬运 submit](#一键搬运-submit)
  - [任务链与单步命令](#任务链与单步命令)
  - [自主批量搬运 auto](#自主批量搬运-auto)
  - [常驻守护进程 daemon](#常驻守护进程-daemon)
  - [作业队列与任务管理](#作业队列与任务管理)
  - [频道监控 channel](#频道监控-channel)
  - [cookies 管理](#cookies-管理)
  - [B站账号与投稿](#b站账号与投稿)
  - [字幕上传](#字幕上传)
  - [HTTP API 服务](#http-api-服务)
- [数据目录与产物](#数据目录与产物)
- [运维与排障](#运维与排障)
- [项目结构](#项目结构)
- [更多文档](#更多文档)

## 功能特性

| 功能 | 说明 |
|------|------|
| YouTube 搜索 | InnerTube API（免 key），支持排序/上传时间/时长过滤与分页 |
| 视频下载 | yt-dlp + cookies 认证，自动带下字幕与封面，可配下载代理 |
| 语音转录 | 本地 whisper.cpp（默认）/ 云端 Bcut ASR（必剪）双后端 |
| 字幕翻译 | DeepSeek 主 + 腾讯/百度/Ollama 多级降级，行级批量翻译、断点续传 |
| 中文配音 | IndexTTS2 本地服务（默认，音色克隆/情感控制）/ 腾讯云 TTS |
| 音画同步 | 清理滚动字幕、智能语速限制、顺延长句并合成中文音轨成片 |
| AI 元数据 | LLM 自动生成中文标题/简介/标签（保存 JSON） |
| B站投稿 | 多账号路由（按标题/标签关键词）、封面、自定义分区，返回 BVID |
| 字幕上传 | 投稿后异步监听审核，通过自动上传中文字幕；状态持久化 |
| 重复检测 | 提交历史去重 + 幂等续跑（产物存在即跳过对应步骤） |
| 频道监控 | RSS 订阅（免 OAuth）/ YouTube OAuth 订阅导入双路线，新视频自动入队 |
| 自主调度 | `auto` 多维评分（popular/fresh/balanced/nowcast）搜索入队 |
| 守护进程 | `daemon` 常驻循环：搜索→评分→去重→入队→串行处理，失败重试 + 超时 kill + 飞书告警 |
| 作业队列 | 文件状态机，支持批量消费、失败重排、失败审计分类 |
| 诊断自检 | `init` 环境依赖检查修复；`check` 配置与外部服务连通性自检；`debug` 全面诊断 |
| HTTP API | 内置服务（默认 127.0.0.1:8096），配套浏览器扩展（`extension/`） |

## 流水线全景

```
搜索/频道订阅发现 ──► 入队(queue) ──► [串行处理]
   download → transcribe → translate → tts → audio-sync → metadata → upload
                                                                      │
                                              （B站审核通过后，异步）  ▼
                                                          自动上传中文字幕
```

每个视频的处理产物集中在 `data/downloads/<videoId>/`：

| 产物 | 说明 |
|------|------|
| `<videoId>.mp4` | yt-dlp 下载的原始视频 |
| `<videoId>.srt` | YouTube 源语言字幕（若存在） |
| `<videoId>.zh-Hans.srt` | 翻译后的中文字幕 |
| `voice/1.mp3, 2.mp3 ...` | TTS 按字幕序号合成的分段配音 |
| `<videoId>.synced.mp4` | 音画同步后的成片（合成中文音轨） |
| `cover.jpg` | 视频封面 |

**幂等续跑**：各步骤会检查上述产物是否存在，存在即跳过。因此重跑 `ytb submit <videoId>` 不会重新下载/转录/翻译/合成配音，只从缺失步骤继续（例如 audio-sync 后即可 `submit` 收尾投稿）。

## 快速开始

### 1. 前置依赖

| 依赖 | 用途 | 说明 |
|------|------|------|
| Go ≥ 1.26 | 编译 | https://go.dev |
| yt-dlp | 视频下载 | 缺失时自动安装到 `~/.local/bin`（无需 sudo）；`ytb init --update` 可更新 |
| ffmpeg | 音视频处理 | `brew install ffmpeg` / `apt install ffmpeg` |
| deno | yt-dlp JS 运行时 | `brew install deno`（可选） |
| whisper.cpp | 本地转录（可选） | `transcriber.provider: whisper` 时必需，`ytb init` 检查；否则走 Bcut 云端 |
| Python 3 + venv | 配音/音画同步脚本 | `ytb init --venv` 一键创建 |
| IndexTTS2 服务 | 中文配音（可选） | `tts.provider: index` 时需另部署，见 `skills/audio-video-sync/SKILL.md` |

```bash
git clone https://github.com/zolagz/ytb2bili-go.git
cd ytb2bili-go

make build             # 产出 ./ytb（等价 go build -o ytb ./cmd/ytb）
make install           # 安装到 ~/.local/bin/ytb

ytb init               # 逐项检查环境依赖（yt-dlp/ffmpeg/deno/whisper/Python）
ytb init --update      # 同时把 yt-dlp 更新到最新版
ytb init --pip         # 自动安装 Python 依赖（requests）
ytb init --venv        # 自动创建 audio-video-sync .venv 并安装配音依赖

ytb check              # 配置有效性 + LLM key + TTS + YouTube 代理 连通性自检
ytb check --json       # 机器可读输出（stdout 仅 JSON）；退出码 0=全通过 / 非0=有异常
```

### 2. 配置

```bash
cp configs/config.example.yaml ./config.yaml   # 复制模板，填入真实值
```

查找顺序：`--config <path>` → `$YTB2BILI_CONFIG` → 当前目录 `./config.yaml`，找不到时用内置默认配置。模板内逐项有注释，必填项见[配置文件](#配置文件)一节。

### 3. 登录并搬运第一条视频

```bash
ytb login                                     # B站扫码登录
ytb submit "https://www.youtube.com/watch?v=VIDEO_ID"
```

## 环境变量

| 变量 | 用途 |
|------|------|
| `DEEPSEEK_API_KEY` | DeepSeek API Key（LLM 翻译/元数据），配置文件留空时必填 |
| `YTB2BILI_CONFIG` | 配置文件路径（默认 `./config.yaml`） |
| `YOUTUBE_COOKIES` | YouTube cookies 文件路径（防下载频率限制），也可用 `ytb cookies refresh` |
| `YOUTUBE_PROXY` | YouTube 下载专用代理：`socks5://user:pass@host:port` 或 `http://...`，只走 yt-dlp（不影响 B站投稿/翻译）；也可配 config 的 `youtube_proxy`（环境变量优先） |
| `LLM_MODEL` / `LLM_BASE_URL` | 覆盖默认 LLM 模型与接入点（默认 deepseek-v4-flash） |
| `TRANSLATION_PRIMARY` | 覆盖翻译主服务商（deepseek/tencent/baidu/ollama） |
| `TRANSLATION_FALLBACKS` | 覆盖降级顺序（逗号分隔，如 `tencent,ollama`） |
| `TRANSLATION_RETRIES` | 覆盖主服务重试次数 |
| `OLLAMA_BASE_URL` / `OLLAMA_MODEL` | 本地 Ollama 地址与模型（翻译兜底） |
| `TENCENTCLOUD_SECRET_ID` / `TENCENTCLOUD_SECRET_KEY` | 腾讯云密钥（TTS/翻译，或写入 config） |
| `INDEX_TTS_API_KEY` | IndexTTS2 服务鉴权 key（config `tts.index.api_key` 可引用 `${INDEX_TTS_API_KEY}`） |
| `YTB2BILI_PROJECT_DIR` | 显式指定项目根（含 `skills/`，非项目根运行 CLI 时用） |
| `YTB2BILI_PYTHON` | 指定 .venv Python 解释器 |
| `YTB2BILI_AUDIO_SYNC_SCRIPT` | 指定音画同步脚本（最细粒度覆盖） |
| `YTB2BILI_SERVER_TOKEN` | HTTP API 对外监听时要求的 Bearer Token |
| `YTB2BILI_ALLOWED_ORIGINS` | HTTP API 浏览器扩展允许的跨域来源 |

## 配置文件

`config.yaml` 含密钥，不入库。主要配置段：

| 段 / 字段 | 说明 |
|-----------|------|
| `llm_api_key` / `llm_base_url` / `llm_model` | DeepSeek（或任意 OpenAI 兼容）LLM：翻译 + 元数据 |
| `data_dir` / `download_dir` | 数据根目录 / 下载目录 |
| `skills_dir` | 技能资源目录（空 = 自动探测项目根 `skills/`；IndexTTS 脚本位置变化时可填绝对路径） |
| `min_duration_sec` | 入队时长下限（默认 240s，过滤 Short 短视频） |
| `bili_tid` | B站投稿默认分区 id |
| `translation_target_lang` | 翻译目标语言（默认 `zh-Hans`） |
| `chrome_debug_port` | cookies refresh 用 Chrome 调试起始端口 |
| `tencent_cloud` | 腾讯云 SecretId/Key/Region（TTS 与翻译降级共用） |
| `translation` | 翻译服务：`primary`（deepseek/baidu/tencent）+ `fallbacks`（降级顺序）+ `retries`；各服务商密钥/模型/批大小见模板 |
| `youtube_oauth` | Google OAuth Client ID/Secret（频道订阅导入用） |
| `tts` | 配音：`provider` = `index`（IndexTTS2 本地，默认）/ `tencent`；腾讯云音色/音量/语速参数；`index` 段含服务地址、鉴权 key、情感（`emotion`/`emotion_alpha`）、克隆音色 `ref_audio`、并发等 |
| `concurrent` | 并发请求参数（worker/限速/批大小） |
| `transcriber` | 转录后端：`provider` = `whisper`（默认，本地 whisper.cpp）/ `bcut`（云 ASR）；whisper 模型路径/线程数 |
| `accounts` | 多账号路由列表：`name` + `type_rule`（稿件标题/标签关键词）+ `is_default`（兜底账号） |
| `search` | **自主调度关键词单一来源**（auto/daemon 共用）：`keywords`、`scorer`、`upload_date`、`max_duration`、`max_videos`、`min_views` |
| `daemon` | 守护进程参数：批间隔/最大批数/每批消费上限/重试上限/步骤超时/心跳文件/飞书告警 webhook |

> **改关键词/调度 = 改 `config.yaml` 的 `search:`/`daemon:` 段后重启服务**，不要再改任何脚本。

### 多账号路由

按稿件标题/标签关键词匹配投稿账号：命中某账号 `type_rule` 中任意关键词 → 该账号投稿；未匹配 → `is_default: true` 账号兜底。

```bash
ytb login --account "主账号"     # 多账号扫码登录
ytb accounts                    # 列出已登录账号
ytb publish ... --account X     # 显式指定投稿账号
```

## CLI 命令总览

```
核心流程        auto / daemon / queue / search / submit
频道与订阅      channel / cookies
流水线步骤      download / transcribe / translate / tts / audio-sync / metadata / publish
B站管理        login / whoami / accounts / review / history / subtitle / task
系统与工具      chain / init / check / debug / server
```

任意命令加 `-h` 查看完整参数；多数命令支持 `--json` 机器可读输出（stdout 仅 JSON，日志转 stderr，退出码 0=成功 / 非0=失败）。

## 使用指南

### 搜索 YouTube

```bash
ytb search "Flutter tutorial"                    # 基本搜索
ytb search --sort view_count --duration long --max 20 "AI tutorial"
ytb search --upload-date this_week "golang"      # 本周发布
ytb search --json --max 5 "Go programming"       # JSON 输出（机器可读）
ytb search --submit 1 "Flutter tutorial"         # 直接提交第 1 个结果进流水线
ytb search --history                             # 查看已提交历史（无需关键词）
```

| 过滤器 | 取值 |
|--------|------|
| `--sort` | `relevance` / `upload_date` / `view_count` / `rating` |
| `--upload-date` | `last_hour` / `today` / `this_week` / `this_month` / `this_year` |
| `--duration` | `short`(<4m) / `medium`(4-20m) / `long`(>20m) |
| `--max` | 最大结果数（默认 10） |

### 一键搬运 submit

```bash
ytb submit "https://www.youtube.com/watch?v=VIDEO_ID"   # 完整流水线
ytb submit yn4MSHbKgmo        # 也接受 11 位 videoId；续跑已有产物（幂等）
ytb submit --dry-run <URL>    # 只处理不上传（验证流程）
ytb submit --skip-translate <URL>   # 跳过翻译
ytb submit --show-plan <URL>        # 只显示规划不执行
ytb submit --chain download,upload <URL>   # 自定义任务链
ytb submit --source-lang en --target-lang zh-Hans <URL>
ytb submit --tid 122 <URL>      # 覆盖 B站分区 id
ytb submit --json <URL>         # 机器可读输出
```

### 任务链与单步命令

任务链可以把任意步骤自由组合（`ytb chain list` 查看全部可用步骤）：

```bash
ytb chain list                        # 列出可用步骤
ytb chain plan download,upload <URL>  # 只规划不执行
ytb chain run download,transcribe,translate <URL>
ytb chain run download <URL>          # 只下载
```

每个流水线步骤也是独立 CLI 命令，支持 `--json`，可被外部 pipeline/Agent 作为工具串联：

```bash
ytb download --json "<URL>"                     # 下载（-o 指定目录）
ytb transcribe --json <videoId|file>            # 转录：--provider whisper|bcut；whisper 额外 --model/-l lang/--threads
ytb translate --json <id>.srt                   # 翻译：--source-lang/--target-lang/--test（自检全部服务商连通性）
ytb tts --json <id>.zh-Hans.srt                 # 分段配音（别名 tencent-tts）：-o 输出、--speed/--voice/--volume/--concurrency
ytb audio-sync --json <videoId>                 # 用已有产物合成中文音轨成片：--missing error|silence、--no-speed-adjust
ytb metadata --json <videoId|srt路径>           # 生成标题/简介/标签 JSON（别名 meta，默认 <字幕>.meta.json，-o 可改）
ytb publish --json video.mp4 --title "..."      # 本地视频直接投稿（别名 upload）
```

> 产物落位约定：`transcribe/translate/tts/audio-sync` 都默认在 `data/downloads/<videoId>/` 读写（按字幕序号 `1.mp3, 2.mp3 ...`），保证各单步可互相衔接。

### 自主批量搬运 auto

自动搜索多关键词 → 多维评分筛选 → 全局去重（跳过已提交历史）→ 接入队列（默认）或直接处理（`--submit`）。

```bash
ytb auto "AI tutorial" "machine learning"        # 默认 popular 评分，入队
ytb auto --scorer balanced --min-views 1000 "python"   # 均衡评分 + 播放量门槛
ytb auto --dry-run --scorer fresh "flutter"      # 只看评分结果，不写数据
ytb auto --duration long "golang backend"        # 时长档位过滤
ytb auto --max-duration 40 --max-videos 5 --submit --skip-translate "music production"
ytb auto --upload-date this_week "devops"
```

| 评分策略 | 说明 |
|----------|------|
| `popular`（默认） | 播放量 × 0.7 + 时效 × 0.3 |
| `fresh` | 时效 × 0.8 + 播放量 × 0.2 |
| `balanced` | 播放/时效/时长均衡 |
| `nowcast` | ytsubs 式频道基线对比（播放 vs 频道常态，捕捉正在起势的视频，需先 `channel baseline`/`rank` 生成基线缓存） |

常用 flag：`--dry-run`（仅搜索）、`--scorer`、`--min-views`、`--max-videos`（默认 3）、`--max-duration`、`--duration`、`--upload-date`、`--submit`（入队后立即处理）、`--skip-translate`。

### 常驻守护进程 daemon

`daemon` 取代旧 `batch_loop.sh`：搜索 → 评分 → 去重 → 入队 → 串行处理，无限循环。关键词与调度参数全部来自 `config.yaml`（`search:`/`daemon:` 段）。

```bash
ytb daemon                     # 前台无限循环（生产由 systemd 托管，勿重复手动启动）
ytb daemon --once              # 只跑一批后退出（等价 --max-batches 1）
ytb daemon --max-batches 10 --interval 30
ytb daemon --keywords "AI tutorial" --dry-run     # 只搜索评分不入队
ytb daemon status              # 查看心跳（状态/批次/当前任务/队列统计）
ytb daemon check-heartbeat     # 心跳新鲜度检查（>15min 未更新则告警并 exit 1，cron 用）
```

其余可覆盖 flag：`--scorer`、`--upload-date`、`--min-views`、`--max-videos`、`--max-duration`、`--skip-translate`、`--consume-per-batch`。

内置可靠性：

- 失败任务自动重试（`daemon.max_retries`，默认 3 次）后停止，不再无限重试堵队列；
- 步骤级超时自动 kill 重试（默认 download/transcribe/translate/audio-sync 30min、tts 60min 等，可配 `daemon.step_timeout_sec`）；
- 心跳文件 `data/daemon/heartbeat.json` 每 30s 刷新，供外部监控；
- 任务重试达上限或心跳过期 → 飞书告警（`daemon.alert_webhook`）；
- 收到 SIGTERM/SIGINT 等当前任务完成再退出（优雅重启）。

**systemd 部署**（示例，本仓库配套 `scripts/deploy.sh`）：

```ini
# ~/.config/systemd/user/ytb-batch-loop.service
[Unit]
Description=ytb2bili batch loop
[Service]
ExecStart=/home/USER/.local/bin/ytb daemon
Restart=always
[Install]
WantedBy=default.target
```

```bash
systemctl --user daemon-reload
systemctl --user enable --now ytb-batch-loop
systemctl --user restart ytb-batch-loop   # 改关键词/配置后重启
journalctl --user -u ytb-batch-loop -f    # 实时日志
```

### 作业队列与任务管理

队列是文件状态机（`data/queue/`），daemon / `queue work` 从队首串行消费。

```bash
ytb queue add "<YouTube URL>"     # 入队
ytb queue status                  # 统计：排队中/处理中/已完成/失败
ytb queue list [--json]           # 明细（含失败原因）
ytb queue remove <videoID>        # 移除单条
ytb queue clear                   # 清空队列
ytb queue retry-failed            # 将失败任务重新排队（先修复根因）
ytb queue work --once             # 手动消费一个后退出
ytb queue work                    # 持续消费（Ctrl+C 停止）
ytb queue audit [--recent N]      # 失败分类统计审计（来自 data/audit/events.jsonl）

ytb task list [--json]            # 任务列表（显示真实步骤进度 [n/7]）
ytb task show <task_id>           # 单任务详情（各步骤状态/错误定位）
ytb task retry <task_id>          # 幂等重试（从失败步骤续跑）
ytb task retry --dry-run <task_id>  # 重试但不投稿
```

### 频道监控 channel

两条发现路线：

**A. RSS 方式（免 OAuth，推荐起步）**

```bash
ytb channel add --title "频道名" <channel_id>   # 添加订阅（--lookback 7 同步近 7 天）
ytb channel list                                # 订阅列表
ytb channel remove <channel_id>                 # 移除订阅
ytb channel sync --lookback 7 --queue           # 同步更新并自动入队（--min-duration 入队时长下限）
ytb channel videos [--status new|queued|submitted|skipped] [--top N]   # 发现的视频
ytb channel rank [--keywords "AI,教程"] [--window 30] [--top N] [--prune-below X]
                                                # 仅用 RSS 数据做频道质量排名（ytsubs 式基线）
ytb channel watch --interval 24h                # 常驻定时检测更新自动入队（--once 单次）
```

**B. OAuth 方式（同步 YouTube 账号订阅列表）**

```bash
ytb channel login      # Google 设备码授权（需 Google Cloud 建 OAuth Client，见下）
ytb channel import     # 导入账号的全部订阅频道
ytb channel status     # 查看授权状态
ytb channel logout     # 清除授权
```

基线评分（nowcast / rank 的数据基础，抗爆款污染）：

```bash
ytb channel baseline <channel_id|@handle>   # 用 yt-dlp 抓最近 N 个视频算 trimmed mean 基线
ytb channel baseline --all --samples 30 --trim 3
```

> OAuth 凭证：Google Cloud Console → APIs & Services → Credentials 创建 **OAuth 2.0 客户端**（类型选 "桌面应用" 或 "TV and Limited Input devices" 支持设备码流程），启用 YouTube Data API v3，把 client_id/secret 填入 `config.yaml` 的 `youtube_oauth` 段。

### cookies 管理

```bash
ytb cookies test       # 测试 YouTube cookies 是否有效
ytb cookies refresh    # 从 Chrome 调试端口刷新 cookies（config chrome_debug_port，被占用自动 +1）
```

### B站账号与投稿

```bash
ytb login                          # B站扫码登录（打印二维码）
ytb login --account "账号名"       # 多账号登录（投稿时按类型路由）
ytb accounts                       # 列出已登录账号
ytb whoami                         # 当前账号信息

# 直接投稿本地视频（不经 YouTube 流水线）
ytb publish video.mp4 --title "我的视频" --tags "科技,评测" \
        --desc "简介" --tid 122 --cover cover.jpg --source "https://..." --account "某账号"

ytb review BV1xx123                # 查看审核状态
ytb review --wait BV1xx123         # 轮询直到审核通过（最长 24h）
ytb history [--json]               # 已提交投稿历史（重复检测依据）
```

`publish` 别名 `upload`；标题默认取文件名，`--source` 可标注源站 URL。

### 字幕上传

投稿后 pipeline 会**异步监听审核**（30 秒轮询，最长 24 小时），审核通过自动上传中文字幕；状态持久化在 `data/subtitles/`，重启不丢失。手动管理：

```bash
ytb subtitle upload BV1xx123 subtitle.zh-Hans.srt   # 手动上传（--lang zh/zh-Hans/en，默认 zh）
ytb subtitle upload BV1xx123 subtitle.srt --lang en
ytb subtitle status                                  # 所有视频的字幕上传状态
```

### HTTP API 服务

```bash
ytb server start                 # 后台启动（默认 127.0.0.1:8096；--addr 可改）
ytb server status                # 运行状态与日志位置（data/server.log）
ytb server restart / stop
```

本地回环访问无需鉴权；对外监听必须设 `YTB2BILI_SERVER_TOKEN`（Bearer 鉴权），浏览器扩展需配 `YTB2BILI_ALLOWED_ORIGINS`。扩展源码见 `extension/`（WXT/TypeScript，独立构建）。

```bash
curl -H "Authorization: Bearer $YTB2BILI_SERVER_TOKEN" \
     http://localhost:8096/api/v1/tasks
```

## 数据目录与产物

| 路径 | 内容 |
|------|------|
| `data/downloads/<videoId>/` | 每视频全部产物（见[流水线全景](#流水线全景)） |
| `data/tasks/*.json` | 任务状态（步骤进度/错误） |
| `data/queue/queue.json` | 作业队列状态 |
| `data/history/history.json` | 提交历史（去重依据） |
| `data/subtitles/*.json` | 字幕上传状态（持久化，重启不丢） |
| `data/daemon/heartbeat.json` | daemon 心跳（批次/PID/当前任务/队列统计） |
| `data/audit/events.jsonl` | 审计事件（`queue audit` 数据源） |
| `data/subscriptions/`、`data/monitored_videos/`、`data/channel_scores.json` | 频道订阅 / 发现视频 / 基线评分缓存 |
| `data/server.log` | HTTP 服务日志 |
| `data/cookies/`、`data/chrome-profile/` | YouTube cookies / Chrome 调试配置 |

## 运维与排障

```bash
ytb check               # 定位配置问题：API key、TTS 服务、代理连通性（先跑这个）
ytb debug               # 环境/登录/数据统计全面诊断
./ytb daemon status     # daemon 心跳（本机已由 systemd 托管时用）
systemctl --user restart ytb        # 改配置后重启调度（ytb = ytb-batch-loop 别名）
journalctl --user -u ytb-batch-loop -f
```

> **不要手动再起一个 `ytb daemon`**（会和 systemd 服务抢队列）；守护进程由 systemd 管理。单次手动搬运请用 `queue add` 或前台 `submit`。

失败任务达重试上限后的常见根因与处理：

| 现象 | 常见根因 | 处理 |
|------|----------|------|
| B站上传连接重置 | 临时网络问题 | 重试（`task retry <id>` / `queue retry-failed`） |
| 下载失败/被风控 | YouTube cookies 过期 / 代理被风控 | `ytb cookies refresh`；检查代理 + 完整登录 cookie |
| TTS 步骤失败 | IndexTTS2 服务未运行 | 启动服务后重试 |
| 翻译报错 | DeepSeek key 失效/限流 | `ytb check` 验证；走降级服务商 |
| `ytb check` 非 0 | 关键外部依赖不可用 | 按输出逐项修复 |

修复根因后用 `ytb task retry <task_id>`（幂等续跑）或 `ytb queue retry-failed` 重排。

## 项目结构

```
ytb2bili-go/
├── cmd/ytb/main.go         # 主入口（唯一二进制 ytb）
├── internal/
│   ├── cli/                # cobra 命令（按命令域拆分：submit/search/auto/daemon/queue/...）
│   ├── pipeline/           # 流水线编排 Processor + 各步骤实现
│   ├── workflow/           # 任务链规划/执行引擎
│   ├── queue/              # 作业队列（文件状态机）
│   ├── search/             # YouTube 搜索（InnerTube API）
│   ├── download/           # yt-dlp 封装
│   ├── transcriber/        # whisper / Bcut ASR
│   ├── translator/         # 多服务商翻译（deepseek/tencent/baidu/ollama）
│   ├── metadata/ llm/      # AI 元数据 / LLM 客户端
│   ├── tts/ audiosync/     # 腾讯云 TTS / IndexTTS2 / 音画同步
│   ├── bili/               # B站 API（上传/字幕/审核）
│   ├── channel/ ytoauth/   # 频道监控 / YouTube OAuth
│   ├── cdp/  auth/         # Chrome DevTools cookies / B站凭证
│   ├── server/ feishu/     # HTTP API / 飞书告警集成
│   ├── storage/            # 任务/凭证/历史/字幕存储
│   ├── resource/           # 运行时资源定位（skills/、.venv/）
│   └── config/             # 配置管理
├── configs/config.example.yaml   # 配置模板（真实 config.yaml 不入库）
├── skills/audio-video-sync/      # IndexTTS2 配音/音画同步技能（运行时引用）
├── extension/                    # 浏览器扩展（独立 TS 项目）
├── scripts/                      # deploy.sh / refresh_youtube_cookies.sh 等运维脚本
└── docs/                         # 设计文档与归档
```

## 更多文档

| 文档 | 内容 |
|------|------|
| `INSTALL_AGENT.md` | 面向 AI Agent 的安装/依赖/配置完整指南 |
| `AGENTS.md` | 命令约定、模块 API、Agent 工作流与运维速查 |
| `LOCAL_DEPLOY.md` | 本地部署细节 |
| `docs/API_DOCS.md` | HTTP API 端点参考 |
| `docs/FEISHU_INTEGRATION.md` | 飞书告警集成 |
| `.claude/skills/` | Claude Code 项目技能（pipeline 驱动、调试、契约等） |
| `docs/` | 设计文档与历史归档 |

## 许可证

MIT License
