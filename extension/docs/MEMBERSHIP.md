# 会员系统集成文档

本项目已集成完整的会员系统，支持功能权限控制和使用限制。

## 功能概述

### 会员等级

- **免费版** - 基础功能，每日 5 个视频
- **基础版** - 批量提交、AI 翻译，每日 20 个视频
- **专业版** - 优先队列、自动上传，每日 100 个视频
- **企业版** - API 访问、团队协作，无限制

## 配置

### 1. 环境变量

在 `.env.local` 中配置会员购买页面地址：

```env
# 会员购买页面地址
VITE_MEMBERSHIP_URL=https://your-domain.com/membership
```

### 2. 购买流程

当用户点击"升级会员"按钮时：

1. 系统自动获取设备唯一标识（device_id）
2. 构造包含以下参数的 URL：
   - `device_id`: 设备唯一标识
   - `source`: 来源标识（popup/content/etc）
   - `product`: 产品名称（ytb2bili-extension）
   - `timestamp`: 时间戳

示例 URL：
```
https://your-domain.com/membership?device_id=abc123&source=popup&product=ytb2bili-extension&timestamp=1234567890
```

3. 在新标签页打开购买页面
4. 用户完成购买后，通过 device_id 激活 License

## 组件使用

### MembershipButton - 会员购买按钮

```tsx
import { MembershipButton } from '../components/MembershipComponents';

<MembershipButton 
  source="popup"  // 来源标识
  fullWidth={true}  // 是否全宽
  className="my-custom-class"
/>
```

### LicenseStatusBadge - 会员状态徽章

```tsx
import { LicenseStatusBadge } from '../components/MembershipComponents';

<LicenseStatusBadge 
  licenseType="pro"  // free | basic | pro | enterprise
  isValid={true}
/>
```

### MembershipCard - 会员卡片

```tsx
import { MembershipCard } from '../components/MembershipComponents';

<MembershipCard 
  licenseStatus={status}  // 可选，不传则自动加载
  className="my-custom-class"
/>
```

## API 使用

### 获取设备ID

```typescript
import { licenseApi } from './utils/api';

const deviceId = licenseApi.getDeviceId();
console.log('Device ID:', deviceId);
```

### 验证 License 状态

```typescript
const status = await licenseApi.verifyLicense();

if (status && status.is_valid) {
  console.log('License 有效');
  console.log('类型:', status.license_type);
  console.log('可用功能:', status.features);
} else {
  console.log('License 无效或已过期');
}
```

### 打开会员购买页面

```typescript
// 在新标签页打开购买页面，自动携带设备ID
await licenseApi.openMembershipPage('popup');
```

## 功能权限控制

### 检查功能访问权限

```typescript
import { checkFeatureAccess } from './utils/membership';

const result = await checkFeatureAccess('ai_translation');

if (!result.allowed) {
  alert(result.reason);  // "此功能需要基础版或更高会员"
  // 提示用户升级
  await licenseApi.openMembershipPage('feature_gate');
  return;
}

// 继续执行功能...
```

### 在功能执行前检查

```typescript
import { checkBeforeFeatureUse } from './utils/membership';

async function submitVideo() {
  // 检查权限和使用限制
  const check = await checkBeforeFeatureUse('video_submit');
  
  if (!check.allowed) {
    alert(check.reason);
    if (check.shouldUpgrade) {
      await licenseApi.openMembershipPage('usage_limit');
    }
    return;
  }

  // 执行视频提交
  // ...
  
  // 记录使用次数
  await usageTracker.incrementUsage('video_submit');
}
```

### 获取功能限制配置

```typescript
import { getFeatureLimits } from './utils/membership';

const limits = await getFeatureLimits();

console.log('每日视频限制:', limits.maxVideosPerDay);
console.log('批量大小限制:', limits.maxBatchSize);
console.log('优先处理:', limits.priorityProcessing);
console.log('AI 翻译:', limits.aiTranslation);
```

### 检查使用限制

```typescript
import { checkUsageLimit, usageTracker } from './utils/membership';

const currentUsage = await usageTracker.getUsage('video_submit');
const limitCheck = await checkUsageLimit('maxVideosPerDay', currentUsage);

if (!limitCheck.allowed) {
  alert(`今日已达到限制 (${limitCheck.limit})，升级会员可提高限额`);
  return;
}

console.log('剩余次数:', limitCheck.remaining);
```

## 功能权限配置

在 `utils/membership.ts` 中配置功能权限：

