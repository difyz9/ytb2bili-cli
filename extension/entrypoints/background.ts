import { defineBackground } from 'wxt/utils/define-background';
import type { MessagePayload, StoredUrl, StoredSubtitle, User } from '../types';
import { MessageType, ActionType } from '../types';
import { initAnalytics } from '../utils/analytics';
import { getCookiesForUrl } from '../utils/cookies';
import { authApi, checkLoginStatus, videoApi, type LoginInfo, type WebLoginGrantSession } from '../utils/api';

// 存储视频ID对应的pot参数
const browserApi: any = (globalThis as any).browser || (globalThis as any).chrome;
const potCache = new Map<string, string>();
const AUTH_FLOW_STORAGE_KEY = 'authFlowStatus';
const AUTH_FLOW_SESSION_KEY = 'authFlowSession';
const AUTH_FLOW_ALARM_NAME = 'ytb2bili-extension-auth-flow-poll';
const AUTH_FLOW_FAST_POLL_MS = 300;

type StoredAuthFlowSession = WebLoginGrantSession & {
  startedAt: number;
  consecutiveFailures: number;
};

let authFlowLocalTimer: ReturnType<typeof setTimeout> | null = null;
let authFlowPollInFlight = false;

async function setAuthFlowStatus(status: 'running' | 'success' | 'error', message: string, user?: User) {
  await browserApi.storage.local.set({
    [AUTH_FLOW_STORAGE_KEY]: {
      status,
      message,
      user,
      updatedAt: Date.now(),
    },
  });
}

async function getAuthFlowSession(): Promise<StoredAuthFlowSession | null> {
  const result = await browserApi.storage.local.get([AUTH_FLOW_SESSION_KEY]);
  return (result[AUTH_FLOW_SESSION_KEY] as StoredAuthFlowSession | undefined) || null;
}

async function saveAuthFlowSession(session: StoredAuthFlowSession) {
  await browserApi.storage.local.set({
    [AUTH_FLOW_SESSION_KEY]: session,
  });
}

async function clearAuthFlowSession() {
  await browserApi.storage.local.remove([AUTH_FLOW_SESSION_KEY]);
}

function clearAuthFlowLocalTimer() {
  if (authFlowLocalTimer) {
    clearTimeout(authFlowLocalTimer);
    authFlowLocalTimer = null;
  }
}

async function scheduleAuthFlowPoll(delayMs: number) {
  clearAuthFlowLocalTimer();
  authFlowLocalTimer = globalThis.setTimeout(() => {
    void pollAuthFlowOnce();
  }, Math.max(AUTH_FLOW_FAST_POLL_MS, delayMs));

  if (!browserApi.alarms?.create) {
    return;
  }

  await browserApi.alarms.clear(AUTH_FLOW_ALARM_NAME);
  await browserApi.alarms.create(AUTH_FLOW_ALARM_NAME, {
    when: Date.now() + Math.max(1000, delayMs),
  });
}

async function finalizeAuthFlowSuccess(loginInfo?: LoginInfo) {
  const user: User | undefined = loginInfo
    ? {
        id: String(loginInfo.user_id || loginInfo.mid || loginInfo.email || loginInfo.name || 'user'),
        name: loginInfo.name || loginInfo.uname || loginInfo.email || '已登录用户',
        mid: String(loginInfo.mid || loginInfo.user_id || ''),
        avatar: loginInfo.avatar || loginInfo.face || '',
      }
    : undefined;
  const userName = user?.name || loginInfo?.email || '已登录用户';
  clearAuthFlowLocalTimer();
  await clearAuthFlowSession();
  await browserApi.alarms?.clear?.(AUTH_FLOW_ALARM_NAME);
  await setAuthFlowStatus('success', `网页登录状态已同步到插件：${userName}`, user);
}

async function finalizeAuthFlowError(message: string) {
  clearAuthFlowLocalTimer();
  await clearAuthFlowSession();
  await browserApi.alarms?.clear?.(AUTH_FLOW_ALARM_NAME);
  await setAuthFlowStatus('error', message);
}

