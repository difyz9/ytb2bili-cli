---
name: skillpay-upgrade-case
description: Use when upgrading an existing free Skill to a SkillHub Pay Skill with X402 payment integration — covers developer key generation, merchant backend setup, X402 AI preorder, local testing, and publishing as a paid skill
---

# Pay Skill 升级案例：ePub 转文本付费集成

## Overview

本案例以 `epub-to-txt` Skill 为例，完整演示将一个**免费 CLI 工具**升级为 **SkillHub Pay Skill** 的全过程，包含：

- 商户后端搭建（Go mch-demo）
- X402 AI 预下单集成
- 本地测试支付流程
- 发布到 SkillHub 商店

### 架构总览

```
┌─ 用户 / AI Agent ─────────────────────────────┐
│                                                │
│  1. 触发 "epub转txt" Skill                     │
│  2. 收到 402 + WeixinPay-Required              │
│  3. 用户手机确认支付（AI 专属卡）                │
│  4. 携带 X-Out-Trade-No 重试获取内容            │
└──────────────────────┬─────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────────────┐
│  SkillHub 平台                                          │
│  - 来源认证、内容完整性校验、可信调用入口                   │
│  - 调度 epub-to-txt Skill                               │
│  - 管理 X402 协议支付流程                                │
└──────────────────────┬──────────────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────────────┐
│  商户后端 (mch-demo/go/main.go)                        │
│  ├─ POST /api/resource      ← SkillHub/Agent 调用入口  │
│  │  1. Native 下单（微信支付）                           │
│  │  2. X402 AI 预下单（SkillHub 开发者密钥签名）          │
│  │  3. 返回 402 + WeixinPay-Required                    │
│  │  4. 用户付费后 → 查单验证 → 执行业务(epub_to_txt)    │
│  │  5. 返回付费内容                                      │
│  ├─ POST /api/pay/notify    ← 微信支付回调               │
│  └─ POST /api/refund/notify ← 微信退款回调               │
└──────────────────────┬──────────────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────────────┐
│  微信支付 + SkillHub X402 网关                          │
│  - Native 下单 / 查单 / 退款                            │
│  - AI 专属卡余额管理、笔笔确认                           │
│  - 资金结算清分                                         │
└─────────────────────────────────────────────────────────┘
```

---

## Phase 1：前提条件

### 1.1 环境准备

| 项目 | 说明 | 获取方式 |
|------|------|---------|
| 企业认证 | SkillHub 企业认证通过 | skillhub.cn/console |
| 微信商户号 | 已开通微信支付商户 | 微信支付商户平台 |
| SkillHub CLI | 用于发布 Skill | `curl -fsSL https://skillhub.cn/install/install.sh \| bash` |
| Go 1.21+ | 运行商户后端 Demo | `brew install go` 或官网下载 |
| Python 3 | epub_to_txt 运行环境 | 系统自带或官网下载 |

### 1.2 前置条件确认清单

```bash
# === 确认企业认证状态 ===
skillhub merchant status

# === 确认微信商户号已绑定 ===
skillhub merchant account

# === 确认 SkillHub CLI 可用 ===
skillhub -h

# === 确认 Go 环境 ===
go version

# === 确认 Python 环境 ===
python3 --version
```

---

## Phase 2：生成开发者密钥（X402 签名用）

> 此步骤在 SkillHub 商户中心完成，生成用于 X402 AI 预下单签名的 RSA2048 密钥对。

### 2.1 操作路径

```
SkillHub 控制台 → 商户中心 → 开发者密钥 → 生成密钥
```

### 2.2 输出

生成后会得到两个值：

| 字段 | 格式 | 说明 |
|------|------|------|
| `pub_key_id` | `PUB_KEY_` + 32 位大写 HEX | 标识使用哪把公钥验签 |
| `private_key_pem` | RSA 2048 私钥（PEM 格式） | **只展示一次**，安全保存 |

> ⚠️ **安全红线：** private_key_pem 不能放到前端、客户端或公开仓库。建议按环境区分测试和生产密钥，同一商户最多保留 3 组。

### 2.3 环境变量配置

