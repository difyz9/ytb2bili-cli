# 运维与排障

## 诊断命令

```bash
ytb check               # 定位配置问题：API key、TTS 服务、代理连通性（先跑这个）
ytb check --json        # 机器可读输出；退出码 0=全通过 / 非0=有异常
ytb debug               # 环境/登录/数据统计全面诊断
ytb init                # 环境依赖逐项检查（yt-dlp/ffmpeg/deno/whisper/Python）
./ytb daemon status     # daemon 心跳（本机已由 systemd 托管时用）
./ytb daemon check-heartbeat   # 心跳新鲜度检查（cron 用，>15min 未更新告警，exit 1）
```

## 服务管理

```bash
systemctl --user restart ytb        # 改配置后重启调度（ytb = ytb-batch-loop.service 别名）
systemctl --user status ytb         # 服务状态
journalctl --user -u ytb-batch-loop -f   # 实时日志
```

> **不要手动再起一个 `ytb daemon`**（会和 systemd 服务抢队列）；守护进程由 systemd 管理。单次手动搬运请用 `queue add` 或前台 `submit`。

## 失败任务常见根因

失败任务达重试上限（默认 3 次）后需要人工判断根因，修复后再 `task retry <id>` 或 `queue retry-failed`：

| 现象 | 常见根因 | 处理 |
|------|----------|------|
| B站上传连接重置 | 临时网络问题 | 重试（`task retry <id>` / `queue retry-failed`） |
| 下载失败/被风控 | YouTube cookies 过期 / 代理被风控 | `ytb cookies refresh`；检查代理 + 完整登录 cookie |
| TTS 步骤失败 | IndexTTS2 服务未运行 | 启动服务后重试 |
| 翻译报错 | DeepSeek key 失效/限流 | `ytb check` 验证；走降级服务商 |
| `ytb check` 非 0 | 关键外部依赖不可用 | 按输出逐项修复 |

## 调试技巧

1. 使用 `submit --dry-run` 测试完整流程但不上传；
2. 检查 `data/downloads/` 查看下载文件；
3. 运行 `task list` / `task show <id>` 查看任务状态和失败步骤；
4. 查看 `data/history/` 的提交历史；
5. 查看 `data/subtitles/` 的字幕上传状态（持久化，重启不丢失）；
6. 检查登录态：`ytb whoami`；检查 cookies：`ytb cookies test`；
7. 日志输出到 stderr，可重定向查看；HTTP 服务日志在 `data/server.log`。

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
│   ├── cdp/ auth/          # Chrome DevTools cookies / B站凭证
│   ├── server/ feishu/     # HTTP API / 飞书告警集成
│   ├── storage/            # 任务/凭证/历史/字幕存储
│   ├── resource/           # 运行时资源定位（skills/、.venv/）
│   └── config/             # 配置管理
├── configs/config.example.yaml   # 配置模板（真实 config.yaml 不入库）
├── skills/audio-video-sync/      # IndexTTS2 配音/音画同步技能（运行时引用）
├── extension/                    # 浏览器扩展（独立 TS 项目）
├── scripts/                      # deploy.sh / refresh_youtube_cookies.sh 等运维脚本
└── docs/                         # 设计文档与归档（本网站构建于 website/）
```

## 测试与调试

```bash
go test ./...                  # 运行单元测试
go test ./internal/cli/ -v     # 只测试某个包
ytb search --max 3 "test query"   # 冒烟测试搜索
```
