import { AUTH_CONFIG, BACKEND_CONFIG, getBackendUrl, getMembershipUrl, getWebAppUrl } from './config';
import { encryptData, generateNonce, generateSignature } from './crypto';
import { getExtensionContextInvalidatedMessage, isExtensionContextInvalidated } from './extension-context';
import type { LicenseStatus, LoginTokenInfo, StoredLoginInfo, User } from '../types';
import type { LicenseType } from '../types';

export interface ApiResponse<T = any> {
  code?: number;
  message?: string;
  data?: T;
}

export interface LoginInfo extends StoredLoginInfo {
  user_id?: string;
  email?: string;
  name?: string;
  avatar?: string;
  mid?: number | string;
  uname?: string;
  face?: string;
  access_token?: string;
  refresh_token?: string;
  expires_in?: number;
}

export interface LoginStatusResult {
  isLoggedIn: boolean;
  user?: User;
  token?: string;
  error?: string;
}

type RequestOptions = RequestInit & {
  requireAuth?: boolean;
  baseUrl?: string;
};

interface AuthUserPayload {
  id: string;
  email: string;
  name?: string;
  avatar?: string | null;
}

interface AuthResponsePayload {
  user: AuthUserPayload;
  accessToken: string;
  refreshToken?: string;
  expiresIn?: number;
}

interface AuthMeResponsePayload {
  user: AuthUserPayload & {
    membership?: {
      tier?: string;
      endDate?: number | null;
    } | null;
  };
}

interface ProjectMembershipView {
  tier?: string;
  end_date?: number | null;
  is_active?: boolean;
}

interface ProjectMembershipResponse {
  membership?: ProjectMembershipView | null;
}

interface ExtensionGrantCreateResponse {
  grantId: string;
  state: string;
  status: 'pending' | 'approved' | 'consumed';
  expiresAt: number;
  pollIntervalMs?: number;
}

interface ExtensionGrantStatusResponse {
  grantId: string;
  status: 'pending' | 'approved' | 'consumed' | 'expired';
  expiresAt: number;
  approvedAt?: number | null;
  consumedAt?: number | null;
}

export interface WebLoginGrantSession {
  grantId: string;
  state: string;
  expiresAt: number;
  pollIntervalMs: number;
  loginUrl: string;
  tabId?: number;
}

const TIER_LEVELS: Record<LicenseType, number> = {
  free: 0,
  basic: 1,
  standard: 2,
  pro: 3,
  enterprise: 4,
};

type MembershipCandidate = {
  tier: LicenseType;
  endDate: number | null;
  source: 'global' | 'project';
};

async function getStoredLoginInfo(): Promise<LoginInfo | null> {
  const browserApi: any = (globalThis as any).browser || (globalThis as any).chrome;
  if (!browserApi?.storage?.local) {
    return null;
  }

  let result: Record<string, unknown>;
  try {
    result = await browserApi.storage.local.get(['loginInfo', 'isLoggedIn']);
  } catch (error) {
    if (isExtensionContextInvalidated(error)) {
      throw new Error(getExtensionContextInvalidatedMessage());
    }
    throw error;
  }
  const rawLoginInfo = result?.loginInfo as LoginInfo | null | undefined;
  if (!rawLoginInfo) {
    return null;
  }

  const normalizedLoginInfo = normalizeStoredLoginInfo(rawLoginInfo);
  const accessToken = extractAccessToken(normalizedLoginInfo);
  if (!accessToken) {
    return null;
  }

  if (!result?.isLoggedIn || normalizedLoginInfo !== rawLoginInfo) {
    await persistLoginInfo(normalizedLoginInfo);
  }

  return normalizedLoginInfo;
}

function extractAccessToken(loginInfo: LoginInfo | null): string {
  if (!loginInfo) {
    return '';
  }

  const tokenInfo = loginInfo.token_info as (LoginTokenInfo & {
    accessToken?: string;
    refreshToken?: string;
    expiresIn?: number;
  }) | undefined;

  return tokenInfo?.access_token
    || tokenInfo?.accessToken
    || loginInfo.access_token
    || loginInfo.accessToken
    || loginInfo.token
    || (loginInfo as LoginInfo & { jwt?: string }).jwt
    || '';
}

