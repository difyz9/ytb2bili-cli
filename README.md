# ytb2bili-go

YouTube → Bilibili 视频搬运工具。Go 编写的 CLI（`ytb`），覆盖 搜索 → 下载 → 转录 → 翻译 → 配音 → 音画同步 → 投稿 → 字幕上传 全流水线，支持守护进程自动化运行。

- 仓库：https://github.com/zolagz/ytb2bili-go （备选镜像：https://gitee.com/difyz/ytb2bili-go ）

## 功能特性

| 功能 | 说明 |
|------|------|
| YouTube 搜索 | InnerTube API，支持排序/时间/时长过滤，`--submit N` 直接提交 |
| 视频下载 | yt-dlp + cookies 认证，自动带下字幕与封面 |
| 语音转录 | 本地 whisper.cpp（默认）/ 云 Bcut ASR 双后端 |
| 字幕翻译 | DeepSeek 主 + 腾讯/百度/Ollama 多级降级，滚动字幕去重、行级翻译记忆 |
| 中文配音 | IndexTTS2 本地服务（音色克隆、情感控制）/ 腾讯云 TTS |
| 音画同步 | 清理滚动字幕、智能语速限制、顺延长句并合成中文音轨 |
| AI 元数据 | LLM 自动生成中文标题/简介/标签 |
| B站投稿 | 多账号路由（按标题/标签关键词）、封面、返回 BVID |
| 字幕上传 | 异步监听审核，通过后自动上传中文字幕 |
| 重复检测 | 提交历史自动去重，幂等续跑 |
| 频道监控 | RSS 订阅 + OAuth 账号订阅导入，新视频自动入队 |
| 自主调度 | `daemon` 守护进程：搜索→评分→去重→入队→串行处理，失败重试+飞书告警 |
| 作业队列 | 文件状态机，支持批量消费、失败重排 |
| HTTP API | 内置服务（默认 127.0.0.1:8096），配套浏览器扩展（`extension/`） |

## 流水线全景

```
搜索/订阅发现 → 入队 → [串行处理] download → transcribe → translate → tts → audio-sync → metadata → upload
                                                                              ↓（异步）
                                                            审核通过后自动上传中文字幕
```

所有步骤产物落在 `data/downloads/<videoId>/`，重跑 `submit <videoId>` 自动跳过已完成步骤（幂等续跑）。

## 快速开始

### 1. 前置依赖

