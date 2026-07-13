# INSTALL_AGENT.md - AI 智能体安装指南

> 面向 AI Agent（Codex、Claude Code、Hermes Agent、Cursor 等）的逐步骤安装文档。
> 按顺序执行每一步，不要跳跃。安装完成后可参考 `AGENTS.md` 使用项目。

---

## 步骤 0：检测系统环境

```bash
# === 执行以下命令，记录输出 ===
echo "OS=$(uname -s)"
echo "ARCH=$(uname -m)"
echo "DISTRO=$(cat /etc/os-release 2>/dev/null | grep -oP 'PRETTY_NAME="\K[^"]+' || uname -s)"
echo "GO=$(go version 2>/dev/null || echo 'not found')"
echo "YTDLP=$(yt-dlp --version 2>/dev/null || echo 'not found')"
echo "FFMPEG=$(ffmpeg -version 2>/dev/null | head -1 || echo 'not found')"
echo "DENO=$(deno --version 2>/dev/null | head -1 || echo 'not found')"
echo "NPM=$(npm --version 2>/dev/null || echo 'not found')"
```

## 步骤 1：安装依赖

### 1.1 安装 Go

> 要求: Go >= 1.21

根据检测到的系统选择执行：

```bash
# ── Linux ──
# Debian/Ubuntu
sudo apt install -y golang

# RHEL/Fedora
sudo dnf install -y golang

# Arch
sudo pacman -S --noconfirm go

# ── macOS ──
brew install go

# ── Windows ──
winget install GoLang.Go
# 或: choco install golang
# 或: scoop install go

# ── 验证 ──
go version  # 必须 >= go1.21
```

### 1.2 安装 yt-dlp

```bash
# ── Linux ──
# Debian/Ubuntu
sudo apt install -y yt-dlp

# RHEL/Fedora
sudo dnf install -y yt-dlp

# Arch
sudo pacman -S --noconfirm yt-dlp

# ── macOS ──
brew install yt-dlp

# ── Windows ──
winget install yt-dlp.yt-dlp
# 或: choco install yt-dlp
# 或: scoop install yt-dlp

# ── 备用方案（所有平台）──
pip3 install yt-dlp

# ── 验证 ──
yt-dlp --version  # 任意版本均可
```

### 1.3 安装 ffmpeg

```bash
# ── Linux ──
# Debian/Ubuntu
sudo apt install -y ffmpeg

# RHEL/Fedora
sudo dnf install -y ffmpeg

# Arch
sudo pacman -S --noconfirm ffmpeg

# ── macOS ──
brew install ffmpeg

# ── Windows ──
winget install FFmpeg
# 或: choco install ffmpeg
# 或: scoop install ffmpeg

# ── 验证 ──
ffmpeg -version  # 任意版本均可
```

### 1.4 安装 Deno

> **注意**：Debian/Ubuntu 的 apt 源没有 deno 包，需要使用官方安装脚本。

```bash
# ── Linux (推荐: 官方安装脚本) ──
curl -fsSL https://deno.land/install.sh | sh

# 然后添加到 PATH
export PATH="$HOME/.deno/bin:$PATH"
# 永久添加（写入到 shell 配置）
echo 'export PATH="$HOME/.deno/bin:$PATH"' >> ~/.bashrc
# 如果使用 zsh:
echo 'export PATH="$HOME/.deno/bin:$PATH"' >> ~/.zshrc

# ── Linux (备选: 通过 npm) ──
npm install -g deno

# ── Linux (Arch) ──
sudo pacman -S --noconfirm deno

# ── macOS ──
brew install deno

# ── Windows ──
winget install DenoLand.Deno
# 或: choco install deno
# 或: scoop install deno

# ── 验证 ──
deno --version  # 应该显示 deno x.y.z
```

### 1.5 检查所有依赖是否就绪

```bash
echo "Go: $(go version 2>/dev/null | grep -oP 'go\S+' || echo '❌')"
echo "yt-dlp: $(yt-dlp --version 2>/dev/null || echo '❌')"
echo "ffmpeg: $(ffmpeg -version 2>/dev/null | head -1 || echo '❌')"
echo "Deno: $(deno --version 2>/dev/null | head -1 || echo '❌')"
```

✅ **所有 4 个依赖都必须就绪才能继续。**

---

## 步骤 2：克隆项目

```bash
git clone https://github.com/zolagz/ytb2bili-go.git
cd ytb2bili-go
```

### 备选仓库

如果 GitHub 不可用，尝试 Gitee：

```bash
git clone https://gitee.com/difyz/ytb2bili-go.git
cd ytb2bili-go
```

---

## 步骤 3：编译项目

```bash
go build -o ytb2bili .

# 验证编译成功
ls -lh ytb2bili
# 期望输出: -rwxr-xr-x ... ytb2bili (~35MB)
```

### 编译失败排查

| 错误 | 解决方案 |
|------|----------|
| `go: not found` | Go 未安装或不在 PATH，回到步骤 1.1 |
| `missing go.sum` | 执行 `go mod tidy` |
| `dial tcp: timeout` | Go 模块下载被墙，设置 GOPROXY：`go env -w GOPROXY=https://goproxy.cn,direct` |

---

## 步骤 4：创建数据目录

```bash
mkdir -p data/downloads
mkdir -p data/history
mkdir -p data/cookies

# 验证
ls -la data/
# 期望看到: downloads/  history/  cookies/
```

---

## 步骤 5：配置环境变量

### 5.1 必需变量

```bash
# DeepSeek API Key - 用于字幕翻译和元数据生成
export DEEPSEEK_API_KEY="sk-your-key-here"
echo 'export DEEPSEEK_API_KEY="sk-your-key-here"' >> ~/.bashrc
```

