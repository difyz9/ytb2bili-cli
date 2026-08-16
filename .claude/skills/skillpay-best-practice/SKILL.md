---
name: skillpay-best-practice
description: Use when designing, pricing, or optimizing paid Skills on Tencent Cloud SkillHub (SkillPay) — covers pricing strategy, user experience, security, billing models, and monetization patterns for enterprise developers
---

# SkillHub SkillPay 最佳实践

## Overview

基于腾讯云 **SkillPay**（Agent 付费技能商业化平台）的实战经验总结。涵盖定价策略、用户体验设计、安全防护、计费模式选择和商业化运营建议。

企业开发者在 SkillHub 上架 Pay Skill 时，除了完成基础入驻发布流程外，还需要考虑如何：
- 合理定价（让用户愿意付、开发者有收益）
- 优化付费体验（不中断对话流）
- 保障支付安全（X402 协议 + AI 专属卡三道防线）
- 选择合适的计费模式（按次/订阅/阶梯）

---

## 零、X402 协议核心原理

X402 是**微信支付 Agent Pay 协议**，SkillPay 的底层支付引擎。理解 X402 有助于设计更好的付费体验。

### X402 核心设计原则

```
原则 1：Agent 不接触资金
  → Agent 代用户发起支付请求，但资金始终在微信支付体系内流转

原则 2：用户始终有控制权
  → 每笔支付需手机端确认（默认），用户可随时关闭授权

原则 3：安全隔离
  → AI 专属卡与微信主账户完全隔离，Agent 无法触碰主账户资金
```

### X402 授权层级

| 层级 | 说明 | 适用场景 | 安全性 |
|------|------|---------|--------|
| **per_payment** | 每次支付需手机确认 | 默认 | ⭐⭐⭐⭐⭐ |
| **session** | 一次会话内免确认 | 高频低价 Skill（¥0.10/次） | ⭐⭐⭐⭐ |
| **whitelist** | 指定 Skill 长期授权 | 订阅制服务 | ⭐⭐⭐ |

### X402 签名与防篡改

Agent 发起的每次 X402 支付请求包含以下信息，由 SkillHub 网关签名：

```
X402 请求载荷：
{
  "skill_id": "skill_xxx",        // SkillHub 认证的 Skill ID
  "amount": 50,                   // 金额（分）
  "user_token": "usr_xxx",        // 用户 AI 专属卡令牌
  "nonce": "rand_str",            // 防重放攻击
  "timestamp": 1234567890,        // 请求时间戳
  "signature": "sha256(..."       // SkillHub 签名
}
```

Skill 提供商**无需处理** X402 签名逻辑——SkillHub 网关自动完成。

### 与用户端的配合

X402 协议要求用户在微信中开通 **AI 专属卡**：

1. 微信 → 我 → 服务 → AI 专属卡
2. 充值余额（与微信零钱隔离）
3. 设置单笔/单日限额（默认 ¥200 / ¥2,000）

> **Skill 提供商应在用户首次付费时引导开卡，否则支付会失败。**

---

## 一、定价策略

### 1.1 定价参考框架

| Skill 类型 | 推荐定价 | 计费模式 | 理由 |
|-----------|---------|---------|------|
| 单次查询（如查天气、查汇率） | ¥0.10 - ¥0.50 | per_call | 低频低价值，薄利多销 |
| 内容生成（如图片、视频） | ¥0.50 - ¥5.00 | per_call | 按产出计费，价值感强 |
| 数据分析/工具类 | ¥1.00 - ¥10.00 | per_call | 专业价值，用户愿意付费 |
| 持续服务（如监控、日报） | ¥9.90 - ¥99.00/月 | subscription | 订阅制，稳定现金流 |
| API 代理类 | ¥0.01 - ¥0.10/次 | tiered | 量大从优，阶梯定价 |

### 1.2 定价原则

```
✅ 做：
- 从用户感知价值定价（用户觉得值多少钱）
- 设置试用次数降低决策门槛
- 批量购买提供折扣

❌ 不做：
- 按成本定价（用户不关心你的成本）
- 一次性定太高（Agent 付费还处于早期）
- 不设试用直接收费（用户拒绝率高）
```

### 1.3 试用策略

```yaml
# skill.yaml 试用配置建议
pay:
  trial:
    enabled: true
    calls: 3                 # 建议 3-5 次免费试用
```

**为什么要设试用：**
- Agent 调用场景中，用户无法像 App 一样预览 Skill 效果
- 3 次免费调用足够用户建立价值认知
- 免费转付费转化率通常为 **15%-30%**
- 试用结束后用户可以继续使用（弹出付费），不强制中断

---

## 二、计费模式选择

### 2.1 per_call（按次计费）

适合：一次性输出价值的工具类 Skill