```bash
# 商户后端运行时需要的环境变量
export SKILLHUB_DEVELOPER_ID="sh-XXXXXXXX"          # SkillHub 商户号
export SKILLHUB_PUB_KEY_ID="PUB_KEY_0123456789..."  # 密钥 ID
export SKILLHUB_PRIVATE_KEY="-----BEGIN RSA PRIVATE KEY-----\n...\n-----END RSA PRIVATE KEY-----"  # 私钥

export MCH_ID="1900000001"                          # 微信支付商户号
export APP_ID="wx1234567890abcdef"                  # 应用 ID
export SERIAL_NO=""                                 # 商户证书序列号
export PRIVATE_KEY_PATH="/path/to/apiclient_key.pem" # 商户私钥文件路径
export MCH_APIV3_KEY=""                             # APIv3 密钥
export PAY_NOTIFY_URL="https://example.com/pay/notify"
export REFUND_NOTIFY_URL="https://example.com/refund/notify"

# Skill 信息
export SKILL_ID="epub-to-txt"                       # Skill slug
export SKILL_VERSION="1.0.0"                        # Skill 版本
```

---

## Phase 3：搭建商户后端

> 商户后端是整个支付集成的核心服务。它处理微信支付下单、X402 预下单签名、支付回调验证和退款。

本仓库提供了完整的 Go 参考实现（`mch-demo/go/main.go`），可直接作为起点。

### 3.1 目录结构

```
mch-demo/go/
├── main.go              # 商户后端完整实现
├── go.mod
├── go.sum
└── README.md
```

### 3.2 核心实现对应关系

参考实现中的函数与 Pay Skill 升级教程的对应关系：

| 教程步骤 | Go 实现函数 | 说明 |
|---------|------------|------|
| Step 1: 开发者密钥 | 环境变量 `SKILLHUB_DEVELOPER_ID` 等 | 启动时加载 |
| Step 2: 微信支付下单 | `callNativeOrder()` | 调用 Native 下单 API |
| Step 3: X402 预下单 | `callAIPreorder()` | 构造 L2 → Base64 → SHA256withRSA 签名 → L1 |
| Step 4: 返回支付触发 | `createPaymentOrder()` | 返回 402 + `WeixinPay-Required` Header |
| 支付后验证履约 | `verifyAndFulfill()` | 查单 → 执行业务 → 返回内容 |
| 支付回调 | `handlePayNotify()` | SDK 验签 + AES-256-GCM 解密 |
| 退款 | `callRefund()` + `handleRefundNotify()` | 业务失败自动退款 |

### 3.3 启动商户后端

```bash
cd mch-demo/go

# 确保环境变量已设置（Phase 2.3）
go run main.go

# 输出示例：
# 微信支付 SDK 客户端初始化成功
# 微信支付回调通知处理器初始化成功
# 微信 Agent Pay 商户 Demo 启动，监听端口: 8080
# 接口地址: POST http://localhost:8080/api/resource
```

### 3.4 验证后端正常运行

```bash
# 健康检查
curl http://localhost:8080/health
# → ok

# 模拟未付费请求
curl -X POST http://localhost:8080/api/resource \
  -H "Content-Type: application/json" \
  -d '{"query": "test.epub"}'

# 应返回 402 Payment Required + WeixinPay-Required Header:
# HTTP/1.1 402
# WeixinPay-Required: <payment_code>
# {
#   "code": "PAYMENT_REQUIRED",
#   "WeixinPay": { "WeixinPay-Required": "..." }
# }
```

---

## Phase 4：连接 epub-to-txt 到支付流程

epub-to-txt 的 `pay_handler` 已经可以返回结构化结果。现在需要让它**作为商户后端的业务执行层**被调用。

### 4.1 替换商户后端的业务逻辑

在 `mch-demo/go/main.go` 中，`executeBusiness()` 函数是替换点：

```go
// === 当前（模拟内容）===
func executeBusiness(query string) (string, error) {
    return generatePaidContent(query), nil  // 返回模拟文本
}

// === 改为调用 epub_to_txt.py ===
func executeBusiness(query string) (string, error) {
    // query 为 EPUB 文件路径
    cmd := exec.Command("python3",
        "/path/to/epub-to-txt/scripts/epub_to_txt.py",
        query)
    output, err := cmd.CombinedOutput()
    if err != nil {
        return "", fmt.Errorf("epub_to_txt 执行失败: %w", err)
    }
    return string(output), nil
}
```

