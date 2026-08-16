---
name: mch-demo
description: Use when integrating a Skill with WeChat Pay X402 protocol as a merchant backend — Go/Java reference implementation for Native order, AI preorder signature, 402 payment trigger, payment verification, and automatic refund
---

# 微信 Agent Pay 商户接入 Demo

## Overview

本 Skill 是**商户后端参考实现**，演示如何通过 SkillHub 平台接入微信支付 X402 协议，为 Pay Skill 提供支付能力。

**典型场景：** 你有一个付费 Skill（如 epub-to-txt），需要搭建商户后端来处理微信支付下单、X402 预下单签名、支付回调验证和退款。

### 项目结构

```
mch-demo/
├── README.md              # 完整文档（架构、签名流程、接口设计、快速开始）
├── SKILL-example.md       # 商户 Skill Prompt 定义示例
├── SKILL.md               # 本文档
├── go/
│   ├── main.go            # Go 完整实现（HTTP 服务 + 微信支付 SDK）
│   └── go.mod
└── java/
    └── src/...            # Java Spring Boot 版本
```

### 两套密钥体系

| 密钥 | 用途 | 签名算法 |
|------|------|---------|
| **微信支付 API 证书** | Native 下单、查单、退款 | `WECHATPAY2-SHA256-RSA2048` |
| **SkillHub 开发者密钥** | AI 预下单（X402） | `SKILLHUB-SHA256-RSA2048` |

## 接口

| 路由 | 用途 | 说明 |
|------|------|------|
| `POST /api/resource` | 付费资源入口 | 首次 → 402 + `WeixinPay-Required`；支付后重试 → 返回内容 |
| `POST /api/pay/notify` | 支付结果回调 | SDK 自动验签 + AES-256-GCM 解密 |
| `POST /api/refund/notify` | 退款结果回调 | 同上 |

## 快速开始

```bash
cd go/
go mod tidy

# 设置环境变量（微信支付 + SkillHub 密钥 + Skill 信息）
export MCH_ID="你的商户号"
export SKILLHUB_DEVELOPER_ID="sh-XXXXXXXX"
export SKILLHUB_PUB_KEY_ID="PUB_KEY_xxx"
export SKILLHUB_PRIVATE_KEY="-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----"
# ... 详见 README.md

go run main.go
```

## 参考

完整文档、签名流程、接口说明、测试命令请见 [README.md](./README.md)。

## Related Skills

- [[skillpay-upgrade-case]] — epub-to-txt 支付升级完整案例
- [[skillpay-publish-skill]] — SkillHub 发布 Pay Skill
- [[skillpay-best-practice]] — 商业化运营
