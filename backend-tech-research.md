# 技术调研：social-auto-upload → Golang 架构方案

## 目标
分析从 Python 迁移到 Golang 的技术可行性，提出架构方案、依赖选择和预估工作量。

## 背景
- **源项目**: social-auto-upload (Python) — 使用 patchright/playwright 进行浏览器自动化实现多平台上传
- **目标项目**: ytb2bili-cli (Go) — 已集成 chromedp、cobra CLI、config系统、bilibili SDK

## 需要调研的内容

### 1. 浏览器自动化方案
- `patchright` (Python) → Go 中有哪些选择？
  - **chromedp** (已在 ytb2bili-cli 项目中) — 纯 Go，支持 CDP 协议
  - 能否用 chromedp 替代所有 patchright 的能力？
  - 特别关注：页面元素选择、表单填写、文件上传 input、等待元素出现、截图、Cookie 注入
  - 对比各 uploader 中用到的 patchright 功能，逐一验证 chromedp 能否实现

### 2. 各平台 Uploader 分析
逐一阅读各 uploader 的 main.py 文件，评估迁移难度：
- **Douyin** (douyin_uploader/main.py)
- **Kuaishou** (ks_uploader/main.py)
- **Xiaohongshu** (xhs_uploader/main.py, xiaohongshu_uploader/main.py)
- **Bilibili** (bilibili_uploader/runtime.py) — 当前调用 biliup CLI
- **Tencent** (tencent_uploader/main.py)
- **YouTube** (youtube_uploader/main.py)
- **TikTok** (tk_uploader/main.py)
- **Baidu Baijiahao** (baijiahao_uploader/main.py)

### 3. 与 ytb2bili-cli 集成方案
- 如何作为子命令嵌入现有 CLI？(cobra command)
- 配置文件复用：现有 config.yaml 能否扩展？
- 共享模块：chromedp、cookie 管理、日志
- 模块化架构建议：是否新建 internal/social/ 包存放各平台上传器？

### 4. 技术难点识别
- 文件上传 (multipart/form-data) 在 Go 中的处理
- 定时任务调度 (Go 的定时器 vs Python asyncio)
- 浏览器 cookie 格式兼容
- 跨平台兼容性

### 5. 工作量估算
- 每个平台的大致代码量
- 总工期建议
- 推荐并行策略 (哪些平台可以同时开发)

## 产出物
1. 技术可行性结论
2. 推荐架构图 (包结构、模块关系)
3. 每个平台的 chromedp 替代方案说明
4. 风险点和难点清单
5. 工作量估算