function normalizeStoredLoginInfo(loginInfo: LoginInfo): LoginInfo {
  const accessToken = extractAccessToken(loginInfo);
  const refreshToken = loginInfo.token_info?.refresh_token
    || loginInfo.token_info?.refreshToken
    || loginInfo.refresh_token
    || loginInfo.refreshToken
    || '';
  const expiresIn = loginInfo.token_info?.expires_in
    || loginInfo.token_info?.expiresIn
    || loginInfo.expires_in
    || 0;
  const normalizedTokenInfo: LoginTokenInfo = {
    ...loginInfo.token_info,
    uname: loginInfo.token_info?.uname || loginInfo.uname || loginInfo.name,
    face: loginInfo.token_info?.face || loginInfo.face || loginInfo.avatar,
    access_token: accessToken,
    refresh_token: refreshToken,
    expires_in: expiresIn,
  };

  const normalizedLoginInfo: LoginInfo = {
    ...loginInfo,
    access_token: accessToken,
    refresh_token: refreshToken,
    expires_in: expiresIn,
    token_info: normalizedTokenInfo,
  };

  const rawTokenInfo = loginInfo.token_info as Record<string, unknown> | undefined;
  const needsNormalization = loginInfo.access_token !== accessToken
    || loginInfo.refresh_token !== refreshToken
    || loginInfo.expires_in !== expiresIn
    || rawTokenInfo?.accessToken !== undefined
    || rawTokenInfo?.refreshToken !== undefined
    || rawTokenInfo?.expiresIn !== undefined
    || loginInfo.accessToken !== undefined
    || loginInfo.refreshToken !== undefined
    || loginInfo.token !== undefined
    || (loginInfo as LoginInfo & { jwt?: string }).jwt !== undefined;

  return needsNormalization ? normalizedLoginInfo : loginInfo;
}

function mapLoginInfoToUser(loginInfo: LoginInfo): User {
  const tokenInfo: LoginTokenInfo | undefined = loginInfo.token_info;
  const mid = loginInfo.mid ?? tokenInfo?.mid ?? '';
  const uname = loginInfo.name ?? loginInfo.uname ?? tokenInfo?.uname ?? loginInfo.email ?? '已登录用户';
  const face = loginInfo.avatar ?? loginInfo.face ?? tokenInfo?.face ?? '';
  return {
    id: String(loginInfo.user_id || mid || uname),
    name: uname,
    mid: String(mid || ''),
    avatar: face,
  };
}

function buildStoredLoginInfo(payload: AuthResponsePayload): LoginInfo {
  return {
    user_id: payload.user.id,
    email: payload.user.email,
    name: payload.user.name,
    avatar: payload.user.avatar || '',
    uname: payload.user.name,
    face: payload.user.avatar || '',
    access_token: payload.accessToken,
    refresh_token: payload.refreshToken || '',
    expires_in: payload.expiresIn || 0,
    login_time: Date.now(),
    token_info: {
      uname: payload.user.name,
      face: payload.user.avatar || '',
      access_token: payload.accessToken,
      refresh_token: payload.refreshToken || '',
      expires_in: payload.expiresIn || 0,
    },
  };
}

async function persistLoginInfo(loginInfo: LoginInfo) {
  const browserApi: any = (globalThis as any).browser || (globalThis as any).chrome;
  if (browserApi?.storage?.local) {
    try {
      await browserApi.storage.local.set({
        loginInfo,
        isLoggedIn: true,
      });
    } catch (error) {
      if (isExtensionContextInvalidated(error)) {
        throw new Error(getExtensionContextInvalidatedMessage());
      }
      throw error;
    }
  }
}

