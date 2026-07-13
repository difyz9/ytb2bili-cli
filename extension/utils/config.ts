/// <reference path="../types/globals.d.ts" />

/**
 * 全局配置管理
 * 统一管理后端服务地址等配置信息
 */

/**
 * 飞书机器人配置
 */
export const FEISHU_CONFIG = {
  // 飞书应用 ID
  get APP_ID(): string {
    return import.meta.env.VITE_FEISHU_APP_ID || 'cli_aaa921692978dce6';
  },
  
  // 飞书应用 Secret (注意: 生产环境应通过后端代理，不应暴露在前端)
  get APP_SECRET(): string {
    return import.meta.env.VITE_FEISHU_APP_SECRET || '';
  },
} as const;

/**
 * 后端服务配置
 */
export const BACKEND_CONFIG = {
  // 生产环境后端地址
  PRODUCTION_URL: 'https://stage.api2key.com',
  
  // 开发环境后端地址
  DEVELOPMENT_URL: 'http://localhost:8096',
  
  // 当前使用的后端地址（可以根据环境变量切换）
  get BASE_URL(): string {
    return import.meta.env.VITE_BACKEND_URL || this.PRODUCTION_URL;
  },
} as const;

/**
 * 会员服务配置
 */
export const MEMBERSHIP_CONFIG = {
  PRODUCTION_URL: 'https://stage.api2key.com/api/v1',

  get BASE_URL(): string {
    return import.meta.env.VITE_MEMBERSHIP_URL || this.PRODUCTION_URL;
  },
} as const;

/**
 * Web 前台配置
 */
export const WEB_APP_CONFIG = {
  PRODUCTION_URL: 'https://ytb2bili.com',

  get BASE_URL(): string {
    return import.meta.env.VITE_WEB_APP_URL || this.PRODUCTION_URL;
  },
} as const;

export const VIDEO_SUBMIT_PATH = '/api/v1/submit';

function normalizeBaseUrl(url: string): string {
  return url.trim().replace(/\/$/, '');
}

export function normalizeBackendBaseUrl(url: string): string {
  return normalizeBaseUrl(url)
    .replace(/\/api\/v\d+\/submit$/i, '')
    .replace(/\/api\/v\d+$/i, '');
}

/**
 * API 认证配置
 */
export const AUTH_CONFIG = {
  APP_ID: import.meta.env.VITE_APP_ID || 'ytb2bili_extension',
  PROJECT_ID: import.meta.env.VITE_PROJECT_ID || '',
  APP_SECRET: import.meta.env.VITE_APP_SECRET || 'ytb2bili_secret_2026',
  COOKIES_ENCRYPT_KEY: import.meta.env.VITE_COOKIES_ENCRYPT_KEY || '59e7052041ce4bd6aff82f6a0bca9cde',
} as const;

/**
 * 从 storage 获取后端 URL（支持用户自定义）
 * 如果用户在设置中配置了自定义地址，优先使用用户配置
 */
export async function getBackendUrl(): Promise<string> {
  const browserApi: any = (globalThis as any).browser || (globalThis as any).chrome;
  if (browserApi?.storage) {
    try {
      const result = await browserApi.storage.local.get('backendUrl');
      const customUrl = result.backendUrl as string | undefined;
      
      if (customUrl) {
        return normalizeBackendBaseUrl(customUrl);
      }
    } catch (error) {
      // 使用默认 URL
    }
  }
  
  // 返回默认配置
  return normalizeBackendBaseUrl(BACKEND_CONFIG.BASE_URL);
}

/**
 * 保存后端 URL 到 storage
 */
export async function saveBackendUrl(url: string): Promise<void> {
  const browserApi: any = (globalThis as any).browser || (globalThis as any).chrome;
  if (!browserApi?.storage?.local) {
    return;
  }

  await browserApi.storage.local.set({ backendUrl: normalizeBackendBaseUrl(url) });
}

/**
 * 清除后端 URL 配置
 */
export async function clearBackendUrl(): Promise<void> {
  const browserApi: any = (globalThis as any).browser || (globalThis as any).chrome;
  if (!browserApi?.storage?.local) {
    return;
  }

  await browserApi.storage.local.remove(['backendUrl']);
}

/**
 * 获取会员服务地址，用于登录、注册、用户信息查询
 */
export function getMembershipUrl(): string {
  return normalizeBaseUrl(MEMBERSHIP_CONFIG.BASE_URL);
}

/**
 * 获取网页登录前台地址
 */
export function getWebAppUrl(): string {
  return normalizeBaseUrl(WEB_APP_CONFIG.BASE_URL);
}

/**
 * 获取基础后端地址（不含 /api/v1）
 * @deprecated 使用 getBackendUrl() 代替
 */
export const getBackendBaseUrl = getBackendUrl;

/**
 * 默认配置值
 */
export const DEFAULT_CONFIG = {
  BACKEND_URL: BACKEND_CONFIG.BASE_URL,
  BACKEND_BASE_URL: BACKEND_CONFIG.BASE_URL,
  MEMBERSHIP_URL: MEMBERSHIP_CONFIG.BASE_URL,
  WEB_APP_URL: WEB_APP_CONFIG.BASE_URL,
} as const;