```yaml
pay:
  billing:
    mode: per_call
    min_balance: 5.00      # 建议设 5-10 元，减少支付频次
```

**优点：** 用户每次付费感知清晰
**缺点：** 频繁小额支付体验割裂
**建议：** 配合 `min_balance` 预存减少支付弹窗

### 2.2 subscription（订阅制）

适合：持续提供价值的服务类 Skill

```yaml
pay:
  billing:
    mode: subscription
    interval: monthly
    interval_count: 1
    min_balance: 0
```

**优点：** 稳定收入、用户无感知续费
**缺点：** 用户付费门槛高（一次性付一个月）
**建议：** 配合免费试用 + 首月优惠

### 2.3 tiered（阶梯定价）

适合：用量差异大的 API / 平台类 Skill

```yaml
pay:
  billing:
    mode: tiered
    tiers:
      - up_to: 100           # 0-100 次
        price: 0.10          # 每次 ¥0.10
      - up_to: 1000          # 101-1000 次
        price: 0.05          # 每次 ¥0.05
      - up_to: infinity      # 1000+ 次
        price: 0.03          # 每次 ¥0.03
```

**优点：** 量大优惠，适合高频调用场景
**缺点：** 计费逻辑复杂，用户不易理解
**建议：** 仅在 API / 平台类 Skill 中使用

---

## 三、用户体验最佳实践

### 3.1 付费弹窗时机

```
❌ 差：用户刚输入就弹出支付
→ "请先付 ¥1.00 才能使用XX技能"

✅ 好：先展示部分价值再付费
→ "我已分析您的需求，需要付费 Skill 生成完整方案"
→ "本次调用需 ¥0.50，是否继续？"
```

**核心原则：** 先让用户感知价值，再引导付费。

### 3.2 Skill 交互设计

```markdown
# 好的 Skill 响应示例

用户: "帮我生成一份数据分析报告"

AI: "好的！我检测到「高级数据分析」Skill 可以完成这个任务。
该 Skill 每次调用收费 ¥2.00。

以下是免费摘要预览：
- 数据概览：共 1,234 条记录
- 关键指标：转化率 3.2%
- 趋势：近 7 天上升 15%

需要我用高级 Skill 生成完整分析报告吗？
（点击确认将扣除 ¥2.00 微信 AI 专属卡余额）"
```

### 3.3 X402 授权模式建议

根据 Skill 的使用频率和价值，在交互中引导用户选择适当的 X402 授权模式：

```
低频高价值（¥5+ 每次）→ 保持 per_payment（笔笔确认）
  → 用户预期内，每次确认不反感

高频低价（¥0.10~¥0.50 每次）→ 建议 session 模式
  → "您在使用高频 Skill，建议开启本次会话免确认，避免频繁弹窗"
  → 用户可在手机上选择「本次会话免确认」

订阅制（按月）→ 建议 whitelist 模式
  → 首次确认后，后续自动扣费
  → 用户可在 AI 专属卡管理页随时取消
```

```yaml
# skill.yaml 推荐配置
pay:
  x402:
    auth_mode: per_payment          # 默认笔笔确认
    # auth_mode: session            # 高频低价 Skill 可建议 session
    max_per_session: 20.00          # session 模式下会话上限
```

### 3.4 付费失败处理

```
场景：用户 AI 专属卡余额不足

❌ 差：
"支付失败，任务终止"

✅ 好：
"检测到您的 AI 专属卡余额不足（当前 ¥1.00，需要 ¥5.00）。
请通过微信为 AI 专属卡充值后重试。

💡 提示：单次充值 ¥10.00 以上可免去下次余额不足的烦恼。
"
```

### 3.4 退款策略

| 策略 | 适用场景 | 说明 |
|------|---------|------|
| `no_refund` | 即时交付类（生成结果已交付） | 不退款 |
| `manual` | 服务类（部分交付） | 人工审核退款 |
| `automatic` | 未交付/执行失败 | 自动退款 |

建议大部分 Skill 设为 `no_refund` + 失败自动退款：

```yaml
pay:
  refund:
    policy: no_refund     # 正常执行不退
  # SkillHub 会在执行失败时自动退款，无需额外配置
```

---

## 四、安全最佳实践

### 4.1 AI 专属卡安全机制

微信支付 AI 专属卡设有 **三道安全防线**：

| 防线 | 说明 | 建议 |
|------|------|------|
| 主账隔离 | AI 卡与微信主账户完全隔离 | 引导用户开设 AI 专属卡 |
| 余额自主 | 用户自行管理转入转出 | 不要尝试绕过余额管理 |
| 笔笔确认 | 每笔支付需用户在手机端确认 | 确保支付流程有确认步骤 |

### 4.2 Skill 内容安全

上架前需通过以下审核，不通过会驳回：

