# 会员系统快速开始

## 🎯 实现效果

用户可以在插件的"账号管理"标签页看到：
1. **会员状态卡片** - 显示当前会员等级、可用功能、到期时间
2. **升级会员按钮** - 点击后自动携带设备ID跳转到购买页面

## 📦 已完成的功能

### 1. 类型定义 (`types/index.ts`)
- ✅ `LicenseType` - 会员类型枚举
- ✅ `LicenseStatus` - License 状态接口
- ✅ `LicenseVerifyResponse` - License 验证响应

### 2. API 模块 (`utils/api.ts`)
- ✅ `licenseApi.getDeviceId()` - 获取设备唯一标识
- ✅ `licenseApi.verifyLicense()` - 验证 License 状态
- ✅ `licenseApi.getMembershipUrl()` - 生成会员购买链接
- ✅ `licenseApi.openMembershipPage()` - 打开购买页面

### 3. 会员组件 (`components/MembershipComponents.tsx`)
- ✅ `MembershipButton` - 升级会员按钮
- ✅ `LicenseStatusBadge` - 会员状态徽章
- ✅ `MembershipCard` - 会员卡片（含状态、权益、升级按钮）

### 4. 功能限制 (`utils/membership.ts`)
- ✅ `checkFeatureAccess()` - 检查功能访问权限
- ✅ `getFeatureLimits()` - 获取功能限制配置
- ✅ `checkUsageLimit()` - 检查使用限制
- ✅ `checkBeforeFeatureUse()` - 功能执行前的完整检查
- ✅ `usageTracker` - 使用统计跟踪器

### 5. UI 集成 (`entrypoints/popup/App.tsx`)
- ✅ 在"账号管理"标签页添加会员卡片
- ✅ 显示会员状态、权益、升级入口

## 🚀 使用方法

### 步骤 1: 配置会员购买页面地址

创建 `.env.local` 文件：

```bash
cp .env.example .env.local
```

编辑 `.env.local`，设置你的会员购买页面地址：

```env
VITE_MEMBERSHIP_URL=https://your-domain.com/membership
```

### 步骤 2: 测试购买流程

1. 启动开发服务器：
```bash
npm run dev
# 或
yarn dev
```

2. 在浏览器中加载扩展

3. 打开扩展 Popup，切换到"账号管理"标签

4. 查看会员卡片，点击"升级会员"按钮

5. 会自动在新标签页打开购买页面，URL 格式如下：
```
https://your-domain.com/membership?device_id=xxx&source=popup&product=ytb2bili-extension&timestamp=xxx
```

### 步骤 3: 在功能中集成权限检查

示例：视频提交功能

```typescript
import { checkBeforeFeatureUse, usageTracker } from '../utils/membership';
import { licenseApi } from '../utils/api';

async function handleSubmitVideo() {
  // 检查权限和限制
  const check = await checkBeforeFeatureUse('video_submit');
  
  if (!check.allowed) {
    const shouldUpgrade = confirm(`${check.reason}\n\n是否立即升级会员？`);
    if (shouldUpgrade) {
      await licenseApi.openMembershipPage('video_submit_gate');
    }
    return;
  }

  // 执行视频提交
  const result = await submitVideo(videoData);
  
  if (result.success) {
    // 增加使用计数
    await usageTracker.incrementUsage('video_submit');
    alert('提交成功！');
  }
}
```

## 🎨 自定义样式

### 修改会员卡片样式

编辑 `components/MembershipComponents.tsx`：

```tsx
<div className="bg-gradient-to-br from-purple-50 to-pink-50 rounded-lg shadow-sm p-4">
  {/* 自定义背景颜色、边框等 */}
</div>
```

### 修改按钮样式

```tsx
<button className="bg-gradient-to-r from-purple-600 to-pink-600 hover:from-purple-700 hover:to-pink-700">
  {/* 自定义渐变色 */}
</button>
```

## 🔧 功能权限配置

在 `utils/membership.ts` 中配置功能权限：

```typescript
const FEATURE_PERMISSIONS: Record<string, LicenseType[]> = {
  // 添加你的功能
  'my_feature': ['pro', 'enterprise'],  // 仅专业版和企业版可用
};
```

