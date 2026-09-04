---
name: audio-video-sync
description: Synthesize translated SRT subtitles with the local IndexTTS2 HTTP API, align segmented dubbing audio to the subtitle timeline, and merge it into a video. Use for Chinese dubbing, voice cloning, emotion control, timing correction, silence padding, controlled speed adjustment, or final audio-track replacement.
---

# Audio Video Sync

Use `scripts/synthesize_srt.py` to generate indexed WAV speech clips through
IndexTTS2, then use
`scripts/audio_processor_v2.py` for deterministic synchronization.

For YouTube rolling captions, first run `scripts/clean_rolling_srt.py`. Never
synthesize rolling captions directly: adjacent cues repeat text and force
unnaturally high speech speed.

## Inputs

Require:

- An input video readable by ffmpeg.
- An SRT subtitle file defining the target timeline.
- An audio directory containing one clip per subtitle index. Supported names include `1.mp3`, `001.wav`, and `audio_1.mp3`.

Require the IndexTTS2 service at `http://localhost:18765` with a loaded model.
If the service is started with `INDEX_TTS_API_KEY` set (see index-tts-admin
`deploy/index-tts-server.py`), the synthesis script must receive the same key
via `--api-key` (config: `tts.index.api_key`; the Go pipeline and CLI forward
it automatically). A missing key makes `/synthesize` return 401.
The synthesis script defaults to the server's reference voice and natural
emotion, and resumes by skipping existing non-empty WAV clips. The
synchronization script fails on missing clips by default so incomplete dubbing
is visible; pass `--missing silence` only when intentional silence is acceptable.

## Run

```bash
python3 scripts/synthesize_srt.py \
  --srt translated.zh.srt \
  --output-dir tts_audio \
  --emotion auto

python3 scripts/audio_processor_v2.py \
  --video INPUT.mp4 \
  --srt translated.zh.srt \
  --audio-dir tts_audio \
  --output OUTPUT.mp4
```

Add `--no-speed-adjust` to preserve clip speed. The script prints a JSON result as its final stdout line. Treat a non-zero exit code as failure.
The default maximum speed is `1.25`; override it with `--max-speed`. Longer
speech is delayed sequentially to consume later silence rather than being
truncated or mixed with the next sentence.

## Verify

Before processing, run:

```bash
python3 scripts/audio_processor_v2.py --check
```

Use `--ref-audio /server/path/reference.wav` for voice cloning and
`--emo-alpha 0.0..1.0` for emotion strength. Keep concurrency at `1` unless the
GPU server explicitly supports parallel inference.

Require Python packages `pydub` and `pysrt`, plus `ffmpeg` and `ffprobe` on PATH.
Verify that the output exists and contains both a video stream and an audio
stream.
