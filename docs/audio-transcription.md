# 音频转文本（语音转录 / ASR）实现说明

> 本文档说明 ytb2bili-go 项目中"音频 → 文本"（语音识别 / 转写）功能的实现方式，
> 覆盖后端选型、代码结构、调用流程、配置与 CLI 使用。

## 1. 概述

项目的语音转写（transcribe）步骤负责把下载到的视频中的**语音**识别成文本，并输出为
SRT 字幕文件，供下游的翻译（translate）、TTS 配音（tts）、音画同步（audio-sync）步骤使用。

当前实现支持 **两个可切换的后端**：

| 后端 | 类型 | 说明 |
|------|------|------|
| `bcut` | 云端 ASR | 调用 Bilibili 必剪（Bcut）的免费语音识别接口，无需本地模型 |
| `whisper`（默认） | 本地 | 调用本地 whisper.cpp（`whisper-cli` 子进程）进行离线转写 |

> 注意：项目早期（AGENTS.md 中的"Bcut ASR (必剪) 免费"）默认走云端必剪，
> 但当前代码已改为**默认 whisper 本地转写**，只有在显式配置 `transcriber.provider: bcut`
> 时才走云端。

## 2. 代码结构

```
internal/transcriber/
├── transcriber.go   # Bcut ASR（必剪云服务）实现 + SRT 生成
├── whisper.go       # whisper.cpp 本地转录实现
└── whisper_test.go  # whisper 控制台输出解析的单元测试
```

相关调用点：

| 文件 | 作用 |
|------|------|
| `internal/pipeline/steps.go` | `transcribeStep` 步骤，按 provider 分发到 Bcut 或 whisper |
| `internal/config/config.go` | `TranscriberConfig` / `WhisperConfig` 配置定义与默认值 |
| `internal/cmd/tools.go` | `transcribe` / `bcut` / `whisper` 三个 CLI 命令 |
| `internal/server/debugger.go` | 调试诊断里直接调用 `BcutASR`（旧路径，仅供诊断） |

## 3. 后端选择逻辑

统一入口是 `internal/pipeline/steps.go` 中的 `selectTranscriberProvider`：

```go
func selectTranscriberProvider(cfg *config.Config) string {
    if cfg != nil {
        switch strings.ToLower(strings.TrimSpace(cfg.EffectiveTranscriberProvider())) {
        case "bcut":
            return "bcut"
        }
    }
    return "whisper"
}
```

`EffectiveTranscriberProvider()`（`internal/config/config.go`）的逻辑：

- 配置为 `bcut`（不区分大小写、忽略首尾空白）→ 返回 `bcut`
- 其它任何值 / 未配置 → 返回 `whisper`（默认）

即在 `transcribe` 步骤中：

```go
switch selectTranscriberProvider(s.config) {
case "bcut":
    state.Result.SubtitlePath, err = transcriber.BcutASRContext(ctx, ...)
default: // whisper
    var wcfg *config.WhisperConfig
    if s.config != nil && s.config.Transcriber != nil {
        wcfg = s.config.Transcriber.Whisper
    }
    state.Result.SubtitlePath, err = transcriber.WhisperContext(ctx, wcfg, ...)
}
```

## 4. 后端一：Bcut ASR（必剪云服务）

实现文件：`internal/transcriber/transcriber.go`

### 4.1 对外接口

```go
func BcutASR(videoPath, outputDir, videoID string) (string, error)          // 便捷封装（context.Background）
func BcutASRContext(ctx context.Context, videoPath, outputDir, videoID string) (string, error)
```

- 输入：视频（或音频）文件路径、输出目录、用于命名 SRT 文件的 videoID
- 输出：生成的 SRT 文件路径（`<outputDir>/<videoID>.srt`）

### 4.2 实现流程（7 步）

```
Step 1  提取音频        ffmpeg 提取 16kHz 单声道 MP3（libmp3lame 128k）
Step 2  申请上传        POST /resource/create 获取分片上传地址
Step 3  分片上传        PUT 各分片到 upload_urls，收集 ETag
Step 4  提交上传        POST /resource/create/complete 换取 download_url
Step 5  创建转录任务    POST /task，返回 task_id
Step 6  轮询查询结果    GET  /task/result，state=4 表示成功
Step 7  生成 SRT        将 utterances 写成标准 SRT 文件
```

#### Step 1：提取音频

```go
audioPath := filepath.Join(outputDir, "audio.mp3")
cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", videoPath,
    "-vn", "-acodec", "libmp3lame", "-ab", "128k",
    "-ar", "16000", "-ac", "1", audioPath)
```

- 去视频流（`-vn`）、采样率 16kHz、单声道（`-ac 1`）、MP3 128kbps
- 临时文件 `audio.mp3` 用完即删（`defer os.Remove`）

