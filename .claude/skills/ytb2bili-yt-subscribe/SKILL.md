---
name: ytb2bili-yt-subscribe
description: YouTube OAuth 授权登录 → 拉取订阅频道 → 定时检测更新 → 自动搬运到 B站的完整任务流。Use when the user asks to 用 YouTube 账号授权、同步关注的频道、订阅频道自动搬运、定时检测频道更新、或配置 YouTube 订阅到 B站的自动化流水线。
---

# ytb2bili YouTube 订阅自动搬运任务流

通过 YouTube OAuth 授权获取用户关注的频道列表，定时检测频道更新，
发现新视频后自动走完 **下载 → 转录 → 翻译 → 元数据 → TTS 配音 →
音画同步 → 上传B站 → 监听审核 → 字幕投稿** 完整流水线。

> Prerequisite: `ytb` 二进制已构建（`make build`），已配置
> `DEEPSEEK_API_KEY`，已完成 `ytb login`（B站扫码）和 `ytb init`。

---

## 一、一次性配置

### 1. 创建 Google OAuth 凭证

1. 打开 [Google Cloud Console → Credentials](https://console.cloud.google.com/apis/credentials)
2. 创建项目（或选择已有项目）
3. **APIs & Services → Library**：启用 **YouTube Data API v3**
4. **Create Credentials → OAuth client ID**
5. 应用类型选 **"TV and Limited Input devices"**（设备码流程专用，无需回调地址）
   ⚠️ 注意：**不是 Desktop，也不是 Web**——Web 类型客户端调用设备码端点会返回
   `invalid_client: Only clients of type 'TVs and Limited Input devices' can use the OAuth 2.0 flow`，
   即 `ytb yt-oauth login` 报「设备码请求被拒绝 [invalid_client]」。
6. 创建后把 Client ID / Client Secret 填入 `config.yaml`：

```yaml
youtube_oauth:
  client_id: "xxxx.apps.googleusercontent.com"
  client_secret: "GOCSPX-xxxx"
```

或环境变量：`GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET`

### 2. 授权登录

```bash
# 发起设备码授权（打印 URL + 授权码）
./ytb yt-oauth login

# 查看登录状态
./ytb yt-oauth status
```

浏览器打开终端打印的 URL，输入授权码即可完成授权。
token 保存在 `data/yt_oauth_token.json`（自动刷新）。

---

## 二、拉取订阅频道

```bash
# 拉取 YouTube 账号关注的频道列表 → 写入本地订阅存储
./ytb yt-oauth sync

# 拉取并同步每个频道最近 7 天的新视频，自动加入任务队列
./ytb yt-oauth sync --queue

# 指定回看天数
./ytb yt-oauth sync --queue --lookback 14
```

查看结果：

```bash
./ytb channel list          # 频道订阅列表
./ytb channel videos        # 发现的视频
./ytb queue list            # 任务队列
```

---

## 三、定时检测频道更新（自动搬运）

### 方式 A：CLI 常驻监控（推荐）

```bash
# 每 24 小时检测一次订阅频道更新，新视频自动入队
./ytb yt-oauth watch

# 每 12 小时
./ytb yt-oauth watch --interval 12h

# 只检测一次（适合 cron 调用）
./ytb yt-oauth watch --once
```

### 方式 B：系统 cron（不常驻）

```bash
# 每天 6:00 检测一次，检测后消费队列处理
crontab -e
# 添加：
# 0 6 * * * cd /path/to/ytb2bili-cli && ./ytb yt-oauth watch --once >> data/yt-oauth-watch.log 2>&1
# 30 7 * * * cd /path/to/ytb2bili-cli && ./ytb queue work --once >> data/queue-work.log 2>&1
```

---

## 四、消费队列执行完整搬运流水线

```bash
# 前台逐个处理队列中的视频（完整 8 步流水线）
./ytb queue work

# 只处理一个后退出（适合 cron）
./ytb queue work --once

# 查看队列状态
./ytb queue status
./ytb queue list
```

`queue work` 对每个视频自动执行完整任务链：

```
download（下载） → transcribe（转录） → translate（翻译）
→ metadata（AI 元数据） → tts（TTS 配音） → audio-sync（音画同步）
→ upload（上传B站） → subtitle（监听审核通过后上传字幕）
```

任务进度查看：

```bash
./ytb task list
./ytb task show <task_id>
./ytb task retry <task_id>          # 失败续跑（幂等）
./ytb review BV1xxxx               # 审核状态
./ytb subtitle status              # 字幕上传状态
```

---

## 五、完整自动化闭环示例

```bash
# 1. 一次性：授权 + 拉订阅
./ytb yt-oauth login
./ytb yt-oauth sync --queue --lookback 14

# 2. 后台常驻：定时检测 + 持续消费（两个终端）
./ytb yt-oauth watch --interval 24h      # 终端 1：检测更新入队
./ytb queue work                          # 终端 2：消费队列执行搬运

# 3. 或者用 cron 无人值守（见"方式 B"）
```

---

## 六、常见问题

| 问题 | 解决 |
|------|------|
| `client_id 未配置` | 在 config.yaml 填 `youtube_oauth.client_id/secret` |
| `设备码请求被拒绝 [invalid_client]` | OAuth 客户端类型不对——设备码流程只能用 **"TV and Limited Input devices"** 类型，Web/Desktop 都会拒绝（见上文创建步骤） |
| `设备码响应为空` | client_id 错误，或未启用 YouTube Data API v3 |
| `token 已过期且刷新失败` | 重新 `ytb yt-oauth login` |
| `没有活跃的频道订阅` | 先 `ytb yt-oauth sync` 拉取，或 `ytb channel add` |
| B站未登录 | `ytb login` 扫码 |
| 转录/翻译失败 | 检查 `DEEPSEEK_API_KEY`、`ytb init` 环境依赖 |

## 七、管理命令

```bash
./ytb yt-oauth status     # 登录状态
./ytb yt-oauth logout     # 清除凭证
./ytb channel remove <channel_id>   # 移除某个频道订阅
./ytb channel sync --queue          # 手动触发全部频道检测
```