### 4.2 或者反向：epub_to_txt.py 直接调用商户后端

在 `epub_to_txt.py` 的 `pay_handler` 中，如果检测到需要付费，先调用商户后端：

```python
import requests

def pay_handler(input_data):
    epub_path = input_data.get('epub_path', '')
    
    # 1. 调用商户后端获取支付凭据
    resp = requests.post(
        "http://localhost:8080/api/resource",
        json={"query": epub_path}
    )
    
    # 2. 如果返回 402，把 payment_code 返回给 Agent
    if resp.status_code == 402:
        payment_code = resp.headers.get("WeixinPay-Required")
        out_trade_no = resp.headers.get("X-Out-Trade-No")
        return {
            'success': False,
            'payment_required': True,
            'payment_code': payment_code,
            'out_trade_no': out_trade_no,
            'message': '需要支付 ¥0.50 后才能转换',
        }
    
    # 3. 如果已支付，正常执行转换
    # ...
```

### 4.3 验证集成

```bash
# 启动商户后端
cd mch-demo/go && go run main.go &

# 测试 epub_to_txt 的 pay_handler
echo '{"epub_path": "/path/to/book.epub"}' | python3 scripts/epub_to_txt.py --handler

# 输出应包含 payment_required 或 直接返回转换结果
```

---

## Phase 5：本地测试支付流程

### 5.1 模拟完整支付链路

测试工具：用 curl 模拟 Agent 行为（参照 Go mch-demo 的接口约定）。

```bash
# ===== Step 1: 首次请求（未支付）=====
FIRST_RESP=$(curl -s -w "\n%{http_code}\n%{header_json}" \
  -X POST http://localhost:8080/api/resource \
  -H "Content-Type: application/json" \
  -d '{"query": "/path/to/book.epub"}')

# 应输出:
# HTTP 402
# WeixinPay-Required: <payment_code>
# X-Out-Trade-No: WX402_20260723xxxxxx

# 提取 out_trade_no
OUT_TRADE_NO=$(echo "$FIRST_RESP" | grep -o '"out_trade_no":"[^"]*"' | cut -d'"' -f4)
echo "订单号: $OUT_TRADE_NO"
```

### 5.2 验证 402 响应格式

对照教程规范检查返回：

```bash
curl -s -D - -X POST http://localhost:8080/api/resource \
  -H "Content-Type: application/json" \
  -d '{"query": "test.epub"}' | head -20

# 必须包含：
# ✅ HTTP/1.1 402 Payment Required
# ✅ WeixinPay-Required: <payment_code>
# ✅ Content-Type: application/json
# ✅ Body 含 WeixinPay 节点
```

### 5.3 查单验证

手动查单模拟 Agent 付费后重试：

```bash
# 模拟 Agent 付费后，携带 X-Out-Trade-No 重试
curl -s -X POST http://localhost:8080/api/resource \
  -H "Content-Type: application/json" \
  -H "X-Out-Trade-No: <OUT_TRADE_NO>" \
  -d '{"query": "/path/to/book.epub"}'

# 如果微信支付订单已支付（trade_state=SUCCESS）：
# → HTTP 200 + 付费内容
# → order.Status = "FULFILLED"

# 如果未支付：
# → HTTP 402 + "订单尚未支付完成"
```

### 5.4 测试退款流程

```bash
# 使用特殊 trigger_refund 查询，触发业务失败 → 自动退款
curl -s -X POST http://localhost:8080/api/resource \
  -H "Content-Type: application/json" \
  -d '{"query": "trigger_refund"}'

# 获取 out_trade_no
# 模拟支付后重试
curl -s -X POST http://localhost:8080/api/resource \
  -H "Content-Type: application/json" \
  -H "X-Out-Trade-No: <OUT_TRADE_NO>" \
  -d '{"query": "trigger_refund"}'

# 应返回:
# {"code": "REFUNDED", "message": "已发起全额退款"}
```

### 5.5 测试幂等性

