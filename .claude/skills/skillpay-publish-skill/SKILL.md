---
name: skillpay-publish-skill
description: Use when publishing a paid Skill on Tencent Cloud SkillHub (SkillPay) — covers enterprise onboarding, Skill packaging, pricing setup, and publishing for monetizing AI agent skills across Claude Code, Codex, WorkBuddy, and other AI tools
---

# SkillHub SkillPay 发布付费 Skill

## Overview

腾讯云 **SkillHub** 于 2026 年 7 月正式上线了 **SkillPay**（Agent 付费技能商业化平台），打通技能分发、Agent 调用与技能支付三端。

通过本 skill，企业开发者可将已有 Skill 封装为 **Pay Skill** 上架售卖，用户通过 WorkBuddy / QClaw 等工具调用时自动弹出付费授权，全程不跳出对话。

**核心优势：**
- 不需要重写业务代码、不需要自建支付链路
- 定价权和收款权完全归商家
- 按调用量结算
- **不对企业规模设置入驻门槛**，中小商家均可自助完成

---

## X402 协议（Agent Pay 底层支付协议）

> 整个 SkillPay 的实现基于 **微信支付 Agent Pay X402 协议**。

### 什么是 X402

X402 是微信支付为 AI Agent 场景设计的专用支付协议。它解决了传统支付流程在 Agent 场景下的核心矛盾：
**Agent 需要代用户完成小额支付，但不能持有用户的完整支付凭证。**

### X402 授权模型

```
用户 → 授权（X402协议）→ Agent → 代扣（单次/限额）→ 商户收款
         ↑                              ↑
   微信支付 AI 专属卡              笔笔手机端确认
```

| 角色 | 说明 |
|------|------|
| **用户** | 拥有 AI 专属卡，卡内余额与微信主账户隔离 |
| **Agent** | 通过 X402 协议获取**一次性**或**会话级**支付授权 |
| **SkillHub** | 负责认证、鉴权、调用链路可信 |
| **微信支付** | X402 协议执行方，处理实际资金划转 |
| **商户** | Pay Skill 提供方，收到扣除平台费用后的款项 |

### X402 支付流程（时序）

```
用户触发 Pay Skill
    │
    ▼
Agent 检测到 Skill 需付费
    │
    ├─ X402 协议发起支付请求
    │     → 携带：skill_id, amount, 用户标识
    │
    ▼
微信支付 → 校验 AI 专属卡余额
    │
    ├─ 余额充足 → 推送手机端确认（笔笔确认）
    │     → 用户在手机上点「确认」
    │     → X402 返回支付令牌
    │
    ▼
SkillHub 收到支付确认 → 执行 Skill
    │
    ▼
Skill 执行完成 → 返回结果给用户
    │
    ▼
X402 完成资金结算（商户账户+平台分账）
```

### X402 关键特性

| 特性 | 说明 |
|------|------|
| **代付机制** | Agent 可代用户请求支付，但**不接触资金** |
| **一次性授权** | 每次支付需用户手机端确认（默认） |
| **会话级授权** | 用户可设置「本次会话免确认」（小额场景） |
| **金额上限** | 单笔最高 ¥200，单日累计 ¥2,000（AI 专属卡规则） |
| **主账隔离** | AI 专属卡与微信主账户完全隔离，资金互不影响 |
| **笔笔确认** | 每笔支付推送手机端，用户确认后才扣款 |

### X402 在 SkillHub 中的角色

```
         SkillHub（认证 + 分发 + 调用链路）
         ┌─────────────────────────────────┐
         │   Skill A  │   Skill B  │   Skill C
         │  (X402)    │  (X402)    │  (普通)
         └────┬───────┴────┬───────┴──────────┘
              │            │
         ┌────▼────────────▼────┐
         │   微信支付 X402 网关    │
         │   AI 专属卡余额管理     │
         │   笔笔确认推送         │
         └─────────────────────┘
```

