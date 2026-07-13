# GitHub Copilot Instructions - ytb2bili-go

## Project Overview

ytb2bili-go is a Go-based tool for transferring YouTube videos to Bilibili with automatic transcription, translation, and subtitle upload.

## Quick Start

```bash
# Environment
export DEEPSEEK_API_KEY=*** export YOUTUBE_COOKIES="/path/to/youtube_cookies.txt"

# Build & Run
cd /home/ubuntu/ytb2bili-go
go build -o ytb2bili .
./ytb2bili submit "https://www.youtube.com/watch?v=VIDEO_ID"
```

## Architecture

### Core Workflow

```
YouTube Video
    ↓ (yt-dlp)
Video File + Subtitles
    ↓ (ffmpeg)
Audio File (MP3)
    ↓ (Bcut ASR)
SRT Subtitles (English)
    ↓ (DeepSeek LLM)
SRT Subtitles (Chinese)
    ↓ (Bilibili API)
Video + Subtitles Uploaded
    ↓ (Async)
Watch Review → Auto Upload Subtitle
```

### Package Structure

| Package | File | Purpose |
|---------|------|---------|
| `command` | `command.go` | CLI entry point, workflow orchestration |
| `download` | `download.go` | YouTube download via yt-dlp |
| `transcriber` | `transcriber.go` | Bcut ASR speech-to-text |
| `translator` | `translator.go` | Batch LLM translation |
| `metadata` | `metadata.go` | AI-generated title/desc/tags |
| `bili` | `bili.go` | Bilibili upload API |
| `channel` | `channel.go` | YouTube RSS channel monitor |

## Key APIs

### Bcut ASR (Free Speech-to-Text)

```
Base URL: https://member.bilibili.com/x/bcut/rubick-interface

POST /resource/create      → Request upload
PUT  {upload_url}          → Upload chunk
POST /resource/create/complete → Commit upload
POST /task                 → Create transcription task
GET  /task/result          → Poll result
```

### DeepSeek LLM (Translation)

```
POST https://api.deepseek.com/chat/completions

{
  "model": "deepseek-chat",
  "messages": [
    {"role": "system", "content": "Translate subtitles..."},
    {"role": "user", "content": "English text..."}
  ]
}
```

### Bilibili SDK

```go
import "github.com/difyz9/bilibili-go-sdk/bilibili"

client := bilibili.NewClient()
uploadClient := bilibili.NewUploadClient(loginInfo)

// Upload video
video, _ := uploadClient.UploadVideo(path)

// Submit
result, _ := uploadClient.SubmitVideo(studio)

// Upload subtitle
client.UploadSubtitle(loginInfo, bvid, subtitlePath, lang)

// Wait for review
status, _ := client.WaitForVideoReviewPassed(bvid, cookies, interval, timeout)
```

## Common Patterns

### Error Handling

```go
if err != nil {
    return fmt.Errorf("context failed: %w", err)
}
```

### HTTP Requests with Context

```go
req, _ := http.NewRequestWithContext(ctx, "POST", url, body)
req.Header.Set("User-Agent", "Bilibili/1.0.0")
req.Header.Set("Content-Type", "application/json")
```

### File Chunk Upload

```go
for i := 0; i < len(data); i += chunkSize {
    end := i + chunkSize
    if end > len(data) { end = len(data) }
    chunk := data[i:end]
    // Upload chunk
}
```

## Testing

```bash
# Run specific test
go test -v -run TestName ./internal/package/

# Run with timeout
go test -timeout 3m ./internal/transcriber/
```

## Debugging

1. Use `--dry-run` to test without uploading
2. Check `data/downloads/` for downloaded files
3. Check `data/tasks/` for task status
4. Enable verbose output with environment variables