async function clearStoredLoginInfo() {
  const browserApi: any = (globalThis as any).browser || (globalThis as any).chrome;
  if (browserApi?.storage?.local) {
    try {
      await browserApi.storage.local.remove(['loginInfo', 'isLoggedIn']);
    } catch (error) {
      if (isExtensionContextInvalidated(error)) {
        throw new Error(getExtensionContextInvalidatedMessage());
      }
      throw error;
    }
  }
}

function getDeviceId(): string {
  const browserApi: any = (globalThis as any).browser || (globalThis as any).chrome;
  const userAgent = globalThis.navigator?.userAgent || 'unknown';
  const language = globalThis.navigator?.language || 'unknown';
  const seed = `${browserApi?.runtime?.id || 'extension'}:${userAgent}:${language}`;
  let hash = 0;
  for (let index = 0; index < seed.length; index += 1) {
    hash = ((hash << 5) - hash) + seed.charCodeAt(index);
    hash |= 0;
  }
  return `ext-${Math.abs(hash)}`;
}

function normalizePlatformBaseUrl(baseUrl: string): string {
  return baseUrl.trim().replace(/\/api\/v1\/?$/i, '').replace(/\/+$/, '');
}

class MembershipApiError extends Error {
  httpStatus: number;

  constructor(message: string, httpStatus: number) {
    super(message);
    this.name = 'MembershipApiError';
    this.httpStatus = httpStatus;
  }
}

function getMembershipApiBaseUrl(): string {
  return getMembershipUrl().trim().replace(/\/+$/, '');
}

function buildExtensionAuthBridgeUrl(grantId: string, state: string): string {
  const locale = globalThis.navigator?.language?.toLowerCase().startsWith('zh') ? 'zh' : 'en';
  const url = new URL(`/${locale}/extension-auth`, getWebAppUrl());
  url.searchParams.set('grantId', grantId);
  url.searchParams.set('state', state);
  url.searchParams.set('source', 'extension');
  return url.toString();
}

async function requestMembershipApi<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(`${getMembershipApiBaseUrl()}${path}`, {
    cache: 'no-store',
    ...init,
  });
  const payload = await response.json().catch(() => ({}));

  if (!response.ok) {
    const message = (payload as { message?: string }).message || `HTTP ${response.status}`;
    throw new MembershipApiError(message, response.status);
  }

  return ((payload as { data?: T }).data ?? payload) as T;
}