#### Step 2~4：分片上传

必剪使用类似对象存储的"申请 → 分片 PUT → 提交"协议：

- `requestUpload`：`POST /resource/create`，payload 携带 `type=2`、文件名、大小、
  `ResourceFileType=mp3`、`model_id="8"`
- `uploadParts`：按 `per_size`（默认 5MB）切片，逐片 `PUT` 到返回的 `upload_urls`
- `commitUpload`：`POST /resource/create/complete`，携带 `InBossKey`、`ResourceId`、
  逗号拼接的 `Etags`、`UploadId`，换回 `download_url`

#### Step 5~6：转录任务与轮询

- `createTask`：`POST /task`，payload `{resource: download_url, model_id: "8"}`，返回 `task_id`
- `queryResult`：`GET /task/result?model_id=8&task_id=xxx`，最多轮询 300 次、每次 2 秒：

| state | 含义 | 处理 |
|-------|------|------|
| 4 | 成功 | 解析 `result` 字段得到转录结果 |
| 3 | 失败 | 直接返回错误 |
| 其它 | 处理中 | 继续等待（每 10 次打一条进度日志） |

#### Step 7：生成 SRT

```go
func generateSRT(r *bcutResult, path string) error
```

遍历 `utterances`，每条生成：

```srt
1
00:00:01,000 --> 00:00:03,500
文本内容
```

`formatSRTTime` 负责把秒数格式化为 `HH:MM:SS,mmm`。

### 4.3 Bcut 返回结构

```go
type bcutResult struct {
    Language   string    `json:"language"`
    Utterances []Segment `json:"utterances"`
}

type Segment struct {
    Transcript string  `json:"transcript"`   // 文本
    StartTime  float64 `json:"start_time"`   // 起始时间（毫秒）
    EndTime    float64 `json:"end_time"`     // 结束时间（毫秒）
}
```

> 时间戳约定：Bcut 返回的 `start_time` / `end_time` 为**毫秒**，因此
> `generateSRT` 中做了 `/ 1000.0` 换算成秒。

## 5. 后端二：whisper.cpp（本地转录）

实现文件：`internal/transcriber/whisper.go`

### 5.1 对外接口

```go
func WhisperContext(ctx context.Context, wcfg *config.WhisperConfig,
    videoPath, outputDir, videoID, language string) (string, error)
```

- `wcfg`：whisper 配置（二进制路径 / 模型路径 / 线程数），可为 nil（走默认值）
- `language`：语言代码（`en` / `zh` / ...）；空或 `"auto"` 时交给 whisper 自动检测
- 输出：`<outputDir>/<videoID>.srt`（与 BcutASR 命名一致，下游无感知）

### 5.2 实现流程

```
前置检查     检查 whisper-cli 二进制与 GGML 模型是否存在
Step 1        ffmpeg 提取 16kHz 单声道 PCM WAV
Step 2        whisper-cli 子进程直接输出 SRT
回退          若未生成 .srt 文件，从控制台输出解析时间戳行
```

#### 前置检查（给出友好报错）

```go
if _, err := exec.LookPath(binary); err != nil {
    return "", fmt.Errorf("未找到 %s，请先安装 whisper.cpp（macOS: brew install whisper-cpp；Debian/Ubuntu: sudo apt install whisper-cpp）", binary)
}
if _, err := os.Stat(model); err != nil {
    return "", fmt.Errorf("whisper 模型不存在: %s\n  请下载: ... curl -L -o ... ggml-base.bin", model)
}
```

模型路径支持 `~/` 开头的主目录展开（`config.ExpandHome`）。

#### Step 1：提取音频

```go
cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", videoPath,
    "-vn", "-ar", "16000", "-ac", "1", "-c:a", "pcm_s16le", wavPath)
```

- 输出格式为 **16kHz 单声道 PCM WAV**（whisper 的最佳输入格式）
- 使用 `os.CreateTemp` 生成唯一临时文件名，避免与输入同名文件冲突
- 临时 WAV 用完即删（`defer os.Remove`）

#### Step 2：调用 whisper-cli

```go
args := []string{
    "-m", model,
    "-t", strconv.Itoa(threads),
    "-l", lang,        // auto / en / zh / ...
    "-osrt",           // 输出 SRT 格式
    "-of", filepath.Join(outputDir, videoID),  // 输出文件前缀（自动补 .srt）
    wavPath,
}
args = append(args, "--no-gpu")  // 强制 CPU 转录
cmd = exec.CommandContext(ctx, binary, args...)
```

要点：

