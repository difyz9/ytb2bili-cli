# IndexTTS2 HTTP API 文档

> TTS 服务器运行在 `http://localhost:18765`，模型常驻显存，直接 HTTP POST 即可调用。

---

## 快速开始

```bash
# 健康检查
curl http://localhost:18765/health

# 列出可用情感预设
curl http://localhost:18765/presets
```

---

## 1. 基础语音合成

### 请求

```
POST /synthesize
Content-Type: application/json
```

### 最简单的调用

```bash
curl -X POST http://localhost:18765/synthesize \
  -H "Content-Type: application/json" \
  -d '{"text": "你好，欢迎使用 IndexTTS 语音合成。"}'
```

返回示例：
```json
{"output": "/tmp/indextts_xxxxxx.wav"}
```

**说明：** 不指定 `output` 时，自动存到临时目录 `/tmp/indextts_*.wav`。

### 指定输出路径

```bash
curl -X POST http://localhost:18765/synthesize \
  -H "Content-Type: application/json" \
  -d '{
    "text": "今天天气真不错，适合出去走走。",
    "output": "/home/guan/tts_output/demo.wav"
  }'
```

---

## 2. 情感控制

### 情感预设

| 情感 | 名称 | 说明 |
|------|------|------|
| 默认 | `default` | 保留参考音频的自然语气 |
| 高兴 | `happy` | 欢快、愉悦 |
| 生气 | `angry` | 愤怒、激动 |
| 悲伤 | `sad` | 悲伤、低沉 |
| 恐惧 | `fearful` | 害怕、紧张 |
| 惊讶 | `surprised` | 惊讶、惊喜 |
| 平静 | `calm` | 冷静、平和 |
| 自动 | `auto` | 根据文本内容自动推断情感 |

### 使用情感预设

```bash
curl -X POST http://localhost:18765/synthesize \
  -H "Content-Type: application/json" \
  -d '{
    "text": "这真是太棒了！我完全没想到会这么顺利。",
    "emotion": "happy",
    "emo_alpha": 0.7
  }'
```

- `emo_alpha`：情感强度，范围 `0.0`（最弱）~ `1.0`（最强），默认 `0.6`

### 自动情感推断

```bash
curl -X POST http://localhost:18765/synthesize \
  -H "Content-Type: application/json" \
  -d '{
    "text": "听到这个消息，我感到无比悲痛和惋惜。",
    "emotion": "auto"
  }'
```

### 自定义情感向量

```bash
curl -X POST http://localhost:18765/synthesize \
  -H "Content-Type: application/json" \
  -d '{
    "text": "今天是个特别的日子。",
    "emotion": "vec:0.0,0.0,0.0,0.0,0.0,0.0,0.0,1.0",
    "emo_alpha": 0.6
  }'
```

情感向量为 8 维数组，维度顺序：
```
[happy, angry, sad, afraid, disgusted, melancholic, surprised, calm]
```

---

## 3. 语音克隆（Voice Cloning）

### 使用默认参考音频

服务器默认使用 `/home/guan/guan/code/index-tts/tests/sample_prompt.wav` 作为音色源。

```bash
curl -X POST http://localhost:18765/synthesize \
  -H "Content-Type: application/json" \
  -d '{
    "text": "这是用默认音色克隆生成的语音。"
  }'
```

### 使用自定义参考音频

通过 `ref_audio` 参数指定音色源文件（推荐 3~10 秒的清晰人声 WAV）：

```bash
curl -X POST http://localhost:18765/synthesize \
  -H "Content-Type: application/json" \
  -d '{
    "text": "这是克隆了其他人声音色的语音。",
    "ref_audio": "/home/guan/guan/code/index-tts/examples/voice_05.wav"
  }'
```

### 可用的参考音频

所有参考音频位于 `/home/guan/guan/code/index-tts/examples/`：

| 文件 | 时长 | 说明 |
|------|------|------|
| `voice_01.wav` | 2.4s | 女声示例 1 |
| `voice_02.wav` | 2.9s | 女声示例 2 |
| `voice_03.wav` | 2.1s | 女声示例 3 |
| `voice_04.wav` | 2.3s | 女声示例 4 |
| `voice_05.wav` | 8.4s | 女声示例 5（较长，推荐）|
| `voice_06.wav` | 6.2s | 女声示例 6 |
| `voice_07.wav` | 2.0s | 男声示例 1 |
| `voice_08.wav` | 1.5s | 男声示例 2 |
| `voice_09.wav` | 10.2s | 男声示例 3（最长）|
| `voice_11.wav` | 7.9s | 女声示例 7 |
| `voice_12.wav` | 2.7s | 女声示例 8 |

也可使用项目根目录的音频（可能为自定义录音）：
- `audio_broadcaster.wav` - 主播风格
- `audio_runninghub.wav` - RunningHub 风格
- `ref_female_broadcaster.wav` - 女声主播

---

## 4. 语音克隆 + 情感合成

将语音克隆和情感控制结合使用：

```bash
curl -X POST http://localhost:18765/synthesize \
  -H "Content-Type: application/json" \
  -d '{
    "text": "太让人感动了，我真的不知道该怎么表达我的感谢。",
    "ref_audio": "/home/guan/guan/code/index-tts/examples/voice_05.wav",
    "emotion": "happy",
    "emo_alpha": 0.8
  }'
```

---

## 5. 完整参数说明

### POST `/synthesize`

| 参数 | 类型 | 必填 | 默认 | 说明 |
|------|------|------|------|------|
| `text` | string | **是** | — | 要合成的文本 |
| `output` | string | 否 | 临时文件 | 输出 WAV 文件路径 |
| `ref_audio` | string | 否 | 默认参考音频 | 语音克隆音色源（WAV 文件路径）|
| `emotion` | string | 否 | `"default"` | 情感预设：`default`/`happy`/`angry`/`sad`/`fearful`/`surprised`/`calm`/`auto`，或自定义向量 `vec:x,x,x,x,x,x,x,x` |
| `emo_alpha` | float | 否 | `0.6` | 情感强度，范围 0.0~1.0 |
| `use_emo_text` | bool | 否 | `false` | 是否使用文本情感推断 |
| `emo_text` | string | 否 | `null` | 自定义情感描述文本 |

### GET `/health`

返回服务器和模型状态。

### GET `/presets`

返回可用情感预设列表。

---

## 6. Python 调用示例

```python
import requests

API = "http://localhost:18765"

# 基础合成
resp = requests.post(f"{API}/synthesize", json={
    "text": "基于深度学习的语音合成技术正在改变人机交互方式。",
    "output": "/home/guan/tts_output/tts_demo.wav",
    "emotion": "calm",
})
print(resp.json())

# 语音克隆 + 情感
resp = requests.post(f"{API}/synthesize", json={
    "text": "今天学了一个新知识，特别开心！",
    "ref_audio": "/home/guan/guan/code/index-tts/examples/voice_09.wav",
    "emotion": "happy",
    "emo_alpha": 0.7,
    "output": "/home/guan/tts_output/clone_happy.wav",
})
print(resp.json())
```

---

## 7. 常见问题

**Q: 返回 500 错误？**
检查参考音频路径是否正确，确保文件是有效的 WAV 格式。

**Q: 合成结果不理想？**
- 参考音频建议 3~10 秒清晰人声，无背景噪音
- 调整 `emo_alpha` 控制情感强度
- 尝试不同的参考音频获得更好的音色克隆效果

**Q: 如何停止服务器？**
```bash
kill $(lsof -t -i:18765)
```
