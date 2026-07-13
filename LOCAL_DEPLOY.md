# ytb2bili-go 本地部署指南

## 前置条件

1. **Go 1.21+** 已安装
2. **yt-dlp** 已安装
3. **ffmpeg** 已安装
4. **deno** 已安装（用于 yt-dlp 解决 YouTube JS 挑战）

### 安装依赖

```bash
# 安装 Go (Mac)
brew install go

# 安装 yt-dlp (Mac)
brew install yt-dlp

# 安装 ffmpeg (Mac)
brew install ffmpeg

# 安装 deno (Mac)
brew install deno

# 验证安装
go version
yt-dlp --version
ffmpeg -version
deno --version
```

## 部署步骤

### 1. 克隆仓库

```bash
git clone https://gitee.com/difyz/ytb2bili-go.git
cd ytb2bili-go
```

### 2. 编译

```bash
go build -o ytb2bili .
```

### 3. 配置环境变量

```bash
# 飞书应用配置（从你的服务器复制）
export FEISHU_APP_ID="cli_aaa921692978dce6"
export FEISHU_APP_SECRET="x6wwyDmSVlbubdcGpO5nKhdwxniTB7ka"

# 多维表格配置
export BITABLE_APP_TOKEN="MEG6bQc6CaXNwbsM1vZc0jBon0Z"
export BITABLE_TABLE_ID="tblkMgbUbScdevlr"

# DeepSeek API（用于字幕翻译）
export DEEPSEEK_API_KEY="你的 DeepSeek API Key"

# YouTube Cookies（可选，如果需要使用全局 cookies）
export YOUTUBE_COOKIES="/path/to/your/youtube_cookies.txt"
```

### 4. 运行多维表格处理器

```bash
# 前台运行（推荐调试）
./ytb2bili bitable

# 或后台运行
nohup ./ytb2bili bitable > bitable.log 2>&1 &
```

## 工作流程

```
┌─────────────────────────────────────────────────────────────┐
│                     本地部署流程                              │
└─────────────────────────────────────────────────────────────┘

1. Chrome 扩展提交任务到飞书多维表格
   └── 视频 URL + Cookies（包含 .google.com 域的 cookies）

2. 本地 ytb2bili-go 轮询多维表格
   └── 每 30 秒检查一次

3. 处理任务
   ├── 下载视频（使用 yt-dlp + cookies）
   ├── 语音转录（Bcut ASR）
   ├── 字幕翻译（DeepSeek API）
   └── 更新状态为 completed

4. 在飞书多维表格查看结果
```

## 验证

### 1. 检查任务状态

```bash
# 查看日志
tail -f bitable.log

# 或查看多维表格
# 访问: https://gcnkq1umi9ma.feishu.cn/base/MEG6bQc6CaXNwbsM1vZc0jBon0Z
```

### 2. 手动测试下载

```bash
# 测试 yt-dlp 是否能下载
yt-dlp --cookies /path/to/your/cookies.txt --dump-json --no-download "https://www.youtube.com/watch?v=jNQXAC9IVRw"
```

## 常见问题

### Q: yt-dlp 仍然失败？

**检查 cookies 是否有效：**
```bash
# 查看 cookies 文件
cat /path/to/your/cookies.txt

# 确保有以下关键 cookies：
# - __Secure-1PSID
# - __Secure-3PSID
# - SAPISID
# - SSID
# - LOGIN_INFO (来自 .google.com)
# - HSID (来自 .google.com)
# - SID (来自 .google.com)
```

### Q: 如何获取有效的 cookies？

1. 在 Chrome 浏览器登录 YouTube
2. 安装 Chrome 扩展 [Get cookies.txt LOCALLY](https://chromewebstore.google.com/detail/get-cookiestxt-locally/cclelndahbckbenkjhflpdbgdldlbecc)
3. 访问 YouTube，点击扩展图标
4. 导出 cookies（选择 Netscape 格式）

### Q: 如何配置代理？

```bash
# 在运行 ytb2bili 之前设置
export https_proxy=http://your-proxy:port
export http_proxy=http://your-proxy:port

./ytb2bili bitable
```

## 数据目录结构

```
ytb2bili-go/
├── data/
│   ├── downloads/           # 下载的视频
│   │   └── <video_id>/
│   │       ├── *.mp4        # 视频文件
│   │       ├── *.srt        # 英文字幕
│   │       ├── *.zh.srt     # 中文字幕
│   │       └── cookies.txt  # 任务 cookies
│   └── history/
│       └── history.json     # 已提交的历史记录
```

## 停止服务

```bash
# 如果是前台运行，按 Ctrl+C

# 如果是后台运行
pkill -f "ytb2bili bitable"
```
