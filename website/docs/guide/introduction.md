# 项目简介

**ytb2bili-go** 是一个 YouTube → Bilibili 视频搬运工具，使用 Go 编写。CLI 构建产物为 **`ytb`**（cobra 框架），覆盖**搜索 → 下载 → 转录 → 翻译 → 配音 → 音画同步 → 投稿 → 字幕上传**完整流水线，并提供作业队列、频道监控、自主调度守护进程（daemon）、HTTP API 等自动化能力。

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

## 运行架构

调度已收敛到 `ytb daemon`（替代旧 `batch_loop.sh` bash 循环），由 systemd 用户服务托管：

```text
systemd 用户服务
├─ ytb-batch-loop.service → ytb daemon   # 主调度（Go 内置循环，别名 ytb.service）
└─ index-tts.service                     # IndexTTS2 TTS 服务（配音后端）
```

- 关键词/搜索参数单一来源 = `config.yaml` 的 `search:` 段；调度参数在 `daemon:` 段。
- 失败任务自动重试（默认 3 次）后停止并飞书告警；步骤超时（下载 30min / TTS 60min）自动 kill 重试。
- 心跳文件 `data/daemon/heartbeat.json`（批次/PID/当前任务/队列统计/状态）每 30s 刷新。

## 更多文档

仓库内还有面向不同场景的补充文档（站点外文件）：

| 文档 | 内容 |
|------|------|
| `README.md` | 用户文档（本文档站点内容的主要来源） |
| `INSTALL_AGENT.md` | 面向 AI Agent 的逐步骤安装指南 |
| `AGENTS.md` | 命令约定、模块 API、Agent 工作流与运维速查 |
| `LOCAL_DEPLOY.md` | 本地部署细节 |
| `docs/API_DOCS.md` | HTTP API 端点参考 |
| `docs/FEISHU_INTEGRATION.md` | 飞书告警集成 |
