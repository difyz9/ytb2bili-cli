# 环境变量与配置

`config.yaml` 及各类凭证（cookies / OAuth client）均**不入库**；首次配置从 `configs/config.example.yaml` 复制脱敏模板。配置查找顺序：`--config <path>` → `$YTB2BILI_CONFIG` → 当前目录 `./config.yaml`，找不到时使用内置默认配置。

## 配置文件段

| 段 / 字段 | 说明 |
|-----------|------|
| `llm_api_key` / `llm_base_url` / `llm_model` | DeepSeek（或任意 OpenAI 兼容）LLM：翻译 + 元数据 |
| `data_dir` / `download_dir` | 数据根目录 / 下载目录 |
| `skills_dir` | 技能资源目录（空 = 自动探测项目根 `skills/`；IndexTTS 脚本位置变化时可填绝对路径） |
| `min_duration_sec` | 入队时长下限（默认 240s，过滤 Short 短视频） |
| `bili_tid` | B站投稿默认分区 id |
| `translation_target_lang` | 翻译目标语言（默认 `zh-Hans`） |
| `chrome_debug_port` | cookies refresh 用 Chrome 调试起始端口 |
| `tencent_cloud` | 腾讯云 SecretId/Key/Region（TTS 与翻译降级共用） |
| `translation` | 翻译服务：`primary`（deepseek/baidu/tencent）+ `fallbacks`（降级顺序）+ `retries`；各服务商密钥/模型/批大小见模板 |
| `youtube_oauth` | Google OAuth Client ID/Secret（频道订阅导入用） |
| `tts` | 配音：`provider` = `index`（IndexTTS2 本地，默认）/ `tencent`；腾讯云音色/音量/语速参数；`index` 段含服务地址、鉴权 key、情感（`emotion`/`emotion_alpha`）、克隆音色 `ref_audio`、并发等 |
| `concurrent` | 并发请求参数（worker/限速/批大小） |
| `transcriber` | 转录后端：`provider` = `whisper`（默认，本地 whisper.cpp）/ `bcut`（云 ASR）；whisper 模型路径/线程数 |
| `accounts` | 多账号路由列表：`name` + `type_rule`（稿件标题/标签关键词）+ `is_default`（兜底账号） |
| `search` | **自主调度关键词单一来源**（auto/daemon 共用）：`keywords`、`scorer`、`upload_date`、`max_duration`、`max_videos`、`min_views` |
| `daemon` | 守护进程参数：批间隔/最大批数/每批消费上限/重试上限/步骤超时/心跳文件/飞书告警 webhook |

> **改关键词/调度 = 改 `config.yaml` 的 `search:` / `daemon:` 段后重启服务**，不要再改任何脚本。

## 环境变量

| 变量 | 用途 |
|------|------|
| `DEEPSEEK_API_KEY` | DeepSeek API Key（LLM 翻译/元数据），配置文件留空时必填 |
| `YTB2BILI_CONFIG` | 配置文件路径（默认 `./config.yaml`） |
| `YOUTUBE_COOKIES` | YouTube cookies 文件路径（防下载频率限制），也可用 `ytb cookies refresh` |
| `YOUTUBE_PROXY` | YouTube 下载专用代理：`socks5://user:pass@host:port` 或 `http://...`，只走 yt-dlp（不影响 B站投稿/翻译）；也可配 config 的 `youtube_proxy`（环境变量优先） |
| `LLM_MODEL` / `LLM_BASE_URL` | 覆盖默认 LLM 模型与接入点（默认 deepseek-v4-flash） |
| `TRANSLATION_PRIMARY` | 覆盖翻译主服务商（deepseek/tencent/baidu/ollama） |
| `TRANSLATION_FALLBACKS` | 覆盖降级顺序（逗号分隔，如 `tencent,ollama`） |
| `TRANSLATION_RETRIES` | 覆盖主服务重试次数 |
| `OLLAMA_BASE_URL` / `OLLAMA_MODEL` | 本地 Ollama 地址与模型（翻译兜底） |
| `TENCENTCLOUD_SECRET_ID` / `TENCENTCLOUD_SECRET_KEY` | 腾讯云密钥（TTS/翻译，或写入 config） |
| `INDEX_TTS_API_KEY` | IndexTTS2 服务鉴权 key（config `tts.index.api_key` 可引用 `${INDEX_TTS_API_KEY}`） |
| `YTB2BILI_PROJECT_DIR` | 显式指定项目根（含 `skills/`，非项目根运行 CLI 时用） |
| `YTB2BILI_PYTHON` | 指定 .venv Python 解释器 |
| `YTB2BILI_AUDIO_SYNC_SCRIPT` | 指定音画同步脚本（最细粒度覆盖） |
| `YTB2BILI_SERVER_TOKEN` | HTTP API 对外监听时要求的 Bearer Token |
| `YTB2BILI_ALLOWED_ORIGINS` | HTTP API 浏览器扩展允许的跨域来源 |

## cookies 管理

```bash
ytb cookies test       # 测试 YouTube cookies 是否有效（自动选 data/cookies/ 最新文件，先剔除已轮换的 PSIDTS 令牌）
ytb cookies refresh    # 从 Chrome 刷新 cookies（Chrome 不在运行则自动拉起；daemon 也每 6h 自动刷，可配 daemon.cookies_refresh_hours）
```

凭证目录约定：`data/cookies/`（YouTube cookies；默认自动选目录下最新有效 `*.txt`，显式 `youtube_cookies:` 配置优先）/ `client_tv.json` / `client_web.apps.googleusercontent.com.json`。
