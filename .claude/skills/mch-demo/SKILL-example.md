# 微信X402协议 - 商户 Skill 定义示例

> 本文件是商户编写 Skill 的参考模板。
> 商户根据自身业务修改后，将 Skill 注册到 Agent 框架中即可。

---

## SKILL.md 示例

```markdown
---
name: example-paid-query
description: 付费AI查询服务，通过微信X402协议完成支付后获取专业分析结果
version: 1.0.0
author: 商户名称
---

# 付费AI查询服务

## 功能描述
本技能提供付费的AI专业查询服务。每次查询需要支付 0.01 元。

## 付费前置检查

调用本 Skill 前，检查当前 Agent 是否已安装 `weixinpay` 插件（extension）：

- 已安装：可继续提供付费服务
- 未安装：向用户提示"当前 Agent 暂不支持微信支付付费能力"，终止流程

## 工作流程

### 第一步：请求资源
向以下 URL 发起 POST 请求：

```bash
curl -X POST https://merchant.example.com/api/resource \
  -H "Content-Type: application/json" \
  -d '{"query": "用户的查询内容"}'
```

### 第二步：处理 402 响应（支付由插件完成）

如果收到 HTTP 402 响应，`weixinpay` 插件会拦截并完成支付流程，无需手动调用支付接口。

402 响应中会通过 Header 和 Body 返回支付信息：

- Header `WeixinPay-Required`：支付凭证码（插件用于完成支付）
- Header `X-Out-Trade-No`：商户订单号

> 支付过程由 `weixinpay` 插件处理，Skill 无需关心支付细节。

### 第三步：支付成功后获取资源（⚠️ 必须执行）

支付成功后，**必须**重新发起请求获取付费内容。重试规则：

- **JSON body 不变**：与首次请求完全一致
- **通过 Header 携带支付信息**：将 402 响应中的 `WeixinPay-Required` 和 `X-Out-Trade-No` Header 原样带上

重试请求示例：

```http
POST https://merchant.example.com/api/resource
Content-Type: application/json
WeixinPay-Required: <payment_code>
X-Out-Trade-No: <out_trade_no>

{"query": "用户的查询内容"}
```

> 注意：body 与第一步完全一致，不要修改 body 中的任何字段。支付信息通过 Header 传递。

### 第四步：处理退款响应

如果第三步返回 `"code": "REFUNDED"`，说明商户服务异常无法提供内容，已自动退款：

- 向用户说明："抱歉，该服务暂时无法提供内容，已自动退款至原支付账户。"
- **不要**再次尝试支付或重新请求

## 注意事项

1. 必须先通过"付费前置检查"确认 weixinpay 插件已安装，否则不要进入支付流程
2. 收到 402 响应时，支付由 weixinpay 插件完成，Skill 无需关心支付细节
3. 支付成功后必须主动发起重试请求，body 保持不变，通过 Header `X-Out-Trade-No` 传递订单号
4. 如果返回 "NOT_PAID"，说明支付尚未完成，请等待后重试
5. 如果返回 "REFUNDED"，说明服务异常已退款，告知用户并终止流程
6. 如果返回 "FULFILL_AND_REFUND_FAILED"，说明服务异常且退款失败，建议用户联系客服
