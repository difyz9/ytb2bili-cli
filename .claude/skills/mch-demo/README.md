# 微信 Agent Pay - 商户接入 Demo

> 本目录包含 Go / Java 两种语言的商户接入示例，演示如何通过 SkillHub 平台接入微信 Agent Pay（X402 协议），实现 Agent 付费资源获取。

---

## 协议流程概览



```mermaid
sequenceDiagram
    participant U as 用户
    participant A as Agent (LLM)
    participant WP as weixinpay_pay 插件
    participant MS as 商户服务（本Demo）
    participant WX as 微信支付
    participant SH as SkillHub 平台

    A->>MS: ① POST /api/resource {"query":"xxx"}
    MS->>WX: ② Native下单（微信支付 API 证书签名）
    WX-->>MS: code_url
    MS->>WX: ③ AI预下单（SkillHub 开发者密钥签名）
    Note over MS,WX: 微信支付向 SkillHub 验签
    WX->>SH: ③a 验签（确认商户身份）
    SH-->>WX: ③b 验签通过
    WX-->>MS: payment_code
    MS-->>A: ④ HTTP 402 + Header: WeixinPay-Required, X-Out-Trade-No

    Note over A: Agent 识别到需要支付
    A->>WP: ⑤ 调用 weixinpay_pay(paymentCode=xxx)
    WP->>U: ⑥ 展示支付确认
    U->>WP: ⑦ 确认支付
    WP-->>A: ⑧ 返回"支付成功"

    Note over A: Skill Prompt 要求：支付成功后携带 out_trade_no 重新请求
    A->>MS: ⑨ POST /api/resource + Header X-Out-Trade-No, WeixinPay-Required（body不变）
    MS->>WX: ⑩ 查单验证（微信支付 API 证书签名）
    WX-->>MS: trade_state=SUCCESS
    MS-->>A: ⑪ HTTP 200 + 付费内容
```

---

## 两套密钥说明

本 Demo 涉及**两套不同的密钥**，请务必区分：

| 密钥                          | 用途                            | 签名算法                  | 获取方式                  |
| ----------------------------- | ------------------------------- | ------------------------- | ------------------------- |
| **微信支付 API 证书**   | Native 下单、查单等标准支付接口 | WECHATPAY2-SHA256-RSA2048 | 微信支付商户平台申请      |
| **SkillHub 开发者密钥** | AI 预下单接口                   | SKILLHUB-SHA256-RSA2048   | SkillHub 商户后台一键生成 |

> ⚠️ AI 预下单接口**不使用**微信支付 API 证书签名，而是使用 SkillHub 颁发的开发者密钥签名。微信支付后台会向 SkillHub 验证签名，确认商户身份。

---

## 商户需要做的事

| 步骤         | 操作                                                  | 签名方式                      | 调用的接口                                                         |
| ------------ | ----------------------------------------------------- | ----------------------------- | ------------------------------------------------------------------ |
| 1. 创建订单  | 生成 out_trade_no（≤32位），调用微信支付下单         | 微信支付 API 证书             | `POST /v3/pay/transactions/native`                               |
| 2. AI 预下单 | 构造 L1/L2 请求体，获取 payment_code                  | **SkillHub 开发者密钥** | `POST payapp.weixin.qq.com/palmpayminiapp/clawagentpay/preorder` |
| 3. 返回 402  | 构造响应，Header 包含 payment_code 和 out_trade_no    | -                             | -                                                                  |
| 4. 验证支付  | Agent 通过 Header X-Out-Trade-No 携带订单号重试时查单 | 微信支付 API 证书             | `GET /v3/pay/transactions/out-trade-no/{out_trade_no}`           |
| 5. 履约交付  | 验证通过后返回付费内容                                | -                             | -                                                                  |

---

## AI 预下单签名流程

AI 预下单采用 L1/L2 两层结构：

```
┌─────────────────────────────────────────────────────┐
│ L1 请求体                                            │
│  signature_type: "SKILLHUB-SHA256-RSA2048"          │
│  developer_platform: "SKILLHUB"                      │
│  developer_id: "sh-XXXXXXXX"                         │
│  pub_key_id: "PUB_KEY_xxx"                           │
│  nonce_str: "32位随机串"                             │
│  timestamp: "Unix秒级时间戳"                         │
│  signature: "Base64(RSA签名)"                        │
│  payment_required: "Base64(L2 JSON)"                 │
│                                                      │
│  ┌─────────────────────────────────────────────┐    │
│  │ L2 业务 JSON（Base64 编码后放入上方字段）     │    │
│  │  skill_info: { skill_id, skill_version }     │    │
│  │  pay_type: "SKILL_PAY"                       │    │
│  │  pay_mode: "AUTH_AND_PAY"                    │    │
│  │  pay_items: [{ product_id, pay_data }]       │    │
│  │  expires_at: "Unix秒级时间戳"                │    │
│  └─────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────┘
```