```bash
# 支付成功后，重复请求应返回缓存结果
curl -s -X POST http://localhost:8080/api/resource \
  -H "Content-Type: application/json" \
  -H "X-Out-Trade-No: <OUT_TRADE_NO>" \
  -d '{"query": "/path/to/book.epub"}'

# 应返回:
# {"code": "SUCCESS", "content": "...", "already_fulfilled": true}
```

---

## Phase 6：发布到 SkillHub

### 6.1 准备 SKILL.md + skill.yaml

epub-to-txt 已经有配置好的 `skill.yaml`：

```yaml
name: epub-to-txt
display_name: "ePub 转文本"
version: "1.0.0"
category: 工具
platforms:
  - workbuddy
  - qclaw
  - claude-code

pay:
  enabled: true
  price:
    amount: 0.50
    currency: CNY
  billing:
    mode: per_call
    min_balance: 5.00
  trial:
    enabled: true
    calls: 3
  x402:
    auth_mode: per_payment
```

### 6.2 提交发布

```bash
# 标准三步流程
cd path/to/epub-to-txt
skillhub init --name "epub-to-txt" --category "工具"
# 确认 SKILL.md frontmatter 已包含 name/description/version/category/platforms
skillhub push          # 推送草稿
skillhub publish       # 提交审核
```

### 6.3 审核等待期

发布后触发三线并行安全审核：

| 审核项 | 预计周期 | 说明 |
|--------|---------|------|
| 内容合规过滤 | 自动 | 检查 Skill 描述和触发词 |
| 漏洞扫描 | 1-2 小时 | 检查脚本安全性 |
| AI 安全评估 | 1-2 工作日 | AI 模型行为评估 |

### 6.4 发布后验证

```bash
# 查看发布状态
skillhub publish status <PUBLISH_ID>

# 搜索验证上架
skillhub search epub-to-txt

# 查看商店中
skillhub explore
```

---

## Phase 7：生产环境验证

### 7.1 完整 E2E 测试清单

```markdown
- [ ] 商户后端运行正常（/health → ok）
- [ ] 未付费请求返回 402 + WeixinPay-Required Header
- [ ] 402 响应 Body 包含 WeixinPay 节点
- [ ] 付费后重试（X-Out-Trade-No）返回 200 + 内容
- [ ] 订单幂等：重复请求返回缓存结果
- [ ] 业务失败自动退款
- [ ] 支付回调正常处理（handlePayNotify）
- [ ] 退款回调正常处理（handleRefundNotify）
- [ ] SkillHub 搜索结果能搜到 epub-to-txt
- [ ] 用户可以通过 `skillhub install epub-to-txt` 安装
```

### 7.2 常见问题排查

| 现象 | 原因 | 解决 |
|------|------|------|
| 402 响应缺少 `WeixinPay-Required` Header | 服务端未正确设置 Header | 检查 `w.Header().Set("WeixinPay-Required", paymentCode)` |
| `X402 预下单失败` | 签名串格式错误（换行/Base64） | 确认 5 行签名串每行以 `\n` 结尾，最后一行也保留换行 |
| `签名校验失败` | private_key 与 pub_key_id 不匹配 | 检查是否使用了同一组密钥，且私钥未换行丢失 |
| `payment_required 内容不一致` | L2 JSON 修改后未重新 Base64 和签名 | 每次修改 L2 都要重新签名 |
| `payment_code 过期` | 超过 15 分钟有效期 | 重新执行微信支付下单 + X402 预下单 |
| `Agent 不识别 WeixinPay-Required` | Agent 版本不支持 | 确认 Agent 支持 WeixinPay-Required 协议，或升级 Agent SDK |

---

## 相关参考

| 资源 | 说明 |
|------|------|
| `epub-to-txt/skill.yaml` | Pay Skill 配置文件 |
| `epub-to-txt/scripts/epub_to_txt.py` | 业务脚本 + SkillHub handler |
| `mch-demo/go/main.go` | 商户后端完整参考实现 |
| `skillpay-publish-skill` | SkillHub 发布 Skill 完整指南 |
| `skillpay-best-practice` | 定价、UX、安全最佳实践 |

## Related Skills

- [[skillpay-publish-skill]] — SkillHub 发布基础
- [[skillpay-best-practice]] — 商业化运营
- [[mch-demo]] — 商户后端参考实现
