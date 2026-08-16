---
name: pay-doc-analyzer
description: Use when building or testing a Pay Skill with X402 payment integration — complete reference including Python merchant backend, payment flow simulation, SkillHub publish config, and local-to-production testing
---

# 付费文档分析 — Pay Skill 完整实现

## Overview

这是一个**完整的支付集成 Skill 案例**，演示了如何将一个付费服务封装为 SkillHub Pay Skill，包含：

- **商户后端** （Python）— 处理 402 支付触发、订单管理、履约交付
- **Handler 脚本** — SkillHub 调用的业务入口，连接商户后端
- **Mock 测试** — 不依赖微信支付即可本地测试完整支付流程
- **Production 发布** — 对接真实 X402 协议，发布到 SkillHub

### 价值

```
开发完成这个案例后，你将掌握：
✅ 如何搭建 Pay Skill 的商户后端
✅ 402 + WeixinPay-Required 协议的完整实现
✅ 支付 → 履约 → 退款的全链路处理
✅ 幂等控制（防重复扣费）
✅ Mock 模式本地测试
✅ 发布到 SkillHub 生产环境
```

---

## 架构

```
用户 / AI Agent
    │
    ├── ① 触发 skill（"分析文档 xxx"）
    │
    ▼
┌──────────────────────────────────────────┐
│  handler.py (SkillHub 调用入口)           │
│  模式 A: 调用商户后端 API                  │
│  模式 B: 嵌入 merchant_server.py          │
└────────────────┬─────────────────────────┘
                 │
    ┌────────────▼────────────┐
    │  merchant_server.py     │  ← 你运行的服务
    │  ─────────────────────  │
    │  POST /api/resource     │
    │  ├─ 首次 → 402          │
    │  │  + WeixinPay-Required │
    │  │  + X-Out-Trade-No    │
    │  │                      │
    │  └─ 重试 → 验证 → 履约  │
    │      ├─ SUCCESS → 内容  │
    │      ├─ NOT_PAID → 402  │
    │      └─ FAIL → REFUND   │
    └─────────────────────────┘
```

---

## 快速上手

### 1. 启动商户后端（Mock 模式）

```bash
# 进入 skill 目录
cd pay-doc-analyzer

# 启动商户后端（纯 Python，无需任何外部依赖）
python3 scripts/merchant_server.py

# 输出：
#   商户后端 | 模式:MOCK | :8080
```

### 2. 测试支付流程

打开另一个终端，用 curl 模拟 Agent 的完整调用过程：

```bash
# ====== 第 1 步：首次请求，应返回 402 ======
FIRST=$(curl -s -D - -X POST http://localhost:8080/api/resource \
  -H "Content-Type: application/json" \
  -d '{"query": "/path/to/report.epub"}')

echo "$FIRST"
# 输出示范:
#   HTTP/1.1 402 Payment Required
#   WeixinPay-Required: MOCK_PAY_ABCDEF1234567890
#   X-Out-Trade-No: PAY_20260723_123456789abc
#   {
#     "code": "PAYMENT_REQUIRED",
#     "WeixinPay": { "WeixinPay-Required": "MOCK_PAY_..." }
#   }

# 提取订单号（供下一步使用）
OUT_TRADE_NO=$(echo "$FIRST" | grep -o '"out_trade_no":"[^"]*"' | cut -d'"' -f4)
echo "订单号: $OUT_TRADE_NO"
```

```bash
# ====== 第 2 步：模拟支付后重试 ======
# （Mock 模式下自动视为已支付）
curl -s -X POST http://localhost:8080/api/resource \
  -H "Content-Type: application/json" \
  -H "X-Out-Trade-No: $OUT_TRADE_NO" \
  -d '{"query": "/path/to/report.epub"}'

# 输出示范:
# {
#   "code": "SUCCESS",
#   "content": "【付费分析报告】\n查询: /path/to/report.epub\n...",
#   "already_fulfilled": false
# }
```

```bash
# ====== 第 3 步：测试幂等（重复请求应返回缓存） ======
curl -s -X POST http://localhost:8080/api/resource \
  -H "Content-Type: application/json" \
  -H "X-Out-Trade-No: $OUT_TRADE_NO" \
  -d '{"query": "/path/to/report.epub"}' | python3 -c "import sys,json; d=json.load(sys.stdin); print('幂等:', d['already_fulfilled'])"

# 输出: 幂等: True
```

```bash
# ====== 第 4 步：测试退款（业务失败场景） ======
FIRST_REFUND=$(curl -s -D - -X POST http://localhost:8080/api/resource \
  -H "Content-Type: application/json" \
  -d '{"query": "trigger_refund"}')
OUT_REFUND=$(echo "$FIRST_REFUND" | grep -o '"out_trade_no":"[^"]*"' | cut -d'"' -f4)

curl -s -X POST http://localhost:8080/api/resource \
  -H "Content-Type: application/json" \
  -H "X-Out-Trade-No: $OUT_REFUND" \
  -d '{"query": "trigger_refund"}'

# 输出:
# {
#   "code": "REFUNDED",
#   "message": "服务异常，已自动退款: 业务执行失败（模拟）"
# }
```

