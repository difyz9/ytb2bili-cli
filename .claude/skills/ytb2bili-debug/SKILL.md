---
name: ytb2bili-debug
description: Troubleshooting guide for the ytb2bili CLI pipeline — diagnose failed steps (download, transcribe, translate, upload, subtitle), invalid cookies, login issues, and missing-dependency problems. Use when a ytb workflow step fails, a task shows failed status, search returns nothing, cookies are invalid, or login expired.
---

# ytb2bili-debug

Systematically diagnose and fix failures in the `ytb` pipeline. Run the
diagnostic flow first, then match the failing step to its fixes.

> For the workflow itself, invoke `ytb2bili-pipeline`; for exact flags,
> `ytb2bili-commands`.

## Diagnostic flow (run in order)

```bash
# 1. Environment dependencies
./ytb init

# 2. Bilibili login state
./ytb whoami                      # ❌ 未登录 / 登录已过期 → ytb login

# 3. YouTube cookies (download/search rate-limit protection)
./ytb cookies test

# 4. Which task failed, and at which step
./ytb task list
./ytb task show <task_id>         # shows per-step status + error message
```

The failing step's `❌` message plus `task show`'s per-step `Error` field tell you
which section below applies.

## Failure fixes by step

### download fails
- **yt-dlp not installed / too old**: `./ytb init --update` or `pip install -U yt-dlp`
- **"Sign in to confirm you're not a bot"**: refresh cookies → `./ytb cookies refresh`, then `./ytb cookies test`
- **Network blocked**: set `HTTP_PROXY`/`HTTPS_PROXY`; verify `curl -I https://www.youtube.com`
- **Age-restricted / region-locked**: needs valid cookies; download may be impossible without them

### transcribe fails
The transcriber is chosen by `config.yaml` → `transcriber.provider` (`whisper` default / `bcut`).

**Bcut ASR (cloud) failures:**
- Check the audio file exists: `ls data/downloads/<videoID>/`
- Bcut ASR may be rate-limited or unavailable; retry the single step:
  `./ytb transcribe --provider bcut <audio file>`
- Long videos may exceed the ASR session — split audio or retry

**whisper.cpp (local) failures:**
- `未找到 whisper-cli` → install: `brew install whisper-cpp` (Debian/Ubuntu: `sudo apt install whisper-cpp`), or set `transcriber.provider: bcut`
- `whisper 模型不存在` → download the model:
  `curl -L -o models/ggml-base.bin https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.bin`
  (path set by `transcriber.whisper.model`)
- `提取音频失败` → check ffmpeg is on PATH and the input file is decodable
- Standalone single-step test: `./ytb transcribe --provider whisper <audio/video file>`
  (旧 `./ytb bcut` / `./ytb whisper` 仍可直接调用，等价于带 `--provider` 的 transcribe)

### translate fails
- **LLM API key missing/invalid**: verify `DEEPSEEK_API_KEY`, `./ytb --config` points at the right config
- **Rate limit**: the translator uses concurrent workers; reduce concurrency or wait
- Retry just the translation: `./ytb translate <input.srt>`

### tts (配音) fails
The pipeline `tts` step selects its provider via `config.yaml` → `tts.provider`:
- `tencent` — Tencent Cloud TTS (needs `tencent_cloud.secret_id/secret_key` or env); no local service
- `index` — local IndexTTS (needs `.venv/bin/python3` + IndexTTS2 HTTP API at `localhost:18765`)
- unset/`auto` — auto-detect: Tencent when credentials present, else IndexTTS

Symptoms:
- `fork/exec .venv/bin/python3: no such file or directory` → `tts.provider` is `index` (or auto fell back) but `.venv` is missing. Set `tts.provider: tencent` in config.yaml.
- `TENCENTCLOUD_SECRET_ID 未设置` → forced Tencent but credentials missing; set `tencent_cloud.secret_id/secret_key` in config.yaml or env.
- Tencent TTS API errors → check the secret/region; per-cue failures fail the step (missing clips break audio-sync).
- Standalone single-step test: `./ytb tts <input.srt>` (writes `1.mp3`, `2.mp3`, …; 旧名 `tencent-tts` 仍可用)

### audio-sync (音画同步) fails
The audio-sync step runs `skills/audio-video-sync/scripts/audio_processor_v2.py`
with a Python interpreter resolved as: `$YTB2BILI_PYTHON` → `.venv/bin/python3`
(repo root) → `python3`. The script needs `pydub`, `pysrt`, and `ffmpeg`/`ffprobe` on PATH.

Symptoms:
- `missing dependencies: No module named 'pydub'` → the venv is missing or incomplete. Run `./ytb init` (step 5 checks it), or `python3 -m venv .venv && .venv/bin/pip install -r skills/audio-video-sync/requirements.txt`
- `missing dependencies: No module named 'pysrt'` → same fix
- Verify the script itself: `.venv/bin/python3 skills/audio-video-sync/scripts/audio_processor_v2.py --check` → `{"ok": true}`
- `ffmpeg`/`ffprobe` not found → install ffmpeg; the script requires both on PATH

### upload to Bilibili fails
- **Not logged in / expired**: `./ytb login`
- **tid invalid**: use a valid B站分区ID (`./ytb submit --tid 122 ...`)
- **Cover/title rejected**: check the generated metadata; retry with `--dry-run` to see metadata before uploading
- **Duplicate**: history records previously submitted videos and blocks re-submission — use a different video

### subtitle not uploaded after review
- Subtitle upload is asynchronous (polls review every 30s, up to 24h). Check
  `./ytb subtitle status` and `data/subtitles/`
- If marked failed, re-run the submit pipeline; the subtitle step retries on review-pass

## Common gotchas

- **Stale binary**: commands act differently than docs → rebuild `make build`
- **Wrong config**: config loads from `--config` → `$YTB2BILI_CONFIG` → `./config.yaml`; if `data_dir` is unexpected, check which file is actually loaded
- **Errors are silent in output**: all errors now go to stderr with `❌` prefix and non-zero exit code — capture stderr (`2>&1`) to see them
- **`queue work` stuck item**: a crashed worker leaves an item `claimed`; the queue's lease eventually expires (see `internal/queue/queue.go`)

## Where to look

| What | Where |
|------|-------|
| Downloaded files | `data/downloads/<videoID>/` |
| Task status | `./ytb task list` / `./ytb task show <id>` |
| Submission history | `./ytb search --history` |
| Subtitle status | `./ytb subtitle status` |
| HTTP server log | `data/server.log` |
| Runtime stderr | captured when running the command |
