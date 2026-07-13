# 后端服务地址配置说明

## 概述

扩展现在拆分了两套地址：

- 会员服务地址走 `VITE_MEMBERSHIP_URL`，用于登录、注册、登出和获取用户信息。
- 视频提交地址走 `VITE_BACKEND_URL`，并且用户登录后可以在弹窗中修改该根地址。

提交前必须先通过会员服务完成认证，登录成功后会携带 Bearer token 请求用户当前配置根地址下自动拼接出的 `/api/v1/submit`。

## 配置文件位置

- `utils/config.ts`
- `.env.example`
- `.env.local`
- `wxt.config.ts`

## 当前配置

```typescript
export const BACKEND_CONFIG = {
  PRODUCTION_URL: 'https://stage.api2key.com',
  DEVELOPMENT_URL: 'http://localhost:8096',
  get BASE_URL(): string {
    return import.meta.env.VITE_BACKEND_URL || this.PRODUCTION_URL;
  },
} as const;

export const MEMBERSHIP_CONFIG = {
  PRODUCTION_URL: 'https://api.ap2ke.com/api/v1',
  get BASE_URL(): string {
    return import.meta.env.VITE_MEMBERSHIP_URL || this.PRODUCTION_URL;
  },
} as const;
```

说明：

- `getBackendUrl()` 返回视频提交根地址，优先读取用户在弹窗中保存的自定义地址，并自动兼容结尾 `/`、`/api/v1`、`/api/v1/submit`。
- `getMembershipUrl()` 返回会员服务基础地址，始终来自环境变量或默认值，不允许用户在弹窗中覆盖。
- `manifest.host_permissions` 需要覆盖默认线上域名。

## 当前使用的接口

### 登录状态

- `GET {VITE_MEMBERSHIP_URL}/auth/me`

### 用户登录与注册

- `POST {VITE_MEMBERSHIP_URL}/auth/login`
- `POST {VITE_MEMBERSHIP_URL}/auth/register`
- `POST {VITE_MEMBERSHIP_URL}/auth/logout`

### 直接提交视频

- `POST {用户自定义或 VITE_BACKEND_URL}/api/v1/submit`
- 请求头必须包含 `Authorization: Bearer <token>`

## 使用示例

```typescript
import { getBackendUrl } from './config';

async function submitVideo(payload: unknown, token: string) {
  const baseUrl = await getBackendUrl();
  const response = await fetch(`${baseUrl}/api/v1/submit`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify(payload),
  });

  return response.json();
}
```

```typescript
import { getMembershipUrl } from '../utils/config';

async function loginWithMembershipApi(email: string, password: string) {
  const baseUrl = getMembershipUrl();
  const response = await fetch(`${baseUrl}/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password }),
  });
  return response.json();
}
```

## 如何修改后端地址

### 开发时切换

1. 修改 `.env.local` 里的 `VITE_BACKEND_URL` 和 `VITE_MEMBERSHIP_URL`
2. 或直接修改 `utils/config.ts` 中的默认值
3. 重新构建扩展

### 用户自定义

用户可以在扩展设置中填写自定义视频提交根地址，例如 `https://example.com` 或 `http://127.0.0.1:8096`；保存后会优先使用 storage 中的地址。扩展会自动拼接 `/api/v1/submit`，该设置不会影响登录、注册和用户信息接口。

## 注意事项

- 扩展现在是“先登录或注册，后提交”；未登录时内容脚本、后台脚本和弹窗都会阻止提交。
- 如果用户修改了弹窗中的提交地址，只有视频提交会切换，认证仍然走会员服务地址。
- 如果更换为新的域名，记得同步更新 `wxt.config.ts` 中的 `host_permissions`。
- 修改地址后需要重新执行构建，例如 `yarn build`。
