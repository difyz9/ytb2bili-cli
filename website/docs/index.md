---
pageType: home

hero:
  name: ytb2bili-go
  text: YouTube → Bilibili 视频搬运工具
  tagline: Go 编写的 CLI（ytb）：搜索 · 下载 · 转录 · 翻译 · 配音 · 音画同步 · 投稿 · 字幕上传，一站式全自动流水线
  actions:
    - theme: brand
      text: 快速开始
      link: /guide/quick-start
    - theme: alt
      text: 项目简介
      link: /guide/introduction
    - theme: alt
      text: GitHub
      link: https://github.com/zolagz/ytb2bili-go
features:
  - title: 🎬 全流水线
    details: 搜索 → 下载 → 转录 → 翻译 → TTS 配音 → 音画同步 → AI 元数据 → 投稿 → 审核通过自动上传字幕，产物幂等续跑
    link: /guide/introduction
  - title: 🤖 自主调度
    details: auto 多维评分 + daemon 常驻守护进程：失败自动重试、步骤超时 kill、心跳与飞书告警，systemd 托管
    link: /guide/automation
  - title: 🧠 AI 驱动
    details: DeepSeek LLM 批量翻译（多服务商降级）、自动生成中文标题/简介/标签；本地 Ollama 可兜底
    link: /guide/configuration
  - title: 🔊 中文配音
    details: IndexTTS2 本地服务（音色克隆/情感控制）或腾讯云 TTS 分段合成，滚动字幕清理 + 语速优化合成成片
    link: /guide/introduction
  - title: 📡 频道监控
    details: RSS 免 OAuth 起步 / YouTube OAuth 导入订阅，自动入队闭环；ytsubs 式基线评分捕捉起势视频
    link: /guide/channel
  - title: ⚙️ 可组合 CLI
    details: 每个流水线步骤都是独立命令，支持 --json 机器可读输出，可被外部 pipeline / AI Agent 自由串联
    link: /guide/usage
---