```typescript
const FEATURE_PERMISSIONS: Record<string, LicenseType[]> = {
  // 基础功能 - 所有用户
  'video_submit': ['free', 'basic', 'pro', 'enterprise'],
  
  // 高级功能 - 付费用户
  'batch_submit': ['basic', 'pro', 'enterprise'],
  'ai_translation': ['basic', 'pro', 'enterprise'],
  
  // 专业功能
  'auto_upload': ['pro', 'enterprise'],
  'priority_queue': ['pro', 'enterprise'],
  
  // 企业功能
  'api_access': ['enterprise'],
};
```

## 使用示例

### 示例 1: 视频提交页面

```tsx
import { checkBeforeFeatureUse, usageTracker } from '../utils/membership';
import { licenseApi } from '../utils/api';

function VideoSubmitPage() {
  const handleSubmit = async () => {
    // 检查权限
    const check = await checkBeforeFeatureUse('video_submit');
    
    if (!check.allowed) {
      if (confirm(`${check.reason}\n是否立即升级会员？`)) {
        await licenseApi.openMembershipPage('submit_gate');
      }
      return;
    }

    // 提交视频
    const result = await submitVideo();
    
    if (result.success) {
      // 增加使用计数
      await usageTracker.incrementUsage('video_submit');
      alert('提交成功！');
    }
  };

  return (
    <button onClick={handleSubmit}>
      提交视频
    </button>
  );
}
```

### 示例 2: 批量操作功能

```tsx
import { checkFeatureAccess } from '../utils/membership';
import { MembershipButton } from '../components/MembershipComponents';

function BatchSubmitFeature() {
  const [hasAccess, setHasAccess] = useState(false);

  useEffect(() => {
    checkFeatureAccess('batch_submit').then(result => {
      setHasAccess(result.allowed);
    });
  }, []);

  if (!hasAccess) {
    return (
      <div className="feature-locked">
        <p>批量提交功能需要付费会员</p>
        <MembershipButton source="batch_feature" />
      </div>
    );
  }

  return (
    <div className="batch-submit">
      {/* 批量提交功能界面 */}
    </div>
  );
}
```

### 示例 3: 显示剩余配额

```tsx
import { getFeatureLimits, usageTracker } from '../utils/membership';

function UsageQuota() {
  const [usage, setUsage] = useState(0);
  const [limit, setLimit] = useState(0);

  useEffect(() => {
    loadQuota();
  }, []);

  const loadQuota = async () => {
    const limits = await getFeatureLimits();
    const currentUsage = await usageTracker.getUsage('video_submit');
    
    setLimit(limits.maxVideosPerDay || 0);
    setUsage(currentUsage);
  };

  return (
    <div className="quota-display">
      <p>今日已使用: {usage} / {limit === -1 ? '∞' : limit}</p>
      <div className="progress-bar">
        <div 
          className="progress" 
          style={{ width: `${(usage / limit) * 100}%` }}
        />
      </div>
    </div>
  );
}
```

## 后端集成

### License 验证接口

后端需要提供 License 验证接口，示例响应：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "is_valid": true,
    "license_type": "pro",
    "device_id": "abc123",
    "expired_at": "2025-12-31T23:59:59Z",
    "features": [
      "批量提交",
      "AI 翻译",
      "自动上传",
      "优先队列"
    ]
  }
}
```

### 购买成功后的处理

1. 用户在购买页面完成支付
2. 后端根据 device_id 激活 License
3. 用户返回扩展后，系统自动验证并更新状态

## 注意事项

1. **设备ID唯一性**: 设备ID使用 Analytics 客户端生成，确保唯一性和持久性
2. **本地缓存**: 使用限制数据存储在本地，每日自动重置
3. **异步验证**: License 验证是异步的，需要处理加载状态
4. **错误处理**: 网络错误时应该提供降级方案
5. **用户体验**: 功能受限时应该给出清晰的提示和升级入口

## 扩展功能

### 添加新功能权限

1. 在 `FEATURE_PERMISSIONS` 中添加功能定义
2. 在 UI 中使用 `checkFeatureAccess` 检查权限
3. 在功能执行前使用 `checkBeforeFeatureUse` 验证

### 自定义会员等级

在 `types/index.ts` 中扩展 `LicenseType`：

```typescript
export type LicenseType = 'free' | 'basic' | 'pro' | 'enterprise' | 'vip';
```

然后在 `membership.ts` 中添加对应配置。

## 测试

### 本地测试

```bash
# 启动开发服务器
npm run dev

# 在浏览器中加载扩展
# 打开 chrome://extensions/
# 启用开发者模式
# 加载解压的扩展
```

### 测试不同会员等级

修改 Analytics 服务器返回的 License 状态来测试不同等级的功能。

## 相关文档

- [Analytics 集成文档](./ANALYTICS.md)
- [组件文档](../components/README.md)
- [API 文档](../utils/README.md)