签名串格式（5 行，每行以 `\n` 结尾）：

```
POST\n
/palmpayminiapp/clawagentpay/preorder\n
{timestamp}\n
{nonce_str}\n
{payment_required}\n
```

---

## 接口设计

### 统一接口: `POST /api/resource`

#### 请求体

```json
{
    "query": "用户查询内容"
}
```

#### 请求 Header（支付后重试时携带）

| Header                 | 说明                                                 |
| ---------------------- | ---------------------------------------------------- |
| `X-Out-Trade-No`     | 商户订单号（402 响应中返回，重试时必须携带）         |
| `WeixinPay-Required` | 支付凭证码（402 响应中返回，重试时携带，商户可忽略） |

#### 场景1：首次请求（无 X-Out-Trade-No Header）

**响应: HTTP 402**

```json
{
    "code": "PAYMENT_REQUIRED",
    "message": "需要支付后才能获取内容",
    "WeixinPay": {
        "WeixinPay-Required": "payment_code_xxx",
        "prompt": "本次使用微信支付，请将 WeixinPay-Required 的值作为 paymentCode 交给 weixinpay_pay，以向用户申请支付授权。"
    },
    "out_trade_no": "WX402_20260630120000abcdef123456",
    "amount": "0.01",
    "currency": "CNY",
    "description": "AI付费查询: xxx"
}
```

响应头:

```
WeixinPay-Required: payment_code_xxx
X-Out-Trade-No: WX402_20260630120000abcdef123456
```

#### 场景2：支付后重试（Header 携带 X-Out-Trade-No）

**响应: HTTP 200**（支付验证通过时）

```json
{
    "code": "SUCCESS",
    "message": "付费内容",
    "out_trade_no": "WX402_20260630120000abcdef123456",
    "transaction_id": "4200001234202306300000000001",
    "content": "【付费内容】...",
    "already_fulfilled": false
}
```

---

## 快速开始

### Go 版本

```bash
cd go/

# 下载依赖
go mod tidy

# 设置环境变量
# 微信支付商户配置
export MCH_ID="你的商户号"
export APP_ID="你的应用ID"
export SERIAL_NO="你的证书序列号"
export PRIVATE_KEY_PATH="/path/to/apiclient_key.pem"  # 商户私钥文件路径
export MCH_APIV3_KEY="你的APIv3密钥"
export PAY_NOTIFY_URL="https://your-domain.com/api/pay/notify"       # 支付结果回调地址
export REFUND_NOTIFY_URL="https://your-domain.com/api/refund/notify" # 退款结果回调地址

# SkillHub 开发者密钥配置
export SKILLHUB_DEVELOPER_ID="sh-XXXXXXXX"
export SKILLHUB_PUB_KEY_ID="PUB_KEY_408B07E79B8269FEC3D5D3E6AB8ED163"
export SKILLHUB_PRIVATE_KEY="-----BEGIN PRIVATE KEY-----\nMIIE...\n-----END PRIVATE KEY-----"

# Skill 信息
export SKILL_ID="your-skill-slug"
export SKILL_VERSION="1.0.0"

# 运行
go run main.go
```

### Java 版本

```bash
cd java/

# 设置环境变量
# 微信支付商户配置
export MCH_ID="你的商户号"
export APP_ID="你的应用ID"
export SERIAL_NO="你的证书序列号"
export PRIVATE_KEY_PATH="/path/to/apiclient_key.pem"  # 商户私钥文件路径
export MCH_APIV3_KEY="你的APIv3密钥"
export PAY_NOTIFY_URL="https://your-domain.com/api/pay/notify"       # 支付结果回调地址
export REFUND_NOTIFY_URL="https://your-domain.com/api/refund/notify" # 退款结果回调地址

# SkillHub 开发者密钥配置
export SKILLHUB_DEVELOPER_ID="sh-XXXXXXXX"
export SKILLHUB_PUB_KEY_ID="PUB_KEY_408B07E79B8269FEC3D5D3E6AB8ED163"
export SKILLHUB_PRIVATE_KEY="-----BEGIN PRIVATE KEY-----\nMIIE...\n-----END PRIVATE KEY-----"

# Skill 信息
export SKILL_ID="your-skill-slug"
export SKILL_VERSION="1.0.0"

# 构建并运行
mvn spring-boot:run
```