```yaml
# skill.yaml 安全声明
security:
  content_review: true          # ✅ 内容合规
  vulnerability_scan: true      # ✅ 漏洞扫描通过
  ai_safety_assessment: true    # ✅ AI 安全评估通过
```

**常见驳回原因：**
- ❌ Skill 输出包含未过滤的用户输入 → 添加输入验证和清洗
- ❌ Skill 调用了未鉴权的外部 API → 添加 API key / OAuth
- ❌ Skill 可能会泄露敏感信息 → 添加输出过滤
- ❌ 未正确处理错误状态 → 添加错误处理逻辑

### 4.3 X402 协议安全注意事项

| 注意点 | 说明 | 建议 |
|--------|------|------|
| **签名验证** | X402 请求由 SkillHub 网关签名，Skill 无需自行签名 | 不要自行构造 X402 请求 |
| **金额篡改** | X402 请求中的 amount 由 SkillHub 根据 skill.yaml 定价生成 | 无法篡改，Skill 侧无需校验 |
| **重放攻击** | X402 请求包含 nonce + timestamp | SkillHub 自动防护 |
| **用户取消** | 用户可在手机上随时取消授权 | 优雅处理取消场景（不要反复弹窗） |
| **超额拦截** | 超过 AI 专属卡限额时支付自动失败 | 给用户友好的充值引导 |

**关键安全原则：** Skill 提供商只需要关注 Skill 本身的质量和安全，X402 协议层的安全由 SkillHub + 微信支付保障。

### 4.4 调用鉴权

如果 Pay Skill 需要调用外部 API：

```yaml
auth:
  type: api_key              # 推荐 api_key 或 oauth
  endpoint: https://api.example.com/auth
  # SkillHub 会在调用前自动完成鉴权
```

---

## 五、商业化运营

### 5.1 冷启动建议

| 阶段 | 目标 | 动作 |
|------|------|------|
| 第 1 周 | 验证付费意愿 | 免费试用 5 次，收集反馈 |
| 第 2-4 周 | 优化定价 | 根据调用数据调整价格 ±30% |
| 第 1-3 月 | 获取种子用户 | 限时折扣、推荐返利 |
| 稳定期 | 规模增长 | 订阅制 + 阶梯定价 |

### 5.2 数据分析关注指标

```bash
# == 全平台通用 ==
# 查看调用统计
skillhub analytics calls --skill-id <SKILL_ID>

# 查看收入
skillhub pay revenue

# 查看转化率
skillhub analytics conversion --skill-id <SKILL_ID>

# 查看用户留存
skillhub analytics retention --skill-id <SKILL_ID>
```

**关键指标：**
| 指标 | 健康值 | 说明 |
|------|--------|------|
| 试用→付费转化率 | >15% | 低于 10% 需检查定价或体验 |
| 付费后复购率 | >40% | 低于 20% 可能是价值不足 |
| 支付成功率 | >95% | 低于 90% 需检查支付流程 |
| 用户投诉率 | <1% | 高于 2% 需检查 Skill 质量 |

### 5.3 更新迭代

```bash
# 更新 Skill 版本（价格不变）
skillhub publish ./my-skill-v2.pkg \
  --skill-id <SKILL_ID> \
  --version "1.1.0"

# 调整价格（需重新审核）
skillhub listing update <LISTING_ID> --price "0.80"

# 下架 Skill
skillhub listing unpublish <LISTING_ID>
```

---

## 六、常见陷阱

### ❌ 定价过高
Agent 付费还处于早期，用户对付费 AI Skill 的价格敏感度较高。建议从低价起步，根据数据逐步上探。

### ❌ 不设试用
用户无法在付费前评估 Skill 质量。至少设 3 次免费试用。

### ❌ 计费模式选错
一次性价值高（如生成图片）→ per_call
持续服务（如日报）→ subscription
两极化用量 → tiered

### ❌ 忽视失败处理
Agent 调用失败时如果不优雅处理，用户流失率极高。确保失败时自动退款并给出友好提示。

### ❌ 安全审核不通过
上架前先自查：内容合规、无漏洞、AI 行为安全。避免反复提交浪费审核周期。

---

## 跨平台 Quick Reference

| 操作 | 命令 | 平台 |
|------|------|------|
| 安装 CLI | `npm i -g @skillhub/cli` | 全平台 |
| 查看分析 | `skillhub analytics calls --skill-id <ID>` | 全平台 |
| 查看收入 | `skillhub pay revenue` | 全平台 |
| 调整价格 | `skillhub listing update <ID> --price "0.80"` | 全平台 |
| 下架 | `skillhub listing unpublish <ID>` | 全平台 |
| 更新版本 | `skillhub publish ./v2.pkg --skill-id <ID>` | 全平台 |

## Related Skills

- [[skillpay-publish-skill]] — 发布付费 Skill 的完整流程