async function pollAuthFlowOnce() {
  if (authFlowPollInFlight) {
    return;
  }

  authFlowPollInFlight = true;

  const session = await getAuthFlowSession();
  if (!session) {
    clearAuthFlowLocalTimer();
    await browserApi.alarms?.clear?.(AUTH_FLOW_ALARM_NAME);
    authFlowPollInFlight = false;
    return;
  }

  if (Date.now() >= session.expiresAt) {
    await finalizeAuthFlowError('网页登录同步超时，请重新发起登录');
    authFlowPollInFlight = false;
    return;
  }

  try {
    const status = await authApi.getWebLoginGrantStatus(session.grantId, session.state);

    if (status.status === 'expired') {
      await finalizeAuthFlowError('网页登录同步超时，请重新发起登录');
      authFlowPollInFlight = false;
      return;
    }

    if (status.status === 'approved') {
      const loginInfo = await authApi.exchangeWebLoginGrant(session.grantId, session.state);

      if (session.tabId && browserApi.tabs?.remove) {
        try {
          await browserApi.tabs.remove(session.tabId);
        } catch {
          // ignore tab close failures
        }
      }

      await finalizeAuthFlowSuccess(loginInfo);
      authFlowPollInFlight = false;
      return;
    }

    await saveAuthFlowSession({
      ...session,
      consecutiveFailures: 0,
    });
    await scheduleAuthFlowPoll(session.pollIntervalMs);
  } catch (error) {
    const nextFailureCount = session.consecutiveFailures + 1;
    const message = error instanceof Error ? error.message : '网页登录同步失败，请重试';

    if (Date.now() >= session.expiresAt || nextFailureCount >= 5) {
      await finalizeAuthFlowError(message);
      return;
    }

    await saveAuthFlowSession({
      ...session,
      consecutiveFailures: nextFailureCount,
    });
    await setAuthFlowStatus('running', '已打开 ytb2bili-web 登录页，正在等待网页登录完成...');
    await scheduleAuthFlowPoll(Math.min(session.pollIntervalMs * nextFailureCount, 10_000));
  } finally {
    authFlowPollInFlight = false;
  }
}

async function startWebLoginFlow() {
  const existingSession = await getAuthFlowSession();
  if (existingSession && Date.now() < existingSession.expiresAt) {
    await setAuthFlowStatus('running', '网页登录同步进行中，请在网页登录页完成授权');
    void pollAuthFlowOnce();
    return;
  }

  await setAuthFlowStatus('running', '已打开 ytb2bili-web 登录页，正在等待网页登录完成...');

  try {
    const grant = await authApi.createWebLoginGrant();
    let createdTabId: number | undefined;

    if (browserApi.tabs?.create) {
      const tab = await browserApi.tabs.create({ url: grant.loginUrl });
      createdTabId = tab?.id;
    } else {
      globalThis.open(grant.loginUrl, '_blank');
    }

    await saveAuthFlowSession({
      ...grant,
      tabId: createdTabId,
      startedAt: Date.now(),
      consecutiveFailures: 0,
    });

    void pollAuthFlowOnce();
  } catch (error) {
    const message = error instanceof Error ? error.message : '网页登录同步失败，请重试';
    await finalizeAuthFlowError(message);
  }
}

export default defineBackground(() => {
  // 初始化分析客户端
  initAnalytics();

  // 监听网络请求以获取pot参数（仅在 Chrome/Edge 中可用）
  if (browserApi.webRequest && browserApi.webRequest.onBeforeRequest) {
    browserApi.webRequest.onBeforeRequest.addListener(
      (details: { url: string }) => {
        try {
          const url = new URL(details.url);
          const pot = url.searchParams.get('pot');
          const v = url.searchParams.get('v');

          if (pot && v) {
            potCache.set(v, pot);
          }
        } catch (error) {
          // 忽略错误
        }
        return undefined; // 不阻止请求
      },
      {
        urls: ['*://www.youtube.com/api/timedtext*'],
      }
    );
  }

  if (browserApi.alarms?.onAlarm) {
    browserApi.alarms.onAlarm.addListener((alarm: { name: string }) => {
      if (alarm.name === AUTH_FLOW_ALARM_NAME) {
        void pollAuthFlowOnce();
      }
    });
  }

  void getAuthFlowSession().then((session) => {
    if (session && Date.now() < session.expiresAt) {
      void scheduleAuthFlowPoll(1000);
    }
  });

  // 监听来自 content script 的消息
  browserApi.runtime.onMessage.addListener((message: any, sender: any, sendResponse: (response: unknown) => void) => {

    // 处理特殊的action格式（兼容原始扩展）
    if (message.action === 'saveUrl') {
      handleSaveUrlToBackend(message)
        .then((result) => sendResponse({ success: true, message: result }))
        .catch((error) => sendResponse({ success: false, error: error.message }));
      return true; // 表示异步响应
    }

    if (message.action === 'getPotParameter') {
      const pot = potCache.get(message.videoId);
      sendResponse({ pot: pot });
      return true;
    }

    if (message.action === 'storePotParameter') {
      if (message.videoId && message.pot) {
        potCache.set(message.videoId, message.pot);
      }
      return true;
    }

    // 获取 cookies
    if (message.action === 'getCookies') {
      getCookiesForUrl(message.url)
        .then((cookies) => {
          // 发送 JSON 格式的 cookies 数组，而不是 Header 格式
          const cookiesJson = JSON.stringify(cookies);
          sendResponse({ success: true, cookies: cookiesJson });
        })
        .catch((error) => {
          sendResponse({ success: false, cookies: '', error: error.message });
        });
      return true; // 异步响应
    }

    if (message.action === 'startWebLoginFlow') {
      void startWebLoginFlow()
        .then(() => sendResponse({ success: true, started: true }))
        .catch((error) => sendResponse({ success: false, error: error instanceof Error ? error.message : '无法启动网页登录同步' }));
      return true;
    }

    // 处理新的MessageType格式
    switch (message.type) {
      case MessageType.SAVE_URL:
        handleSaveUrl(message.data)
          .then((result) => sendResponse({ success: true, data: result }))
          .catch((error) => sendResponse({ success: false, error: error.message }));
        return true;



      case MessageType.GET_TRANSCRIPT:
        handleGetTranscript(message.data)
          .then((result) => sendResponse({ success: true, data: result }))
          .catch((error) => sendResponse({ success: false, error: error.message }));
        return true;

      default:
        sendResponse({ success: false, error: 'Unknown message type' });
        return true; // 改为 true 以保持一致性
    }
  });
});

