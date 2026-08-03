---
name: ytb2bili-pipeline
description: End-to-end workflow for the ytb2bili tool — install, configure, search, submit (download→transcribe→translate→metadata→upload→subtitle), channel monitoring, batch auto mode, and queue consumption. Use when the user asks to 搬运/投稿 YouTube 视频到 B站, search and submit videos, monitor a YouTube channel, run the auto batch workflow, or process a video through the full pipeline.
---

# ytb2bili-pipeline

Drive the complete YouTube → Bilibili workflow through the `ytb` CLI. Build it
once, then use it to search, submit, monitor, and batch-process videos.

> Prerequisite: the `ytb` binary must be built. If missing, install it first
> (below). For exact flags of a single command, see `ytb2bili-commands`. If a
> step fails, see `ytb2bili-debug`.

## Install & configure (one-time)

```bash
# 1. Build the CLI (binary is ./ytb)
make build          # or: go build -o ytb .

# 2. Verify environment dependencies (ffmpeg/yt-dlp/python/deno)
./ytb init

# 3. Configure
#    - config.yaml in cwd, or --config <path>, or $YTB2BILI_CONFIG
#    - Required env: DEEPSEEK_API_KEY
#    - Optional env: YOUTUBE_COOKIES (anti rate-limit), LLM_MODEL, LLM_BASE_URL

# 4. Login to Bilibili (one-time QR scan)
./ytb login
./ytb whoami       # verify login
```

## Core workflows

### 1. Search & discover

```bash
./ytb search "flutter tutorial"
./ytb search --sort view_count --duration long --upload-date this_week "AI"
./ytb search --json --max 5 "golang"      # machine-readable output
./ytb search --history                     # previously submitted videos
./ytb search --submit 1 "flutter tutorial" # submit the Nth result directly
```

### 2. Full submit pipeline

`submit` runs: **download → transcribe → translate → metadata → upload → subtitle**.
Upload is immediate; subtitle upload happens asynchronously after review passes.

```bash
./ytb submit "https://www.youtube.com/watch?v=VIDEO_ID"
./ytb submit --dry-run "https://www.youtube.com/watch?v=VIDEO_ID"   # no upload
./ytb submit --skip-translate "https://www.youtube.com/watch?v=VIDEO_ID"
./ytb submit --tid 122 "https://www.youtube.com/watch?v=VIDEO_ID"   # bilibili section
./ytb chain run download,transcribe,translate "<URL>"               # custom chain
./ytb chain plan download,upload "<URL>"                            # plan only
./ytb chain list                                                    # available steps
```

After submit, monitor progress:

```bash
./ytb task list
./ytb task show <task_id>
```

### 3. Channel monitoring

```bash
./ytb channel add UCBJcsmduvYEL83R_U4JriQ                      # 频道；自动同步+入队最近 7 天
./ytb channel add --lookback 14 PLlYbQHffs-L9VmQDOMgRb9ASHPCmieBlK   # 播放列表，最近 14 天
./ytb channel add --lookback 0 <id>                            # 不限制时间范围
./ytb channel list
./ytb channel sync --lookback 7            # discover new videos
./ytb channel sync --lookback 7 --queue    # auto-enqueue discovered videos
./ytb channel videos                       # view discovered videos
./ytb channel remove <channel_id>
```

`channel add` 支持频道(`UC...`)与播放列表(`PL...`)，添加后按 `--lookback`（默认 7 天，
`0`=不限）同步并自动将新视频加入任务队列（队列与历史双重去重）。

### 4. Batch / auto mode

`auto` searches keywords, scores candidates (popular/fresh/balanced), and either
queues them or submits directly.

```bash
./ytb auto "machine learning" "neural network"          # score → enqueue
./ytb auto --dry-run --scorer balanced --min-views 1000 "python"
./ytb auto --submit --max-videos 5 --scorer fresh "golang"
```

### 5. Consume the queue

```bash
./ytb queue status
./ytb queue work --once      # process one item then exit
./ytb queue work             # continuous consumer (Ctrl+C to stop)
```

### 6. HTTP API server (optional)

```bash
./ytb server start      # background daemon
./ytb server status
./ytb server stop
```

## Verification checklist

- [ ] `./ytb init` shows all dependencies ✅
- [ ] `./ytb whoami` reports a valid Bilibili account
- [ ] `./ytb search --max 3 "test"` returns results
- [ ] `./ytb task list` shows the task; `./ytb task show <id>` shows step status
- [ ] For submission, the BVID is printed: `https://www.bilibili.com/video/<bvid>`

## References

- Command reference: invoke `ytb2bili-commands`
- Troubleshooting: invoke `ytb2bili-debug`
- Full docs: `AGENTS.md`, `INSTALL_AGENT.md`