SkillHub 对接 X402 协议后：来源认证、内容完整性校验、可信调用入口由 SkillHub 负责；实际资金安全、授权确认、结算清分由微信支付 X402 协议完成。

---

## 适用工具

| 工具 | 说明 |
|------|------|
| Claude Code | 通过 `npx skills add` 安装 SkillHub Pay Skill |
| Codex | 支持 SkillHub 协议，可调用 Pay Skill |
| WorkBuddy | 内置 SkillHub 集成，可安装和调用 Pay Skill |
| QClaw | 支持 SkillHub CLI 安装和调用 |
| Cursor | 支持 `npx skills add` 加载 Pay Skill |

---

## Phase 1: 企业认证与入驻

### 1.1 前置条件

| 项目 | 说明 |
|------|------|
| 企业资质 | 有效的营业执照 / 企业认证 |
| 微信支付商户号 | 已完成微信支付商户开通 |
| SkillHub 账号 | 已完成企业注册（skillhub.cn） |
| 待发布的 Skill | 已完成开发和本地测试的 Skill |

### 1.2 入驻流程

```bash
# 1. 登录 SkillHub 控制台
# https://skillhub.cn/console

# 2. 进入「SkillPay」>「入驻申请」
#  - 提交企业认证材料
#  - 绑定微信支付商户号
#  - 签署 SkillPay 服务协议

# 3. 等待审核（通常 1-3 个工作日）
```

### 1.3 验证入驻状态

```bash
# == 全平台通用（如已安装 SkillHub CLI）==
# 查看商户状态
skillhub merchant status

# 查看签约信息
skillhub merchant info

# 验证绑定商户号
skillhub merchant account
```

---

## Phase 2: 封装 Pay Skill

### 2.1 Skill 改造要求

Pay Skill = 普通 Skill + 价格配置 + 计费模型。

```yaml
# example: skill.yaml — Pay Skill 配置文件
name: my-paid-skill
description: "付费技能描述"
version: "1.0.0"

# ====== SkillPay 配置 ======
pay:
  enabled: true
  price:
    amount: 0.50          # 单次调用价格（元）
    currency: CNY
  billing:
    mode: per_call        # 计费模式：per_call / subscription
    min_balance: 5.00     # 用户最低预存余额（元）
  refund:
    policy: no_refund     # 退款策略
  trial:
    enabled: false        # 是否允许免费试用
    calls: 0              # 试用次数

# ====== 原有 Skill 配置 ======
triggers:
  - keyword: "帮我处理XXX"
    match: fuzzy

actions:
  - type: script
    path: ./handler.py
```

### 2.2 计费模式选择

| 模式 | 适用场景 | 说明 |
|------|---------|------|
| `per_call` | 按次计费 | 每调用一次扣费一次，适合工具类 Skill |
| `subscription` | 订阅制 | 按周期扣费，适合持续服务类 Skill |
| `tiered` | 阶梯定价 | 不同用量区间不同单价，适合 API 类服务 |

### 2.3 Pay Skill 规范

```yaml
# skill.yaml 完整规范
name: <技能名称>                # 必填
description: <技能描述>         # 必填，展示给用户的说明
version: <语义化版本号>          # 必填

pay:
  enabled: true                 # 必填，开启付费
  price:
    amount: <价格>              # 必填，单次/周期价格（元）
    currency: CNY               # 必填，币种
  billing:
    mode: per_call | subscription  # 必填
    interval: monthly           # subscription 模式下的周期
    interval_count: 1           # 每个周期单位数
  refund:
    policy: no_refund | manual | automatic
  trial:
    enabled: true | false
    calls: <试用次数>            # 免费试用次数

# 内容安全（必填，上架前审核用）
security:
  content_review: true          # 已通过内容合规检测
  vulnerability_scan: true      # 已通过漏洞扫描
  ai_safety_assessment: true    # 已通过 AI 模型安全评估

# 调用鉴权（可选，如 Skill 需要调用外部 API）
auth:
  type: none | api_key | oauth
  endpoint: <鉴权地址>
```