async function requestMembershipAuthApi<T>(
  path: string,
  accessToken?: string,
  init: RequestInit = {},
): Promise<T> {
  const headers = new Headers(init.headers);
  if (accessToken) {
    headers.set('Authorization', `Bearer ${accessToken}`);
  }

  return requestMembershipApi<T>(path, {
    ...init,
    headers,
  });
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

async function waitForStoredLoginInfo(userId?: string, attempts = 10, delayMs = 200): Promise<LoginInfo> {
  for (let attempt = 0; attempt < attempts; attempt += 1) {
    const storedLoginInfo = await getStoredLoginInfo();
    const storedAccessToken = extractAccessToken(storedLoginInfo);

    if (storedLoginInfo && storedAccessToken) {
      if (!userId || !storedLoginInfo.user_id || storedLoginInfo.user_id === userId) {
        return storedLoginInfo;
      }
    }

    await sleep(delayMs);
  }

  throw new Error('网页登录已完成，但插件本地登录状态尚未同步成功，请重新打开插件重试');
}

function toTimestamp(input: number | string | null | undefined): number | null {
  if (typeof input === 'number') {
    return Number.isFinite(input) ? input : null;
  }

  if (typeof input === 'string' && input.trim()) {
    const parsed = Date.parse(input);
    return Number.isFinite(parsed) ? parsed : null;
  }

  return null;
}

function isTierMember(tier: string | null | undefined): tier is LicenseType {
  return Boolean(tier && tier in TIER_LEVELS);
}

function isActiveMembership(tier: string | null | undefined, endDate: number | string | null | undefined): tier is LicenseType {
  if (!isTierMember(tier) || tier === 'free') {
    return false;
  }

  const expiresAt = toTimestamp(endDate);
  return expiresAt == null || expiresAt > Date.now();
}

function pickHigherTier(current: MembershipCandidate | null, candidate: MembershipCandidate): MembershipCandidate {
  if (!current) {
    return candidate;
  }

  const currentLevel = TIER_LEVELS[current.tier] ?? 0;
  const candidateLevel = TIER_LEVELS[candidate.tier] ?? 0;

  if (candidateLevel !== currentLevel) {
    return candidateLevel > currentLevel ? candidate : current;
  }

  return (candidate.endDate ?? 0) > (current.endDate ?? 0) ? candidate : current;
}

function getTierFeatures(tier: LicenseType): string[] {
  const baseFeatures = ['video_submit', 'subtitle_download'];

  if (tier === 'free') {
    return baseFeatures;
  }

  const memberFeatures = ['custom_backend_url', 'batch_submit', 'ai_translation'];
  if (tier === 'basic' || tier === 'standard') {
    return [...baseFeatures, ...memberFeatures];
  }
  if (tier === 'pro') {
    return [...baseFeatures, ...memberFeatures, 'auto_upload', 'priority_queue'];
  }

  return [...baseFeatures, ...memberFeatures, 'auto_upload', 'priority_queue', 'api_access', 'team_collaboration'];
}

async function resolveEffectiveMembership(accessToken: string): Promise<MembershipCandidate | null> {
  const [meResult, projectMembershipResult] = await Promise.all([
    requestMembershipAuthApi<AuthMeResponsePayload>(
      `/auth/me${AUTH_CONFIG.PROJECT_ID ? `?projectId=${encodeURIComponent(AUTH_CONFIG.PROJECT_ID)}` : ''}`,
      accessToken,
    ),
    AUTH_CONFIG.PROJECT_ID
      ? requestMembershipAuthApi<ProjectMembershipResponse>(
          `/projects/${encodeURIComponent(AUTH_CONFIG.PROJECT_ID)}/membership`,
          accessToken,
        ).catch(() => null)
      : Promise.resolve(null),
  ]);

  const candidates: MembershipCandidate[] = [];
  const globalMembership = meResult.user.membership;
  if (isActiveMembership(globalMembership?.tier, globalMembership?.endDate)) {
    candidates.push({
      tier: globalMembership.tier,
      endDate: toTimestamp(globalMembership.endDate),
      source: 'global',
    });
  }

  const projectMembership = projectMembershipResult?.membership;
  if (projectMembership?.is_active && isActiveMembership(projectMembership.tier, projectMembership.end_date)) {
    candidates.push({
      tier: projectMembership.tier as LicenseType,
      endDate: toTimestamp(projectMembership.end_date),
      source: 'project',
    });
  }

  return candidates.reduce<MembershipCandidate | null>((current, candidate) => pickHigherTier(current, candidate), null);
}

/**
 * API 请求封装
 */
async function request<T>(
  endpoint: string,
  options: RequestOptions = {}
): Promise<ApiResponse<T>> {
  const API_BASE_URL = options.baseUrl || await getBackendUrl();
  const url = `${API_BASE_URL}${endpoint}`;
  const headers = new Headers({
    'Content-Type': 'application/json',
  });

  if (options.headers) {
    new Headers(options.headers).forEach((value, key) => {
      headers.set(key, value);
    });
  }

  if (options.requireAuth) {
    const loginInfo = await getStoredLoginInfo();
    const accessToken = extractAccessToken(loginInfo);
    if (!accessToken) {
      throw new Error('请先登录插件账号后再提交视频');
    }

    const timestamp = Math.floor(Date.now() / 1000).toString();
    const nonce = generateNonce();
    const sign = await generateSignature({
      appId: AUTH_CONFIG.APP_ID,
      timestamp,
      nonce,
    }, AUTH_CONFIG.APP_SECRET);

    headers.set('X-App-Id', AUTH_CONFIG.APP_ID);
    headers.set('X-Timestamp', timestamp);
    headers.set('X-Nonce', nonce);
    headers.set('X-Sign', sign);
    headers.set('Authorization', `Bearer ${accessToken}`);
  }

	// A self-hosted ytb2bili server uses its own bearer token. When configured,
	// it intentionally takes precedence over the membership access token.
	if (BACKEND_CONFIG.API_TOKEN) {
		headers.set('Authorization', `Bearer ${BACKEND_CONFIG.API_TOKEN}`);
	}
  
  try {
    const response = await fetch(url, {
      ...options,
      headers,
    });

    if (!response.ok) {
      const payload = await response.json().catch(() => null) as ApiResponse<T> | null;
      throw new Error(payload?.message || `HTTP error! status: ${response.status}`);
    }

    const data = await response.json();
    return data;
  } catch (error) {
    throw error;
  }
}

async function getSerializedCookies(url: string): Promise<string> {
  const browserApi: any = (globalThis as any).browser || (globalThis as any).chrome;
  if (!browserApi?.runtime?.sendMessage) {
    return '';
  }

  const response = await browserApi.runtime.sendMessage({
    action: 'getCookies',
    url,
  }).catch((error: unknown) => {
    if (isExtensionContextInvalidated(error)) {
      throw new Error(getExtensionContextInvalidatedMessage());
    }
    throw error;
  });

  if (!response?.success || typeof response.cookies !== 'string') {
    return '';
  }

  return response.cookies;
}

async function buildCookiesMeta(url: string, serializedCookies?: string): Promise<string> {
  const cookiesPayload = serializedCookies?.trim() || await getSerializedCookies(url);
  if (!cookiesPayload) {
    return '';
  }

  return encryptData(cookiesPayload, AUTH_CONFIG.COOKIES_ENCRYPT_KEY);
}

// 提交视频数据的接口类型
export interface VideoSubmissionData {
  platform: 'youtube' | 'bilibili';
  video_id: string;
  title: string;
  description?: string;
  duration?: number;
  uploader_name?: string;
  uploader_id?: string;
  url: string;
  thumbnail_url?: string;
  subtitles?: {
    title: string;
    language: string;
    language_code: string;
    content: Array<{
      text: string;
      duration: number;
      offset: number;
      lang: string;
    }>;
  };
  timestamp?: string;
  source?: string;
  cookies?: string; // Optional serialized cookies JSON
}

export interface VideoSubmissionResponse {
  success: boolean;
  message: string;
  submission_id?: string;
  task_id?: string;
  data?: any;
}

/**
 * 视频相关 API - 只保留直接提交功能
 */
export const videoApi = {
  /**
   * 直接提交视频和字幕数据到后端
   */
  async submitVideoData(data: VideoSubmissionData): Promise<VideoSubmissionResponse> {
    try {
      // 验证必需字段
      if (!data.url || !data.video_id || !data.title) {
        throw new Error('缺少必需的视频信息：url、video_id 或 title');
      }

      let cookiesMeta = '';
      try {
        cookiesMeta = await buildCookiesMeta(data.url, data.cookies);
      } catch {
        cookiesMeta = '';
      }

      // 转换为后端期望的格式
      const backendData = {
        url: data.url,
        title: data.title,
        description: data.description || '',
        operationType: 'manual',
        subtitles: data.subtitles?.content || [], // 字幕为空时传空数组
        playlistId: '',
        timestamp: data.timestamp || new Date().toISOString(),
        savedAt: new Date().toISOString(),
        meta: cookiesMeta,
      };

      const loginInfo = await getStoredLoginInfo();
      const accessToken = extractAccessToken(loginInfo);
      const requireAuth = !!accessToken; // 只在有登录信息时才添加认证信息

      const response = await request<any>('/api/v1/submit', {
        method: 'POST',
        body: JSON.stringify(backendData),
        requireAuth,
      });

      const submissionId = response.data?.id
        || response.data?.videoId
        || response.data?.submittedVideoId
        || response.data?.video?.id
        || data.video_id;

      const isSuccess = response.code === undefined || response.code === 200;

      const result: VideoSubmissionResponse = isSuccess
        ? {
            success: true,
            message: response.message || '提交成功',
            submission_id: submissionId,
            task_id: submissionId,
            data: response.data,
          }
        : {
            success: false,
            message: response.message || '提交失败'
          };

      return result;
    } catch (error) {
      return {
        success: false,
        message: `提交失败: ${error instanceof Error ? error.message : String(error)}`
      };
    }
  },
};

export async function checkLoginStatus(): Promise<LoginStatusResult> {
  try {
    const loginInfo = await getStoredLoginInfo();
    const accessToken = extractAccessToken(loginInfo);
    if (!loginInfo || !accessToken) {
      return {
        isLoggedIn: false,
        error: '未登录，请先在插件中完成登录',
      };
    }

    try {
      const response = await requestMembershipAuthApi<AuthMeResponsePayload>(
        `/auth/me${AUTH_CONFIG.PROJECT_ID ? `?projectId=${encodeURIComponent(AUTH_CONFIG.PROJECT_ID)}` : ''}`,
        accessToken,
      );
      const userData = response.user || {};
      return {
        isLoggedIn: true,
        token: accessToken,
        user: {
          id: String(userData.id || loginInfo.mid || ''),
          name: userData.name || userData.email || loginInfo.uname || '已登录用户',
          mid: String(loginInfo.mid || userData.id || ''),
          avatar: userData.avatar || loginInfo.face || '',
        },
      };
    } catch (error) {
      if (error instanceof MembershipApiError && (error.httpStatus === 401 || error.httpStatus === 403)) {
        await clearStoredLoginInfo();
        return {
          isLoggedIn: false,
          error: '登录已过期，请重新登录',
        };
      }

      return {
        isLoggedIn: true,
        token: accessToken,
        user: mapLoginInfoToUser(loginInfo),
      };
    }
  } catch (error) {
    if (isExtensionContextInvalidated(error)) {
      return {
        isLoggedIn: false,
        error: getExtensionContextInvalidatedMessage(),
      };
    }
    return {
      isLoggedIn: false,
      error: error instanceof Error ? error.message : '检查登录状态失败',
    };
  }
}

export const authApi = {
  async getStoredLoginInfo() {
    return getStoredLoginInfo();
  },
  async createWebLoginGrant(): Promise<WebLoginGrantSession> {
    const browserApi: any = (globalThis as any).browser || (globalThis as any).chrome;
    const state = crypto.randomUUID();
    const grant = await requestMembershipApi<ExtensionGrantCreateResponse>('/auth/extension/grants', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        state,
        deviceId: getDeviceId(),
        extensionId: browserApi?.runtime?.id || undefined,
        source: 'extension_popup',
        projectId: AUTH_CONFIG.PROJECT_ID || undefined,
      }),
    });

    return {
      grantId: grant.grantId,
      state: grant.state,
      expiresAt: grant.expiresAt,
      pollIntervalMs: grant.pollIntervalMs ?? 750,
      loginUrl: buildExtensionAuthBridgeUrl(grant.grantId, grant.state),
    };
  },
  async getWebLoginGrantStatus(grantId: string, state: string): Promise<ExtensionGrantStatusResponse> {
    return requestMembershipApi<ExtensionGrantStatusResponse>(
      `/auth/extension/grants/${encodeURIComponent(grantId)}?state=${encodeURIComponent(state)}`,
    );
  },
  async exchangeWebLoginGrant(grantId: string, state: string): Promise<LoginInfo> {
    const payload = await requestMembershipApi<AuthResponsePayload>(
      `/auth/extension/grants/${encodeURIComponent(grantId)}/exchange`,
      {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ state }),
      }
    );

    const loginInfo = buildStoredLoginInfo(payload);
    await persistLoginInfo(loginInfo);
    return loginInfo;
  },
  async startWebLogin() {
    const browserApi: any = (globalThis as any).browser || (globalThis as any).chrome;
    const grant = await this.createWebLoginGrant();
    let createdTabId: number | undefined;
    if (browserApi?.tabs?.create) {
      const tab = await browserApi.tabs.create({ url: grant.loginUrl });
      createdTabId = tab?.id;
    } else {
      globalThis.open(grant.loginUrl, '_blank');
    }

    while (Date.now() < grant.expiresAt) {
      await sleep(grant.pollIntervalMs);

      const status = await this.getWebLoginGrantStatus(grant.grantId, grant.state);

      if (status.status === 'expired') {
        break;
      }

      if (status.status === 'approved') {
        const syncedLoginInfo = await this.exchangeWebLoginGrant(grant.grantId, grant.state);

        if (createdTabId && browserApi?.tabs?.remove) {
          try {
            await browserApi.tabs.remove(createdTabId);
          } catch {
            // ignore tab close failures
          }
        }

        return syncedLoginInfo;
      }
    }

    throw new Error('网页登录同步超时，请重新发起登录');
  },
  async login(email: string, password: string) {
    const payload = await requestMembershipApi<AuthResponsePayload>('/auth/login', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        email,
        password,
        projectId: AUTH_CONFIG.PROJECT_ID || undefined,
      }),
    });
    if (!payload?.accessToken || !payload?.user) {
      throw new Error('登录响应缺少必要字段');
    }
    const loginInfo = buildStoredLoginInfo(payload);
    await persistLoginInfo(loginInfo);
    return loginInfo;
  },
  async register(email: string, password: string, name?: string) {
    const requestBody: Record<string, unknown> = { email, password };
    if (name?.trim()) {
      requestBody.name = name.trim();
    }
    if (AUTH_CONFIG.PROJECT_ID) {
      requestBody.projectId = AUTH_CONFIG.PROJECT_ID;
    }
    const payload = await requestMembershipApi<AuthResponsePayload>('/auth/register', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(requestBody),
    });
    if (!payload?.accessToken || !payload?.user) {
      throw new Error('注册响应缺少必要字段');
    }
    const loginInfo = buildStoredLoginInfo(payload);
    await persistLoginInfo(loginInfo);
    return loginInfo;
  },
  async checkLoginStatus() {
    return checkLoginStatus();
  },
  async logout() {
    const loginInfo = await getStoredLoginInfo();
    const accessToken = extractAccessToken(loginInfo);
    if (accessToken) {
      try {
        await requestMembershipAuthApi('/auth/logout', accessToken, {
          method: 'POST',
        });
      } catch {
        // 忽略远端登出失败，本地状态仍需清理
      }
    }
    await clearStoredLoginInfo();
  },
};

