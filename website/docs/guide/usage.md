# 命令与核心用法

## CLI 命令总览

```
核心流程        auto / daemon / queue / search / submit
频道与订阅      channel / cookies
流水线步骤      download / transcribe / translate / tts / audio-sync / metadata / publish
B站管理        login / whoami / accounts / review / history / subtitle / task
系统与工具      chain / init / check / debug / server
```

任意命令加 `-h` 查看完整参数；多数命令支持 `--json` 机器可读输出（stdout 仅 JSON，日志转 stderr，退出码 0=成功 / 非0=失败），可被外部 pipeline / AI Agent 作为工具步骤串联。

## 搜索 YouTube

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

## 一键搬运 submit

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

## 任务链与单步命令

任务链可以把任意步骤自由组合：

```bash
ytb chain list                        # 列出可用步骤
ytb chain plan download,upload <URL>  # 只规划不执行
ytb chain run download,transcribe,translate <URL>
ytb chain run download <URL>          # 只下载
```

每个流水线步骤也是独立 CLI 命令，支持 `--json`，可被外部 pipeline / Agent 串联（契约：stdout 仅 JSON，如 `{"ok":true,"step":"download",...}`）：

```bash
ytb download --json "<URL>"                     # 下载（-o 指定目录）
ytb transcribe --json <videoId|file>            # 转录：--provider whisper|bcut；whisper 额外 --model/-l lang/--threads
ytb translate --json <id>.srt                   # 翻译：--source-lang/--target-lang/--test（自检全部服务商连通性）
ytb tts --json <id>.zh-Hans.srt                 # 分段配音（别名 tencent-tts）：-o 输出、--speed/--voice/--volume/--concurrency
ytb audio-sync --json <videoId>                 # 用已有产物合成中文音轨成片：--missing error|silence、--no-speed-adjust
ytb metadata --json <videoId|srt路径>           # 生成标题/简介/标签 JSON（别名 meta，默认 <字幕>.meta.json，-o 可改）
ytb publish --json video.mp4 --title "..."      # 本地视频直接投稿（别名 upload）
```

> **产物落位约定**：`transcribe/translate/tts/audio-sync` 都默认在 `data/downloads/<videoId>/` 读写（按字幕序号 `1.mp3, 2.mp3 ...`），保证各单步可互相衔接。`audio-sync` 输出 `<videoId>.synced.mp4`，随后 `submit <videoId>` 续跑即可收尾投稿。