/**
 * 保存 URL 到本地存储
 */
async function handleSaveUrl(data: { url: string; title: string }): Promise<StoredUrl> {
  const { url, title } = data;
  const timestamp = Date.now();
  
  const storedUrl: StoredUrl = {
    url,
    title,
    timestamp,
  };

  // 获取现有的 URLs
  const result = await browserApi.storage.local.get('savedUrls');
  const savedUrls: StoredUrl[] = (result.savedUrls as StoredUrl[] | undefined) || [];
  
  // 添加新的 URL
  savedUrls.unshift(storedUrl);
  
  // 限制保存数量（最多 100 个）
  if (savedUrls.length > 100) {
    savedUrls.pop();
  }
  
  // 保存到存储
  await browserApi.storage.local.set({ savedUrls });
  
  console.log('URL saved:', storedUrl);
  return storedUrl;
}



/**
 * 获取视频字幕（YouTube Transcript API）
 */
async function handleGetTranscript(data: { url: string }): Promise<any> {
  // 这里可以实现获取 YouTube 字幕的逻辑
  // 由于 YouTube Transcript API 通常需要在页面上下文中运行
  // 这个功能将在 content script 中实现
  throw new Error('Transcript fetching should be handled in content script');
}

/**
 * 保存URL到后端服务器（兼容原始扩展格式）
 * 使用统一的 videoApi.submitVideoData 方法
 */
async function handleSaveUrlToBackend(message: any): Promise<string> {
  try {
    const loginStatus = await checkLoginStatus();
    if (!loginStatus.isLoggedIn) {
      throw new Error(loginStatus.error || '请先登录插件账号后再提交视频');
    }

    console.log('保存视频信息到后端:', {
      url: message.url,
      title: message.title,
      description: message.description,
      operationType: message.operationType,
      subtitles: message.subtitles ? `${message.subtitles.length} 条字幕` : '无字幕',
      playlistId: message.playlistId,
    });

    // 转换为 VideoSubmissionData 格式
    const videoData = {
      platform: message.url.includes('youtube.com') || message.url.includes('youtu.be') ? 'youtube' as const : 'bilibili' as const,
      video_id: extractVideoId(message.url),
      title: message.title,
      description: message.description,
      url: message.url,
      subtitles: message.subtitles ? {
        title: message.title,
        language: 'auto',
        language_code: 'auto',
        content: message.subtitles
      } : undefined,
      timestamp: new Date().toISOString(),
      source: 'extension',
    };

    const result = await videoApi.submitVideoData(videoData);

    if (result.success) {
      const subtitleInfo = message.subtitles ? ` (含 ${message.subtitles.length} 条字幕)` : '';
      return `视频信息已保存到服务器${subtitleInfo}`;
    } else {
      throw new Error(result.message);
    }
  } catch (error: any) {
    console.error('保存到服务器失败:', error);
    throw new Error(error.message || '网络错误或服务器不可用');
  }
}

/**
 * 从 URL 提取视频 ID
 */
function extractVideoId(url: string): string {
  try {
    const urlObj = new URL(url);
    if (urlObj.hostname.includes('youtube.com')) {
      return urlObj.searchParams.get('v') || '';
    } else if (urlObj.hostname.includes('youtu.be')) {
      return urlObj.pathname.slice(1);
    } else if (urlObj.hostname.includes('bilibili.com')) {
      const match = urlObj.pathname.match(/\/video\/(BV\w+)/);
      return match ? match[1] : '';
    }
    return '';
  } catch {
    return '';
  }
}