- `-osrt`：让 whisper-cli 直接写 `<of>.srt`，省去手动解析
- `--no-gpu`：**强制 CPU 转录**，避免与 IndexTTS 等 GPU 服务争抢显存导致 CUDA OOM
  （代码注释：RTX 5060 Ti 16GB 被 IndexTTS 占一半后 GPU 转录必崩）

#### 回退：控制台输出解析

如果 `whisper-cli` 未生成 `.srt` 文件（部分版本行为差异），则从 stdout 解析时间戳行：

```go
var whisperSegmentRe = regexp.MustCompile(
    `\[\s*(\d{2}):(\d{2}):(\d{2})\.(\d{3})\s*-->\s*(\d{2}):(\d{2}):(\d{2})\.(\d{3})\s*\]\s*(.*)`)
```

匹配形如 `[00:00:00.000 --> 00:00:02.500]   Hello world` 的行，提取起止时间与文本，
再复用 `generateSRT` 写文件。

> ⚠️ 时间单位差异：`timestampToSeconds` 返回的是**秒**（见 `whisper_test.go` 断言
> `StartTime == 0, EndTime == 10.5`），而 `generateSRT` 内部按 Bcut 的毫秒约定做了
> `/ 1000.0`。因此这条**回退路径**的时间戳与主路径（`-osrt` 直接产出）可能存在
> 单位换算偏差；正常路径下 whisper-cli 会直接生成 `.srt`，该回退逻辑极少触发。

## 6. 配置

`config.yaml` 的 `transcriber:` 段：

```yaml
transcriber:
  provider: whisper          # bcut(云ASR) | whisper(本地whisper.cpp)
  whisper:
    binary: whisper-cli      # whisper-cli 可执行路径（按 PATH 查找）
    model: ~/.biliup/models/ggml-base.bin   # GGML 模型文件路径（支持 ~/ 展开）
    threads: 4               # 推理线程数
```

对应 Go 结构（`internal/config/config.go`）：

```go
type TranscriberConfig struct {
    Provider string         `yaml:"provider"`
    Whisper  *WhisperConfig `yaml:"whisper"`
}

type WhisperConfig struct {
    Binary  string `yaml:"binary"`
    Model   string `yaml:"model"`
    Threads int    `yaml:"threads"`
}
```

默认值（未配置时）：

- `provider` → `whisper`
- `binary` → `whisper-cli`
- `model` → `models/ggml-base.bin`
- `threads` → `4`

## 7. 在流水线中的集成

`internal/pipeline/steps.go` 中的 `transcribeStep`：

1. **幂等检查**：若下载目录已存在 `<videoID>.srt`，直接复用，跳过转写
2. **分发**：按 `selectTranscriberProvider` 结果调用 Bcut 或 whisper
3. **写入状态**：结果保存到 `state.Result.SubtitlePath`，供下游步骤使用

该步骤在 workflow 中的定义：

```go
func (*transcribeStep) Definition() workflow.Step {
    return workflow.Step{Name: "transcribe", Description: "转录字幕", Requires: []string{"download"}}
}
```

## 8. CLI 命令

### `ytb transcribe`（统一入口，推荐）

```bash
ytb transcribe video.mp4                    # 默认 whisper
ytb transcribe --provider bcut video.mp4    # 云 ASR
ytb transcribe --model models/ggml-base.bin -l en video.mp4
```

### `ytb bcut`（直接走必剪云服务）

```bash
ytb bcut audio.mp4
```

### `ytb whisper`（隐藏命令，已被 transcribe 取代）

```bash
ytb whisper --model models/ggml-small.bin -l en video.mp4
```

## 9. 相关 API 端点

| 用途 | 端点 |
|------|------|
| Bcut 申请上传 | `POST https://member.bilibili.com/x/bcut/rubick-interface/resource/create` |
| Bcut 提交上传 | `POST https://member.bilibili.com/x/bcut/rubick-interface/resource/create/complete` |
| Bcut 创建任务 | `POST https://member.bilibili.com/x/bcut/rubick-interface/task` |
| Bcut 查询结果 | `GET https://member.bilibili.com/x/bcut/rubick-interface/task/result` |

（`model_id` 固定为 `"8"`，请求 `User-Agent` 为 `Bilibili/1.0.0 (https://www.bilibili.com)`）

## 10. 小结

| 维度 | Bcut（云） | whisper.cpp（本地） |
|------|-----------|---------------------|
| 依赖 | 网络 + 必剪接口 | 本地 `whisper-cli` + GGML 模型 |
| 成本 | 免费（云端） | 本地算力（强制 CPU） |
| 隐私 | 音频上传第三方 | 全程离线 |
| 时间戳来源 | 接口返回（毫秒） | `-osrt` 直接产出 SRT |
| 当前默认 | 否 | **是** |
