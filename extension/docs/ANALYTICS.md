# Analytics 集成说明

本项目已集成 `@difyz/ts-analysis-client` 用于用户行为分析和数据追踪。

## 配置

### 1. 环境变量配置

复制 `.env.example` 文件为 `.env.local` 并配置：

```bash
cp .env.example .env.local
```

编辑 `.env.local` 文件：

```env
# 分析服务器配置
VITE_ANALYTICS_SERVER_URL=http://localhost:8080
VITE_ANALYTICS_PRODUCT_NAME=ytb2bili-extension
VITE_ANALYTICS_DEBUG=true

# 后端服务器配置
VITE_BACKEND_URL=http://localhost:8096
```

### 2. 分析服务器

确保分析服务器（go-analysis-server）已启动并运行在配置的地址上。

## 功能特性

### 自动追踪的事件

1. **用户登录事件** (`user_login`)
   - 触发时机：用户成功登录后
   - 数据字段：
     - `user_id`: 用户ID
     - `user_mid`: 用户MID
     - `username`: 用户名
     - `avatar`: 头像URL
     - `level`: 用户等级
     - `fans`: 粉丝数
     - `attention`: 关注数
     - `platform`: 平台（bilibili）
     - `timestamp`: 时间戳

2. **用户状态检查** (`user_status_check`)
   - 触发时机：调用 `getUserStatus` API 获取用户状态时
   - 数据字段：包含用户信息和登录状态

3. **用户退出登录** (`user_logout`)
   - 触发时机：用户点击退出登录按钮
   - 数据字段：时间戳

4. **视频提交事件** (`video_submission`)
   - 触发时机：提交视频到后端处理
   - 数据字段：
     - `video_id`: 视频ID
     - `title`: 视频标题
     - `platform`: 平台（youtube/bilibili）
     - `success`: 提交是否成功
     - `timestamp`: 时间戳

## 代码结构

```
utils/
  ├── analytics.ts        # 分析客户端封装模块
  └── api.ts             # API 模块（已集成分析追踪）

entrypoints/
  ├── background.ts      # 后台脚本（已初始化 Analytics）
  └── popup/
      └── main.tsx       # Popup 入口（已初始化 Analytics）
```

## API 使用

### 初始化（已自动完成）

在 `background.ts` 和 `popup/main.tsx` 中已自动初始化：

```typescript
import { initAnalytics } from '../utils/analytics';

initAnalytics();
```

### 手动追踪事件

如需在其他地方追踪自定义事件：

```typescript
import { trackPageView } from '../utils/analytics';

// 追踪页面浏览
trackPageView('/settings', '设置页面');

// 追踪自定义事件
import Analytics from '@difyz/ts-analysis-client';

Analytics.track('custom_event', {
  property1: 'value1',
  property2: 123,
});
```

### 手动刷新队列

如需立即发送所有待发送的事件：

```typescript
import { flushAnalytics } from '../utils/analytics';

await flushAnalytics();
```

## 数据流程

1. **用户登录** → `authApi.getUserStatus()` → 自动推送 `user_login` 和 `user_status_check` 事件
2. **用户退出** → `authApi.logout()` → 自动推送 `user_logout` 事件
3. **视频提交** → `videoApi.submitVideoData()` → 自动推送 `video_submission` 事件

## 注意事项

1. **异步推送**：所有事件推送都是异步的，不会阻塞主业务流程
2. **批量上报**：SDK 会自动将事件加入队列，批量发送（默认 10 条或 5 秒）
3. **错误处理**：推送失败会在控制台输出日志，不影响主功能
4. **调试模式**：开发环境下会输出详细的调试日志

## 生产环境配置

生产环境部署时，需要修改 `.env.production`：

```env
VITE_ANALYTICS_SERVER_URL=https://your-analytics-server.com
VITE_ANALYTICS_DEBUG=false
```

## 相关资源

- [@difyz/ts-analysis-client 文档](https://www.npmjs.com/package/@difyz/ts-analysis-client)
- [go-analysis-server](https://github.com/difyz9/go-analysis-server)