### 5.2 可选变量

```bash
# YouTube cookies 文件路径（防止下载频率限制）
export YOUTUBE_COOKIES="/path/to/your/youtube_cookies.txt"

# 自定义 LLM 模型（默认 deepseek-v4-flash）
export LLM_MODEL="deepseek-chat"

# 自定义 LLM API 地址（默认 https://api.deepseek.com）
export LLM_BASE_URL="https://api.deepseek.com"
```

### 5.3 飞书多维表格（可选，配合 Chrome 扩展使用）

```bash
export FEISHU_APP_ID="cli_xxx"
export FEISHU_APP_SECRET="xxx"
export BITABLE_APP_TOKEN="xxx"
export BITABLE_TABLE_ID="xxx"
```

### 5.4 验证环境变量已生效

```bash
echo "DEEPSEEK_API_KEY=${DEEPSEEK_API_KEY:0:8}..."  # 只显示前8位
```

---

## 步骤 6：验证安装

### 6.1 查看帮助

```bash
./ytb2bili --help
```

**期望输出（8 个命令）：**

```
COMMANDS:
   login    B站扫码登录
   search   搜索 YouTube 视频
   submit   提交搬运任务
   task     任务管理
   channel  YouTube 频道监控管理
   server   启动 HTTP API 服务器
   debug    调试模式
   bitable  从飞书多维表格读取任务
```

### 6.2 测试搜索

```bash
./ytb2bili search --max 3 "Go programming tutorial"
```

**期望输出：** 返回 3 个 YouTube 搜索结果，包含标题、频道、时长、播放量。

### 6.3 测试 JSON 输出

```bash
./ytb2bili search --max 2 --json "AI tutorial"
```

**期望输出：** 合法的 JSON 对象，包含 `query`、`videos` 等字段。

---

## 步骤 7：B站登录（首次使用）

```bash
# 执行登录，会生成二维码
./ytb2bili login

# 用 B站 APP 扫描终端中显示的二维码
# 等待提示 "登录成功"
```

### 登录说明

| 场景 | 说明 |
|------|------|
| **首次登录** | 二维码有效期为 120 秒，扫码后自动保存 token |
| **重新登录** | 删除 `data/cookies/` 下的文件后重新执行 |
| **登录状态检查** | 直接执行 `./ytb2bili submit <url>`，如已登录会直接上传 |
| **登录失败** | 检查系统时间是否正确，二维码可能与时间戳绑定 |

---

## 常见问题（Agent 排查指南）

### Q1: deno 安装后 `deno` 命令找不到

```bash
# 原因：官方 install.sh 安装到 ~/.deno/bin/，不在 PATH 中
export PATH="$HOME/.deno/bin:$PATH"
# 然后永久添加
echo 'export PATH="$HOME/.deno/bin:$PATH"' >> ~/.bashrc
source ~/.bashrc
```

### Q2: Go 模块下载失败（网络超时）

```bash
# 设置 Go 代理（中国大陆必做）
go env -w GOPROXY=https://goproxy.cn,direct
go env -w GOSUMDB=sum.golang.org
```

### Q3: `./ytb2bili search` 返回 `No help topic for 'search'`

```bash
# 原因是二进制文件是旧版本，需要重新编译
# 检查编译时间与代码修改时间
ls -l ytb2bili main.go
stat -c '%Y' ytb2bili 2>/dev/null  # 编译时间戳
stat -c '%Y' main.go 2>/dev/null   # 代码修改时间戳

# 如果代码更新，重新编译
go build -o ytb2bili .
```

### Q4: YouTube 搜索返回空结果

```bash
# 检查网络连通性
curl -s -o /dev/null -w "%{http_code}" --connect-timeout 5 "https://www.youtube.com"
# 如果返回 000：YouTube 被墙，需要代理
# 设置 HTTP_PROXY 后再试：
export HTTP_PROXY="http://proxy:port"
export HTTPS_PROXY="http://proxy:port"
./ytb2bili search --max 3 "test"
```

### Q5: 编译后二进制比预期小很多

```bash
# 正常的 ytb2bili 约 30-35MB
# 如果只有几 MB，可能是 go build 有缓存问题
go clean -cache
go build -o ytb2bili .
```

---

## 完整安装清单

| 步骤 | 检查项 | 验证命令 |
|------|--------|----------|
| 1.1 | Go >= 1.21 | `go version` |
| 1.2 | yt-dlp 已安装 | `yt-dlp --version` |
| 1.3 | ffmpeg 已安装 | `ffmpeg -version` |
| 1.4 | Deno 已安装 + 在 PATH | `deno --version` |
| 2 | 项目已克隆 | `ls ytb2bili-go/main.go` |
| 3 | 编译成功 | `ls -lh ytb2bili` (~35MB) |
| 4 | 数据目录已创建 | `ls data/downloads/` |
| 5 | DEEPSEEK_API_KEY 已设置 | `echo $DEEPSEEK_API_KEY` |
| 6.1 | 帮助正常 | `./ytb2bili --help` (8 cmds) |
| 6.2 | 搜索正常 | `./ytb2bili search --max 1 "test"` |
| 7 | B站已登录 | `./ytb2bili login` |

---

## 一键安装（Agent 简化版）

如果所有依赖已确认就绪，可以直接执行：

```bash
git clone https://github.com/zolagz/ytb2bili-go.git
cd ytb2bili-go
bash deploy.sh
```

`deploy.sh` 脚本会自动完成：依赖检查 → 编译 → 创建目录 → 提示环境变量配置。

---

*本文档供 AI Agent 使用，详见 [AGENTS.md](./AGENTS.md) 获取使用说明。*