## 📊 使用统计

查看今日使用情况：

```typescript
import { usageTracker } from '../utils/membership';

const usage = await usageTracker.getUsage('video_submit');
console.log('今日已提交视频数:', usage);
```

重置使用统计：

```typescript
// 重置特定功能
await usageTracker.resetUsage('video_submit');

// 重置所有统计
await usageTracker.resetUsage();
```

## 🎯 购买页面参数说明

当用户点击"升级会员"时，会携带以下参数：

| 参数 | 说明 | 示例 |
|------|------|------|
| `device_id` | 设备唯一标识 | `abc123def456` |
| `source` | 点击来源 | `popup`, `content`, `video_submit_gate` |
| `product` | 产品名称 | `ytb2bili-extension` |
| `timestamp` | 时间戳 | `1732512000000` |

### 后端处理流程

1. 接收参数，展示会员套餐
2. 用户选择套餐并支付
3. 支付成功后，根据 `device_id` 激活 License
4. 用户返回扩展，自动验证并更新会员状态

## 🔐 License 验证

### 前端调用

```typescript
import { licenseApi } from '../utils/api';

const status = await licenseApi.verifyLicense();

if (status?.is_valid) {
  console.log('会员类型:', status.license_type);
  console.log('到期时间:', status.expired_at);
  console.log('可用功能:', status.features);
}
```

### 后端接口格式

后端需要实现 License 验证接口，返回格式：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "is_valid": true,
    "license_type": "pro",
    "device_id": "abc123",
    "expired_at": "2025-12-31T23:59:59Z",
    "launch_count": 10,
    "features": [
      "批量提交最多20个",
      "AI 字幕翻译",
      "自动上传到B站",
      "优先处理队列"
    ]
  }
}
```

## 🎁 会员等级配置

在 `utils/membership.ts` 中配置各等级限制：

```typescript
const FEATURE_LIMITS: Record<LicenseType, {...}> = {
  free: {
    maxVideosPerDay: 5,      // 每日5个视频
    maxBatchSize: 1,         // 不支持批量
    aiTranslation: false,    // 无AI翻译
  },
  basic: {
    maxVideosPerDay: 20,
    maxBatchSize: 5,
    aiTranslation: true,
  },
  pro: {
    maxVideosPerDay: 100,
    maxBatchSize: 20,
    priorityProcessing: true,
    autoUpload: true,
  },
  enterprise: {
    maxVideosPerDay: -1,     // 无限制
    maxBatchSize: 100,
    // 所有功能
  },
};
```

## 📱 UI 效果

### 会员卡片预览

```
┌─────────────────────────────────┐
│  会员状态            ⭐          │
│  [专业版]                        │
│                                  │
│  当前权益：                       │
│  ✓ 批量提交最多20个              │
│  ✓ AI 字幕翻译                   │
│  ✓ 自动上传到B站                 │
│                                  │
│  到期时间：2025-12-31            │
└─────────────────────────────────┘
```

### 免费用户卡片

```
┌─────────────────────────────────┐
│  会员状态            ⭐          │
│  [免费版]                        │
│                                  │
│  升级会员解锁更多高级功能         │
│                                  │
│  ┌─────────────────────────┐   │
│  │  ⭐ 升级会员             │   │
│  └─────────────────────────┘   │
└─────────────────────────────────┘
```

## 🐛 调试

启用调试日志：

```typescript
// 在 utils/api.ts 中查看设备ID
console.log('Device ID:', licenseApi.getDeviceId());

// 在 utils/membership.ts 中查看权限检查
const result = await checkFeatureAccess('my_feature');
console.log('Access check:', result);
```

## 📚 完整文档

详细文档请查看：
- [会员系统文档](./MEMBERSHIP.md) - 完整功能说明
- [Analytics 文档](./ANALYTICS.md) - 分析系统集成

## 🎉 完成！

现在你的插件已经拥有完整的会员系统，包括：
- ✅ 会员状态显示
- ✅ 购买入口（携带设备ID）
- ✅ 功能权限控制
- ✅ 使用限制管理
- ✅ 友好的 UI 组件

开始构建你的会员购买页面，让用户能够解锁更多功能吧！🚀