---

## 测试

```bash
# 首次请求（会返回 402）
curl -X POST http://localhost:8080/api/resource \
  -H "Content-Type: application/json" \
  -d '{"query": "帮我分析一下AI行业趋势"}'

# 支付后重试（通过 Header 携带 out_trade_no）
curl -X POST http://localhost:8080/api/resource \
  -H "Content-Type: application/json" \
  -H "X-Out-Trade-No: WX402_20260630120000abcdef123456" \
  -H "WeixinPay-Required: payment_code_xxx" \
  -d '{"query": "帮我分析一下AI行业趋势"}'
```

---

## 关键设计说明

### out_trade_no 规则

- 长度不超过 **32 位**（微信支付 Native 下单接口要求）
- 本 Demo 生成格式: `WX402_` + 时间戳(14位) + 随机串(12位) = 32位
- 商户可自定义生成规则，只要满足唯一性和长度限制即可

### 幂等控制

- 同一 `out_trade_no` 只履约一次
- 重复请求返回缓存的履约结果（`already_fulfilled: true`）
- 生产环境建议使用数据库事务保证幂等

### 签名说明

| 接口                | 签名密钥                      | 本 Demo 实现情况                       |
| ------------------- | ----------------------------- | -------------------------------------- |
| Native 下单         | 微信支付 API 证书             | **完整实现**（使用微信支付 SDK） |
| 查单                | 微信支付 API 证书             | **完整实现**（使用微信支付 SDK） |
| 退款                | 微信支付 API 证书             | **完整实现**（使用微信支付 SDK） |
| **AI 预下单** | **SkillHub 开发者密钥** | **完整实现**（RSA 签名逻辑）     |

微信支付官方 SDK：

| 语言 | 官方 SDK                                                                                    |
| ---- | ------------------------------------------------------------------------------------------- |
| Go   | [github.com/wechatpay-apiv3/wechatpay-go](https://github.com/wechatpay-apiv3/wechatpay-go)     |
| Java | [com.github.wechatpay-apiv3:wechatpay-java](https://github.com/wechatpay-apiv3/wechatpay-java) |

---

## 商户 Skill 定义

参见 [SKILL-example.md](./SKILL-example.md)，展示如何编写 Skill Prompt 指导 Agent 完成完整的支付流程。

---

## 目录结构

```
mch-demo/
├── README.md              # 本文件
├── SKILL-example.md       # 商户 Skill 定义示例
├── go/                    # Go 版本
│   ├── main.go
│   └── go.mod
└── java/                  # Java 版本（Spring Boot）
    ├── pom.xml
    └── src/main/java/com/example/weixinpayx402demo/
        └── WeixinPayX402DemoApplication.java
```

---

## 参考文档

| 文档                                                                                     | 说明                                                                                                                                                                               |
| ---------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [Agent Pay Skill 商户简易改造教程](https://doc.weixin.qq.com/doc/w3_ANYA8AbAACcCN63aHvQxpRHKetJmt?scode=AJEAIQdfAAoQq0nfHnANYA8AbAACc)                  | SkillHub 平台完整的商户接入教程                                                                                                                                                    |
| 微信支付下单接口                                                                         | [Native](https://pay.weixin.qq.com/doc/v3/merchant/4012791877) / [JSAPI](https://pay.weixin.qq.com/doc/v3/merchant/4012062524) / [H5](https://pay.weixin.qq.com/doc/v3/merchant/4012791832) |
| [商户订单号查询订单](https://pay.weixin.qq.com/doc/v3/merchant/4012791880)                  | 支付后验证订单状态                                                                                                                                                                 |
| [微信支付 Agent 支付 X402 协议](https://doc.weixin.qq.com/doc/w3_AKUAqAbdAFwCNhqHxTahnQE0nBQwN?scode=AJEAIQdfAAoNWryXPcANYA8AbAACc) | 微信支付提供的 X402 协议完整文档                                                                                                                                                   |