---

## Phase 3: 通过 CLI 发布（publish-via-cli 工作流）

> 以下流程来自 SkillHub 官方 CLI 发布教程。推荐开发者使用命令行完成从初始化到上架的全流程。

### 3.0 安装 CLI

```bash
# 方式 1：一键安装脚本（推荐）
curl -fsSL https://skillhub.cn/install/install.sh | bash

# 方式 2：npm 全局安装
npm install -g skillhub-cli

# 验证
skillhub -h
```

### 3.1 初始化项目

```bash
# 在技能目录中执行初始化
cd ./my-skill-dir

skillhub init --name "<技能名称>" --category "<分类>"
```

**常用分类参考：** `工具`、`数据分析`、`内容生成`、`图像处理`、`开发`、`生活`、`教育`

`skillhub init` 会生成标准项目结构：

```
my-skill-dir/
├── SKILL.md          # 技能描述文件（必需，YAML frontmatter + Markdown 正文）
├── README.md         # 技能说明文档（可选）
└── assets/           # 资源文件夹（可选，存放图片、示例等）
```

### 3.2 SKILL.md 必需格式

SKILL.md 是发布的核心文件。必须包含 **YAML frontmatter**（元数据）和 **Markdown 正文**：

```markdown
---
name: my-skill          # 技能名称，创建后不可修改
display_name: "我的技能"  # 显示名称
description: 技能描述     # 简短介绍
version: "1.0.0"        # 语义化版本号
category: 工具            # 分类
platforms:               # 支持平台
  - workbuddy
  - qclaw
  - claude-code
author:
  name: 作者名
  email: author@example.com
---

# 技能正文

技能的功能说明、使用方法、示例等。
```

如果是 Pay Skill，需要在 `SKILL.md` 的 frontmatter 中添加付费字段，或在同目录下放 `skill.yaml`（推荐）：

```yaml
# skill.yaml（与 SKILL.md 同目录）
pay:
  enabled: true
  price:
    amount: 0.50
    currency: CNY
  billing:
    mode: per_call
  trial:
    enabled: true
    calls: 3
  x402:
    auth_mode: per_payment
```

### 3.3 推送草稿

> 推荐工作流：`init` → `push` → `publish`（三步分离，可多次修改再提交）

```bash
# === 全平台通用 ===

# 登录认证（首次需要）
skillhub login
# 支持浏览器 OAuth 登录，或用 --token 参数：
# skillhub login --token sk_xxx

# 推送技能文件到 SkillHub（草稿状态，不会上架）
skillhub push
```

`push` 将本地文件推送至 SkillHub 保存为**草稿**。可多次推送、反复修改，不会影响线上版本。

### 3.4 发布上线

```bash
# 确认推送完成后，提交发布申请
skillhub publish
```

`publish` 执行后：
- 触发 **三线并行安全审核**（内容合规 + 漏洞扫描 + AI 安全评估）
- 审核通过 → 技能自动上架
- 审核驳回 → 根据反馈修改后重新 `push` + `publish`

### 3.5 快捷一命令发布

对于已准备好的技能目录，可直接发布（跳过 init/push 两步）：

```bash
# 直接发布目录（自动打包上传 + 提交审核）
skillhub publish ./my-skill-dir --namespace <你的命名空间>

# 发布 zip 包
skillhub publish ./my-skill.zip --namespace <你的命名空间>

# 设置可见性
skillhub publish ./my-skill-dir \
  --namespace <命名空间> \
  --visibility public
```

**可见性选项：**
| 值 | 说明 |
|----|------|
| `public`（默认） | 所有人可见、可安装 |
| `namespace-only` | 仅命名空间成员可见 |
| `private` | 仅自己可见 |

### 3.6 通过控制台上传

