---
name: ytb2bili-tool
description: Use the ytb CLI as a composable, scriptable tool — call individual pipeline steps (download / transcribe / translate / tts / metadata / audio-sync) standalone with machine-readable --json output and exit codes, then chain them into a pipeline (shell or agent skill) with other CLI tools. Use when the user wants to run a single step, glue ytb steps together with other tools, or drive ytb programmatically instead of the monolithic submit/daemon workflow.
---

# ytb2bili-tool

把 `ytb` 当作一个**可组合的 CLI 工具**来用：每个流水线步骤都能**单独调用**，用
`--json` 输出机器可读结果 + 正确的退出码，方便拼进任意 pipeline（shell 脚本 / Agent
skill / 其它工具）。

> 完整命令参考见 `ytb2bili-commands`；端到端一键流程见 `ytb2bili-pipeline`；
> 排障见 `ytb2bili-debug`。本 skill 只讲"作为工具被外部调用"的契约。

## 工具契约（Tool Contract）

所有步骤命令遵循同一契约：

1. **幂等**：产物已存在则跳过，重复调用安全（可安全重试）。
2. **`--json`**：stdout 只输出一个 JSON 对象（`ok` + 步骤字段），人读日志转 stderr。
3. **退出码**：成功 `0`，失败非 `0`（错误写 stderr）。
4. **可独立调用**：参数接受 `videoId` 或完整路径，见 `ytb2bili-commands` 的约定。

```bash
ytb download --json "<url>"          # → {"ok":true,"step":"download","video":"...","dir":"..."}
ytb transcribe --json "<videoId>"    # → {"ok":true,"step":"transcribe","srt":"..."}
ytb translate --json "<id>.srt"      # → {"ok":true,"step":"translate","output":"..."}
ytb tts --json "<id>.zh-Hans.srt"    # → {"ok":true,"step":"tts","voice_dir":"..."}
ytb metadata --json "<videoId>"      # → {"ok":true,"step":"metadata","title":"...","output":"..."}
ytb audio-sync --json "<videoId>"    # → {"ok":true,"step":"audio-sync","output":"<id>.synced.mp4"}
ytb submit --json "<url>"            # → {"ok":true,"step":"submit","bvid":"BV..."}
ytb chain run --json download,translate "<url>"
```

失败时（退出码非 0），stdout 无 JSON，错误在 stderr：
```bash
if ytb download --json "$URL" > out.json 2> err.log; then
  VIDEO=$(python3 -c 'import json,sys;print(json.load(open("out.json"))["video"])')
else
  echo "download failed"; cat err.log; exit 1
fi
```

## 步骤 JSON 字段

| 步骤 | 关键输出字段 |
|------|--------------|
| `download` | `video`, `dir`, `cover`, `title`, `video_id` |
| `transcribe` | `srt`, `provider`, `video_id` |
| `translate` | `output`（译文 .srt 路径）, `input`, `source_lang`, `target_lang` |
| `tts` | `voice_dir`, `provider`, `success`, `failed` |
| `metadata` | `title`, `description`, `tags`, `output`（.meta.json） |
| `audio-sync` | `output`（.synced.mp4）, `duration`, `clips` |
| `submit` / `chain run` | `bvid`, `task_id`, `video_id`, `plan` |

> 所有对象都含 `ok:true` 与 `step`。用 `ytb <step> --json` 打印完整结构确认。

## 步骤数据流（产物路径约定）

下载根目录由 config 的 `download_dir`（默认 `<data_dir>/downloads`）控制：

```
<download_dir>/<videoId>/
├── <videoId>.mp4             # download 产出（video）
├── cover.jpg                 # download 产出（封面）
├── <videoId>.srt             # transcribe 产出（源字幕）
├── <videoId>.zh-Hans.srt     # translate 产出（译文，target-lang 可改）
├── <videoId>.zh-Hans.meta.json  # metadata 产出
├── voice/1.mp3 ...           # tts 产出（分段配音）
└── <videoId>.synced.mp4      # audio-sync 产出
```

每个步骤的**输出正是下一步的输入**，所以既可用内置 `chain run` 串，也可在外部 pipeline
里用文件路径手工串（见下）。

## 外部 Pipeline 组合示例

### shell 脚本串联（每步一个 `ytb` 调用）

```bash
#!/usr/bin/env bash
set -euo pipefail
URL="$1"; ID="${2:-}"

ytb download --json "$URL" > /tmp/d.json 2>/dev/null
ytb transcribe --json "${ID:-$URL}" > /tmp/t.json 2>/dev/null
SRT=$(python3 -c 'import json;print(json.load(open("/tmp/t.json"))["srt"])')
ytb translate --json "$SRT" > /tmp/tr.json 2>/dev/null
ZHSRT=$(python3 -c 'import json;print(json.load(open("/tmp/tr.json"))["output"])')
ytb tts --json "$ZHSRT" > /tmp/tts.json 2>/dev/null
ytb audio-sync --json "$ID" > /tmp/as.json 2>/dev/null
echo "done: $(python3 -c 'import json;print(json.load(open("/tmp/as.json"))["output"])')"
```

### 混入其它 CLI 工具

`--json` 使每个步骤都只是"输入文件 → 输出文件/JSON"的纯函数式工具，
可与 ffmpeg、whisper、任意上传器自由组合：

```bash
ytb download --json "$URL" | jq -r .video | xargs ffprobe     # 下游接任意工具
ytb tts --json "$SRT" | jq -r .voice_dir                      # 取产物继续处理
```

## 在任意目录运行（relocatable）

二进制不依赖当前工作目录。项目资源（`skills/`、`.venv/`）按以下顺序定位
（`internal/pipeline/resource.go`）：

1. `$YTB2BILI_PROJECT_DIR`（显式指定）
2. 可执行文件所在目录（若含 `skills/`）
3. 当前工作目录（若含 `skills/`）
4. 源码回溯（`go run`/测试）

```bash
make install                              # 安装到 ~/.local/bin/ytb
export YTB2BILI_PROJECT_DIR=/path/to/repo # 或把 skills/ 与 .venv/ 放在二进制旁
ytb tts --json some.srt                   # 从任意目录调用
```

## 在 Agent 里作为工具封装

一个最小 Agent 工具定义（把 `--json` 命令包成可调用函数）：

```python
import json, subprocess

def ytb_step(name: str, *args: str) -> dict:
    proc = subprocess.run(
        ["ytb", name, "--json", *args],
        capture_output=True, text=True,
    )
    if proc.returncode != 0:
        raise RuntimeError(f"ytb {name} failed: {proc.stderr.strip()}")
    return json.loads(proc.stdout)
```

> 每个 step 都独立幂等、返回结构化结果，Agent 只需按字段串接，无需理解内部状态机。
