# 频道监控 channel

两条视频发现路线：**RSS 免 OAuth**（推荐起步）与 **YouTube OAuth**（同步账号订阅）。

## A. RSS 方式（免 OAuth）

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

## B. OAuth 方式（同步账号订阅）

```bash
ytb channel login      # Google 设备码授权（需 Google Cloud 建 OAuth Client，见下）
ytb channel import     # 导入账号的全部订阅频道
ytb channel status     # 查看授权状态
ytb channel logout     # 清除授权
```

> **OAuth 凭证**：Google Cloud Console → APIs & Services → Credentials 创建 **OAuth 2.0 客户端**（类型选 "桌面应用" 或 "TV and Limited Input devices" 以支持设备码流程），启用 YouTube Data API v3，把 client_id/secret 填入 `config.yaml` 的 `youtube_oauth` 段。

## 基线评分

`nowcast` 评分与 `channel rank` 的数据基础（抗爆款污染）：

```bash
ytb channel baseline <channel_id|@handle>   # 用 yt-dlp 抓最近 N 个视频算 trimmed mean 基线
ytb channel baseline --all --samples 30 --trim 3
```
