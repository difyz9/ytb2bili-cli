# 功能分析：social-auto-upload → Golang 迁移

## 目标
全面分析 social-auto-upload 项目的所有功能、模块和依赖，输出一份完整的功能清单和迁移对照表。

## 背景
- **源项目**: social-auto-upload (Python) — 多平台视频/图文自动上传工具
- **目标项目**: ytb2bili-cli (Go) — 已有 chromedp 浏览器自动化、CLI框架、config系统
- **需求方要求**: 将此项目的功能迁移到 Golang 实现，然后集成到 ytb2bili-cli 项目中

## 需要分析的内容

### 1. 支持的平台及能力
列出每个平台支持的操作类型，标记：
- 视频上传 (单视频/批量)
- 图文/笔记上传
- 定时发布
- 登录方式 (Cookie/QRCode/交互式)
- Cookie 管理 (登录/校验/续期)

### 2. 核心架构
- CLI 入口和参数结构 (sau_cli.py)
- 各 uploader 的公共基类和差异化逻辑
- utils 工具模块 (网络请求、日志、二维码、文件处理)
- myUtils 模块 (auth、login、postVideo)
- 配置文件 (conf.py)
- cookie 存储路径和格式

### 3. 外部依赖映射
- patchright/playwright → chromedp (Go)
- requests → net/http (Go)
- qrcode → go-qrcode (已存在于 ytb2bili-cli)
- opencv-python → ?
- loguru → log/slog / zap (Go)
- segno → go-qrcode

### 4. 关键业务流程
- 登录流程 (每种平台的登录方式)
- 上传流程 (每种平台的页面操作步骤)
- cookie 校验流程
- 定时发布实现方式

### 5. 优先级建议
基于技术难度和业务价值，给出平台迁移优先级排序

## 产出物
1. 完整的功能清单 (平台 × 能力 矩阵)
2. 依赖映射表 (Python → Go)
3. 迁移难度评估 (每个模块：简单/中等/困难)
4. 推荐的迁移阶段划分
