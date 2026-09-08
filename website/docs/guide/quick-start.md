# 快速开始

## 1. 前置依赖

| 依赖 | 用途 | 说明 |
|------|------|------|
| Go ≥ 1.26 | 编译 | <https://go.dev> |
| yt-dlp | 视频下载 | 缺失时自动安装到 `~/.local/bin`（无需 sudo）；`ytb init --update` 可更新 |
| ffmpeg | 音视频处理 | `brew install ffmpeg` / `apt install ffmpeg` |
| deno | yt-dlp JS 运行时 | `brew install deno`（可选） |
| whisper.cpp | 本地转录（可选） | `transcriber.provider: whisper` 时必需，`ytb init` 检查；否则走 Bcut 云端 |
| Python 3 + venv | 配音/音画同步脚本 | `ytb init --venv` 一键创建 |
| IndexTTS2 服务 | 中文配音（可选） | `tts.provider: index` 时需另部署，见 `skills/audio-video-sync/SKILL.md` |

## 2. 编译与检查

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

环境依赖是否安装 → 用 `ytb init`；定位配置/外部服务问题 → 优先跑 `ytb check`。

## 3. 配置

```bash
cp configs/config.example.yaml ./config.yaml   # 复制模板，填入真实值
```

配置查找顺序：`--config <path>` → `$YTB2BILI_CONFIG` → 当前目录 `./config.yaml`，找不到时使用内置默认配置。模板内逐项有注释；必填的环境变量与配置段见[环境变量与配置](/guide/configuration)。

```bash
# 必需的环境变量
export DEEPSEEK_API_KEY=***

# 可选：YouTube cookies（防止下载频率限制）
export YOUTUBE_COOKIES="/path/to/youtube_cookies.txt"

# 可选：YouTube 下载专用代理（仅走 yt-dlp，不影响 B站投稿/翻译）
export YOUTUBE_PROXY="socks5://user:pass@host:port"
```

## 4. 登录并搬运第一条视频

```bash
ytb login             # B站扫码登录（终端打印二维码）
ytb whoami            # 确认登录账号

# 一键搬运：下载 → 转录 → 翻译 → 元数据 → 上传 → 字幕（审核通过后自动）
ytb submit "https://www.youtube.com/watch?v=VIDEO_ID"
# 也支持直接用 11 位 videoId；已有产物时幂等续跑
ytb submit VIDEO_ID
```

YouTube cookies 建议一并配置：`ytb cookies test` 验证有效，`ytb cookies refresh` 从 Chrome 刷新（自动选 `data/cookies/` 下最新有效文件）。
