# 数据目录与产物

## 目录约定

| 路径 | 内容 |
|------|------|
| `data/downloads/<videoId>/` | 每视频全部产物（见[项目简介](/guide/introduction)） |
| `data/tasks/*.json` | 任务状态（步骤进度/错误） |
| `data/queue/queue.json` | 作业队列状态 |
| `data/history/history.json` | 提交历史（去重依据） |
| `data/subtitles/*.json` | 字幕上传状态（持久化，重启不丢） |
| `data/daemon/heartbeat.json` | daemon 心跳（批次/PID/当前任务/队列统计） |
| `data/audit/events.jsonl` | 审计事件（`queue audit` 数据源） |
| `data/subscriptions/`、`data/monitored_videos/`、`data/channel_scores.json` | 频道订阅 / 发现视频 / 基线评分缓存 |
| `data/server.log` | HTTP 服务日志 |
| `data/cookies/`、`data/chrome-profile/` | YouTube cookies / Chrome 调试配置 |

数据根目录可在 `config.yaml` 的 `data_dir` 字段修改；`data/` 默认不入库（见仓库 `.gitignore`）。

## 产物与幂等续跑

每个视频的处理产物集中在 `data/downloads/<videoId>/`：

| 产物 | 说明 |
|------|------|
| `<videoId>.mp4` | yt-dlp 下载的原始视频 |
| `<videoId>.srt` | YouTube 源语言字幕（若存在） |
| `<videoId>.zh-Hans.srt` | 翻译后的中文字幕 |
| `voice/1.mp3, 2.mp3 ...` | TTS 按字幕序号合成的分段配音 |
| `<videoId>.synced.mp4` | 音画同步后的成片（合成中文音轨） |
| `cover.jpg` | 视频封面 |

**幂等续跑**：所有步骤会检查产物是否存在，存在则跳过——download（视频文件）、transcribe（`.srt`）、translate（`.zh-Hans.srt`）、tts（`voice/` 配音）、audio-sync（`.synced.mp4`）。重跑 `submit <videoId>` 不会重新下载/转录/翻译/合成配音，只做缺失的剩余部分，是任务重试与断点续传的基础。