export const licenseApi = {
  getDeviceId,
  async verifyLicense(): Promise<LicenseStatus | null> {
    const loginStatus = await checkLoginStatus();
    if (!loginStatus.isLoggedIn || !loginStatus.token) {
      return {
        is_valid: false,
        license_type: 'free',
        device_id: getDeviceId(),
        features: [],
      };
    }

    let membership: MembershipCandidate | null = null;
    try {
      membership = await resolveEffectiveMembership(loginStatus.token);
    } catch (error) {
      console.error('Failed to resolve membership tier:', error);
    }

    const licenseType = membership?.tier ?? 'free';
    return {
      is_valid: true,
      license_type: licenseType,
      device_id: getDeviceId(),
      expired_at: membership?.endDate ? new Date(membership.endDate).toISOString() : undefined,
      features: getTierFeatures(licenseType),
    };
  },
  async openMembershipPage(source = 'extension') {
    const browserApi: any = (globalThis as any).browser || (globalThis as any).chrome;
    const membershipUrl = new URL('/zh/membership', getWebAppUrl());
    membershipUrl.searchParams.set('source', source);
    membershipUrl.searchParams.set('device_id', getDeviceId());
    if (browserApi?.tabs?.create) {
      await browserApi.tabs.create({ url: membershipUrl.toString() });
      return;
    }
    globalThis.open(membershipUrl.toString(), '_blank');
  },
};