---

## 集成到 epub-to-txt

将 epub-to-txt 的转换能力作为付费业务注入商户后端：

```bash
# 一键启动：商户后端 + epub_to_txt 业务逻辑
python3 -c "
from merchant_server import set_business_handler, main as ms_main
import sys
sys.path.insert(0, 'scripts')
from epub_to_txt import epub_to_txt

def epub_business(query):
    '''将 EPUB 转换为 TXT 作为付费服务'''
    if not query or not query.endswith('.epub'):
        return False, '', '请提供 .epub 文件路径'
    try:
        result = epub_to_txt(query)
        with open(result, 'r') as f:
            content = f.read()
        return True, content[:2000], None  # 返回前 2000 字
    except Exception as e:
        return False, '', str(e)

set_business_handler(epub_business)
ms_main()
"
```

---

## 文件结构

```
pay-doc-analyzer/
├── SKILL.md                       # 本文档
├── skill.yaml                     # SkillHub 发布配置（含 Pay）
└── scripts/
    ├── merchant_server.py         # ★ 商户后端（Mock + Production）
    └── handler.py                 # ★ Skill 调用入口
```

## 重要函数说明

### merchant_server.py

| 函数 | 说明 |
|------|------|
| `_create_payment_order()` | 首次请求：生成订单 → 返回 402 + `WeixinPay-Required` |
| `_verify_and_fulfill()` | 支付后重试：验证 → 执行业务 → 返回内容 |
| `set_business_handler(fn)` | **注入你的业务逻辑**：`fn(query) -> (ok, content, err)` |

### handler.py

| 函数 | 说明 |
|------|------|
| `pay_handler(input_data)` | SkillHub 调用入口，处理首次请求和支付后重试 |
| `execute_business(query)` | 业务逻辑（可替换为真实业务） |

---

## Production 部署

### 1. 配置真实密钥

```bash
# 微信支付商户配置
export MCH_ID="1900000001"
export APP_ID="wx1234567890abcdef"
export SERIAL_NO="你的证书序列号"
export PRIVATE_KEY_PATH="/path/to/apiclient_key.pem"
export MCH_APIV3_KEY="你的 APIv3 密钥"

# SkillHub 开发者密钥（商户后台生成）
export SKILLHUB_DEVELOPER_ID="sh-XXXXXXXX"
export SKILLHUB_PUB_KEY_ID="PUB_KEY_xxx"
export SKILLHUB_PRIVATE_KEY="-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----"

# Skill 信息
export SKILL_ID="pay-doc-analyzer"
export SKILL_VERSION="1.0.0"
```

### 2. 启动 Production 模式

```bash
python3 scripts/merchant_server.py --production \
  --mch-id "$MCH_ID" \
  --app-id "$APP_ID" \
  --skillhub-dev-id "$SKILLHUB_DEVELOPER_ID" \
  --skillhub-pub-key "$SKILLHUB_PUB_KEY_ID" \
  --skillhub-priv-key /path/to/skillhub_private.pem
```

### 3. 发布到 SkillHub

```bash
cd pay-doc-analyzer
skillhub init --name "pay-doc-analyzer" --category "工具"
skillhub push
skillhub publish

# 验证
skillhub search pay-doc-analyzer
```

---

## 关键设计

### 402 响应规范

商户后端严格按照 Agent Pay X402 协议返回：

```
HTTP/1.1 402 Payment Required
WeixinPay-Required: <payment_code>    ← 支付凭证
X-Out-Trade-No: <out_trade_no>        ← 商户订单号
```

### 幂等控制

同一 `out_trade_no` 只履约一次，重复请求返回缓存结果（`already_fulfilled: true`），防止 Agent 重试导致重复扣费。

### 退款机制

业务失败时自动退款，用户无需申请。退款后原地返回 `{"code": "REFUNDED"}`，Agent 收到后应告知用户并终止。

---

## 测试清单

```markdown
- [ ] `python3 scripts/merchant_server.py` 启动成功（Mock 模式）
- [ ] `GET /health` 返回 `{"status": "ok"}`
- [ ] `POST /api/resource` 首次请求返回 **402**
- [ ] 402 响应包含 `WeixinPay-Required` Header
- [ ] 402 响应包含 `X-Out-Trade-No` Header
- [ ] 402 响应 Body 包含 `WeixinPay` 节点
- [ ] 重试（带 `X-Out-Trade-No`）返回 **200** + 付费内容
- [ ] 幂等：重复重试返回 `already_fulfilled: true`
- [ ] 退款：`trigger_refund` 场景返回 `code: REFUNDED`
- [ ] `echo '{"query":"test"}' | python3 handler.py --handler` 输出合法 JSON
```

## Related Skills

- [[skillpay-upgrade-case]] — 升级免费 Skill 为 Pay Skill 完整流程
- [[mch-demo]] — Go/Java 商户后端参考（含真实 X402 签名）
- [[skillpay-best-practice]] — 定价、UX、安全最佳实践