| 依赖 | 用途 | 安装 |
|------|------|------|
| Go >= 1.26 | 编译 | https://go.dev |
| yt-dlp | 视频下载 | `ytb init --update` 可自动更新 |
| ffmpeg | 音视频处理 | `brew install ffmpeg` / `apt install ffmpeg` |
| deno | yt-dlp JS 运行时 | `brew install deno` |
| whisper-cli | 本地转录（可选，否则走 Bcut 云端） | [whisper.cpp](https://github.com/ggml-org/whisper.cpp) |
| Python 3 + venv | 配音/音画同步脚本 | `ytb init --venv` 一键创建 |

```bash
git clone https://github.com/zolagz/ytb2bili-go.git
cd ytb2bili-go
make build          # 产出 ./ytb（等价 go build -o ytb ./cmd/ytb）
make install        # 安装到 ~/.local/bin/ytb

ytb init            # 逐项检查环境依赖
ytb init --venv     # 同时创建配音 Python 环境

# 配置与外部服务自检：配置有效性 + LLM/DeepSeek key + 语音合成 + YouTube 代理
ytb check           # 退出码 0=全通过 / 非0=有异常；--json 机器可读输出
```

### 2. 配置

```bash
cp configs/config.example.yaml ./config.yaml   # 复制模板，填入真实值
```

> ⚠️ `config.yaml` 与各类凭证（cookies.txt、OAuth client JSON）均不入库（见 .gitignore）。查找顺序：`--config` → `$YTB2BILI_CONFIG` → `./config.yaml`。

必填项（模板内有逐项注释）：

- `llm_api_key`：DeepSeek API Key（或环境变量 `DEEPSEEK_API_KEY`）
- `tencent_cloud`：腾讯云 SecretId/Key（TTS 与翻译降级共用）
- `youtube_oauth`：Google OAuth Client ID/Secret（订阅导入用，见下文）
- `search` / `daemon`：自动调度的关键词与参数（单一来源，改这里即可）

### 3. 登录并投稿

```bash
ytb login                          # B站扫码登录
ytb submit "https://www.youtube.com/watch?v=VIDEO_ID"
ytb submit VIDEO_ID                # 也支持 11 位 ID 续跑已有产物
```

## 核心用法

### submit —— 一键搬运

```bash
ytb submit <URL>                     # 完整流水线：下载→转录→翻译→配音→同步→投稿
ytb submit --dry-run <URL>           # 只处理不上传（验证流程）
ytb submit --skip-translate <URL>    # 跳过翻译
ytb submit --show-plan <URL>         # 只显示任务规划
ytb submit --chain download <URL>    # 自定义任务链（只下载）
ytb submit --json <URL>              # 机器可读输出（stdout 仅 JSON）
```

幂等续跑：各步骤检查 `data/downloads/<videoId>/` 已有产物（视频/`.srt`/`.zh-Hans.srt`/`voice/`/`.synced.mp4`），存在即跳过。

### chain —— 可组合任务链

```bash
ytb chain list                      # 查看可用步骤（download/transcribe/translate/tts/audio-sync/metadata/upload）
ytb chain run download,transcribe <URL>     # 执行指定步骤
ytb chain plan download,upload <URL>        # 只规划不执行
```

### 单步命令（独立可组合）

每个流水线步骤都是独立 CLI 命令，支持 `--json`，可被外部 pipeline/Agent 串联：

```bash
ytb download  --json "<URL>"      # {"ok":true,"step":"download",...}
ytb transcribe --json <videoId>   # 转录（--provider whisper|bcut 可覆盖配置）
ytb translate  --json <id>.srt    # 翻译（--test 自检全部翻译服务商连通性）
ytb tts        --json <id>.zh-Hans.srt
ytb audio-sync --json <videoId>   # 对已有产物合成中文音轨视频
ytb metadata   --json <videoId>   # 生成标题/简介/标签 JSON
ytb publish    --json video.mp4 --title "..." --tags "..."   # 本地视频直接投稿
```

## 自主模式（批量搬运）

### auto —— 搜索评分入队

```bash
ytb auto "AI tutorial" "ChatGPT"            # 多关键词搜索→评分→去重→入队
ytb auto --submit --max-videos 5 "golang"   # 入队后立即逐个处理
ytb auto --dry-run --scorer nowcast "AI"    # 只看评分结果不写数据
ytb auto --duration long --min-views 1000 "machine learning"
```

| 评分策略 | 说明 |
|----------|------|
| `popular`（默认） | 播放量 × 0.7 + 时效 × 0.3 |
| `fresh` | 时效 × 0.8 + 播放量 × 0.2 |
| `balanced` | 播放/时效/时长均衡 |
| `nowcast` | ytsubs 式频道基线对比（抗爆款污染，配合 `channel baseline`） |

### daemon —— 常驻守护进程

搜索→评分→去重→入队→串行处理，无限循环。**关键词/调度参数单一来源是 `config.yaml` 的 `search:` 与 `daemon:` 段**，改配置后 `systemctl --user restart ytb-batch-loop`。

```bash
ytb daemon                  # 前台运行（生产环境由 systemd 管理，勿手动重复启动）
ytb daemon --once           # 只跑一批
ytb daemon status           # 查看心跳（状态/批次/当前任务/队列统计）
ytb daemon check-heartbeat  # 心跳新鲜度检查（>15min 未更新 exit 1，供 cron 告警）
```

内置可靠性：失败自动重试（默认 3 次后停止并飞书告警 `daemon.alert_webhook`）、步骤超时自动 kill（下载 30min/TTS 60min 等）、心跳文件 `data/daemon/heartbeat.json` 每 30s 刷新。

### queue / task —— 队列与任务

```bash
ytb queue status             # 排队中/处理中/已完成/失败 统计
ytb queue list               # 明细（含失败原因）
ytb queue add <URL> && ytb queue work --once    # 手动入队并消费一个
ytb queue retry-failed       # 修复根因后重排失败任务

ytb task list [--json]       # 任务真实步骤进度 [n/7]
ytb task show <task_id>      # 各步骤状态/错误定位
ytb task retry <task_id>     # 幂等重试（从失败步骤续跑）
```

## 频道监控与订阅

```bash
# RSS 方式（无需 OAuth）
ytb channel add --title "频道名" <channel_id>
ytb channel sync --lookback 7 --queue     # 同步近 7 天新视频并入队
ytb channel videos                        # 按频道基线表现评分排序
ytb channel rank                          # 频道质量排名（RSS 数据即可）

# OAuth 方式（拉取 YouTube 账号订阅列表）
ytb channel login                         # Google 设备码授权
ytb channel import                        # 导入订阅频道
ytb channel watch --interval 24h          # 常驻定时检测更新并自动入队
ytb channel status                        # 查看授权状态
```

> OAuth 凭证需在 Google Cloud Console 创建 **"TV and Limited Input devices"** 类型的 OAuth Client，启用 YouTube Data API v3 后填入 `config.yaml` 的 `youtube_oauth` 段。

## B站账号与投稿管理

```bash
ytb login                          # 扫码登录
ytb login --account "账号名"       # 多账号登录
ytb accounts                       # 列出已登录账号
ytb whoami                         # 当前账号

ytb publish video.mp4 --title "标题" --tags "科技,评测" --desc "简介" \
                    --tid 122 --cover cover.jpg --account "某账号"
ytb review BV1xx123                # 查看审核状态
ytb review --wait BV1xx123         # 轮询直到通过（最长 24h）
ytb history                        # 投稿历史
```

多账号路由：`config.yaml` 的 `accounts` 段按稿件标题/标签关键词（`type_rule`）匹配投稿账号，未匹配用 `is_default` 账号兜底。

## 字幕上传

投稿后异步监听审核（30 秒轮询，最长 24 小时），通过后自动上传中文字幕；状态持久化在 `data/subtitles/`，重启不丢失。

```bash
ytb subtitle status                                  # 所有视频的字幕上传状态
ytb subtitle upload BV1xx123 subtitle.zh-Hans.srt    # 手动上传（--lang en 可指定语言）
```

## HTTP API 服务

```bash
ytb server start                  # 后台启动（默认 127.0.0.1:8096）
ytb server status                 # 运行状态与日志位置（data/server.log）
ytb server stop
```

对外监听必须配 Token（`YTB2BILI_SERVER_TOKEN`）；浏览器扩展需配置 `YTB2BILI_ALLOWED_ORIGINS`。扩展源码见 `extension/`（WXT/TypeScript 项目，独立构建）。

```bash
curl -H "Authorization: Bearer $YTB2BILI_SERVER_TOKEN" \
     http://localhost:8096/api/v1/tasks
```

## 配置与运行时资产

| 项 | 说明 |
|----|------|
| `skills_dir` | 技能资源目录（空 = 自动探测项目根下 `skills/`，可指定绝对路径） |
| `YTB2BILI_PROJECT_DIR` | 显式指定项目根（非项目根目录运行 CLI 时用） |
| `YTB2BILI_PYTHON` / `YTB2BILI_AUDIO_SYNC_SCRIPT` | 细粒度覆盖解释器/音画同步脚本 |
| `TRANSLATION_PRIMARY` / `TRANSLATION_FALLBACKS` | 覆盖翻译主/降级服务商 |

转录/配音后端均由配置切换：`transcriber.provider`（whisper/bcut）、`tts.provider`（index=本地 IndexTTS2 / tencent）。IndexTTS2 服务需另行部署（默认 `http://localhost:18765`，支持音色克隆与情感参数，详见 `skills/audio-video-sync/SKILL.md`）。

## 项目结构

```
ytb2bili-go/
├── cmd/ytb/main.go         # 主入口（唯一二进制）
├── internal/
│   ├── cli/                # cobra 命令（按命令域拆分：submit/search/auto/daemon/queue/...）
│   ├── pipeline/           # 流水线编排 Processor + 各步骤实现
│   ├── workflow/           # 任务链规划/执行引擎
│   ├── queue/              # 作业队列（文件状态机）
│   ├── search/             # YouTube 搜索（InnerTube API）
│   ├── download/           # yt-dlp 封装
│   ├── transcriber/        # whisper / Bcut ASR
│   ├── translator/         # 多服务商翻译（deepseek/tencent/baidu/ollama）
│   ├── tts/  audiosync/    # 腾讯云 TTS / 音画同步
│   ├── metadata/  llm/     # AI 元数据 / LLM 客户端
│   ├── bili/               # B站 API（上传/字幕/审核）
│   ├── channel/  ytoauth/  # 频道监控 / YouTube OAuth
│   ├── auth/  cdp/         # B站凭证 / Chrome DevTools cookies
│   ├── server/  feishu/    # HTTP API / 飞书集成
│   ├── storage/            # 任务/凭证/历史/字幕存储
│   ├── resource/           # 运行时资源定位（skills/、.venv/）
│   └── config/             # 配置管理
├── configs/config.example.yaml   # 配置模板（真实 config.yaml 不入库）
├── skills/audio-video-sync/      # IndexTTS2 配音/音画同步技能（运行时引用）
├── extension/                    # 浏览器扩展（独立 TS 项目）
├── scripts/                      # 部署与运维脚本
└── docs/                         # 详细文档（含历史归档）
```

## 运维速查

```bash
ytb check                         # 配置有效性 + LLM key + 语音合成 + YouTube 代理连通性自检
ytb debug                         # 环境/登录/数据统计全面诊断
ytb cookies test                  # YouTube cookies 有效性
ytb cookies refresh               # 从 Chrome 刷新 cookies
systemctl --user restart ytb-batch-loop        # 改关键词/配置后重启调度
journalctl --user -u ytb-batch-loop -f         # 实时日志
```

失败任务达重试上限后常见根因：B站上传连接重置（临时网络）、YouTube cookies 过期（`ytb cookies refresh`）、IndexTTS2 服务未运行。修复后 `ytb queue retry-failed`。

## 更多文档

| 文档 | 内容 |
|------|------|
| `INSTALL_AGENT.md` | 面向 Agent 的安装/依赖/配置完整指南 |
| `AGENTS.md` | 项目结构、命令约定、模块 API、Agent 工作流 |
| `LOCAL_DEPLOY.md` | 本地部署细节 |
| `.claude/skills/` | Claude Code 项目技能（pipeline 驱动、调试、契约等） |
| `docs/` | 设计文档与历史分析归档 |

## 许可证

MIT License