```
https://skillhub.cn/console/skills/publish

1. 点击「上传新 Skill」
2. 选择 SKILL.md + skill.yaml（或打包的 zip）
3. 填写价格和计费模式
4. 提交审核
```

### 3.7 审核流程

上架前 SkillHub 会执行以下审核：

| 审核项 | 说明 | 周期 |
|--------|------|------|
| 内容合规过滤 | 检查 Skill 内容是否合规 | 自动 |
| 科恩实验室漏洞扫描 | 代码安全扫描 | 1-2 小时 |
| 云鼎实验室 AI 安全评估 | AI 模型行为安全 | 1-2 工作日 |

### 3.8 发布后验证

```bash
# 查看上架状态
skillhub publish status <PUBLISH_ID>
# 或查看所有已发布技能
skillhub list

# 搜索验证
skillhub search <技能名称>

# 在 SkillHub 商店中浏览
skillhub explore

# 查看交易记录（仅 Pay Skill）
skillhub pay transactions
```

---

## Phase 4: 用户端体验（安装与调用）

用户通过以下方式安装和调用 Pay Skill：

```bash
# == WorkBuddy / QClaw ==
skillhub install <your-skill-name>

# == Claude Code / Cursor ==
npx skills add <your-skill-name>
```

### 支付调用流程（X402 协议驱动）

```
用户触发 Skill
    │
    ▼
Agent 调用 SkillHub → X402 鉴权 → 校验余额
    │
    ▼
微信推送手机端确认 → 用户确认
    │
    ▼
X402 返回支付凭据 → SkillHub 执行 Skill
    │
    ▼
结果返回用户 + 资金结算
```

**全程不跳出对话，不中断对话流。**

### X402 授权模式配置

Skill 可以建议用户的授权模式（通过 `skill.yaml`）：

```yaml
pay:
  x402:
    auth_mode: per_payment     # 每次支付都确认（默认，最安全）
    # auth_mode: session       # 会话级授权（本次对话免确认）
    max_per_session: 20.00     # session 模式下的单会话上限（元）
```

> **安全建议：** 默认使用 `per_payment` 模式（笔笔确认）。对于高频低价的 Skill（如每次 ¥0.10），可建议用户开启 `session` 模式减少确认频次。

---

## 跨平台 Quick Reference

| 步骤 | 命令 | 说明 |
|------|------|------|
| 安装 CLI | `curl -fsSL https://skillhub.cn/install/install.sh \| bash` | 全平台 |
| 登录 | `skillhub login` 或 `skillhub login --token sk_xxx` | 全平台一致 |
| 初始化 | `skillhub init --name "名称" --category "分类"` | 生成 SKILL.md 骨架 |
| 推送草稿 | `skillhub push` | 可多次推送反复修改 |
| 发布上线 | `skillhub publish` | 触发三线安全审核 |
| 一键发布 | `skillhub publish ./dir --namespace <ns>` | 跳过 init/push |
| 设置可见性 | 加 `--visibility public\|namespace-only\|private` | 默认 public |
| 查看状态 | `skillhub publish status <ID>` | 全平台一致 |
| 搜索验证 | `skillhub search <名称>` | 全平台一致 |

---

## Common Issues

| 问题 | 原因 | 解决 |
|------|------|------|
| `认证失败` | 企业认证未完成或商户号未绑定 | 检查 SkillHub 控制台认证状态 |
| `发布失败：安全审核未通过` | 漏洞扫描或内容合规未通过 | 查看审核报告，修复后重新提交 |
| `用户无法安装` | Skill 尚未上架或搜索不到 | 确认 `skillhub listing status` 为 `published` |
| `支付弹出失败` | 用户未开通 AI 专属卡 | 引导用户在微信开通 AI 专属卡 |
| `价格显示异常` | `skill.yaml` 中价格格式错误 | 确认 `amount` 为数字，`currency` 为 CNY |

## Related Skills

- [[skillpay-best-practice]] — Agent Pay 最佳实践
- [[multica-create-team]] — 如需要先搭建多智能体团队
