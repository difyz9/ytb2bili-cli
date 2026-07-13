# 飞书集成方案

## 架构

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  Chrome 插件    │ ──► │  飞书机器人     │ ──► │  ytb2bili-go    │
│  (视频+cookies) │     │  (Webhook/WS)   │     │  (处理+上传)    │
└─────────────────┘     └─────────────────┘     └─────────────────┘
```

## 优势

- ✅ 用户无需配置 HTTPS 域名
- ✅ 无需服务器，飞书免费
- ✅ 消息实时推送
- ✅ 支持卡片交互

## 配置步骤

### 1. 创建飞书机器人

1. 访问 [飞书开放平台](https://open.feishu.cn/)
2. 创建企业自建应用
3. 获取 App ID 和 App Secret
4. 启用机器人能力
5. 配置事件订阅 (接收消息)

### 2. 配置 ytb2bili-go

```bash
y2b server --feishu-app-id APP_ID --feishu-app-secret APP_SECRET
```

### 3. 配置 Chrome 插件

在插件设置中配置:
- 飞书 Webhook URL (可选)
- 或使用默认的飞书机器人

## 消息格式

Chrome 插件发送的消息格式:

```json
{
  "type": "video_submit",
  "data": {
    "url": "https://www.youtube.com/watch?v=xxxxx",
    "title": "视频标题",
    "description": "视频描述",
    "subtitles": [...],
    "cookies": "加密的cookies"
  }
}
```

## 飞书机器人命令

| 命令 | 说明 |
|------|------|
| `help` | 显示帮助 |
| `history` | 查看历史 |
| `status` | 查看状态 |
| YouTube 链接 | 自动处理视频 |
