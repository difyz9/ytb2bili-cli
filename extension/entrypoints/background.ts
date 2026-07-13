import { defineBackground } from 'wxt/utils/define-background';
import type { MessagePayload, StoredUrl, StoredSubtitle, User } from '../types';
import { MessageType, ActionType } from '../types';
import { initAnalytics } from '../utils/analytics';
import { getCookiesForUrl } from '../utils/cookies';
import { getBitableConfig } from '../utils/bitable';

// 存储视频ID对应的pot参数
const browserApi: any = (globalThis as any).browser || (globalThis as any).chrome;
const potCache = new Map<string, string>();

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

  // 监听来自 content script 的消息
  browserApi.runtime.onMessage.addListener((message: any, sender: any, sendResponse: (response: unknown) => void) => {

    // 提交到多维表格
    if (message.action === 'submitToBitable') {
      handleSubmitToBitable(message.data)
        .then((result) => sendResponse(result))
        .catch((error) => sendResponse({ success: false, error: error.message }));
      return true;
    }

    // 获取 cookies
    if (message.action === 'getCookies') {
      getCookiesForUrl(message.url)
        .then((cookies) => {
          const cookiesJson = JSON.stringify(cookies);
          sendResponse({ success: true, cookies: cookiesJson });
        })
        .catch((error) => {
          sendResponse({ success: false, cookies: '', error: error.message });
        });
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
        return true;
    }
  });
});

/**
 * 提交视频数据到飞书多维表格
 */
async function handleSubmitToBitable(data: {
  url: string;
  title: string;
  channel: string;
  videoId: string;
  cookies: string;
}): Promise<{ success: boolean; recordId?: string; message: string }> {
  const config = await getBitableConfig();
  
  if (!config?.appId || !config?.appSecret) {
    // 没有飞书配置时，降级提交到本地 HTTP 服务
    return handleSubmitToLocal(data);
  }

  if (!config?.appToken || !config?.tableId) {
    // 没有飞书配置时，降级提交到本地 HTTP 服务
    return handleSubmitToLocal(data);
  }

  // 获取 access token
  const tokenResponse = await fetch('https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      app_id: config.appId,
      app_secret: config.appSecret,
    }),
  });

  const tokenData = await tokenResponse.json();
  if (tokenData.code !== 0) {
    return { success: false, message: tokenData.msg || '获取 Token 失败' };
  }

  const token = tokenData.tenant_access_token;

  // 构造记录数据
  const timestamp = Date.now();
  const record = {
    fields: {
      "链接": { link: data.url, text: data.title },
      "标题": data.title,
      "频道": data.channel,
      "视频ID": data.videoId,
      "Cookies": data.cookies,
      "状态": "pending",
      "创建时间": timestamp,
      "更新时间": timestamp,
    }
  };

  // 写入多维表格
  const bitableUrl = `https://open.feishu.cn/open-apis/bitable/v1/apps/${config.appToken}/tables/${config.tableId}/records`;
  const bearerPrefix = 'Bearer ';
  const authHeader = bearerPrefix + token;
  const response = await fetch(bitableUrl, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': authHeader,
    },
    body: JSON.stringify(record),
  });

  const result = await response.json();
  
  if (result.code === 0) {
    return {
      success: true,
      recordId: result.data?.record?.record_id,
      message: '提交成功'
    };
  } else {
    return { success: false, message: result.msg || '提交失败' };
  }
}

/**
 * 降级提交到本地 HTTP 服务 (http://localhost:8096)
 */
async function handleSubmitToLocal(data: {
  url: string;
  title: string;
  channel: string;
  videoId: string;
  cookies: string;
}): Promise<{ success: boolean; recordId?: string; message: string }> {
  const BACKEND_URL = 'http://localhost:8096/api/v1/submit';
  try {
    const response = await fetch(BACKEND_URL, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        url: data.url,
        title: data.title,
        description: `[${data.channel}] ${data.title}`,
        operationType: 'manual',
        subtitles: [],
        playlistId: '',
        timestamp: new Date().toISOString(),
        savedAt: new Date().toISOString(),
        meta: data.cookies || '',
      }),
    });
    if (!response.ok) {
      const text = await response.text();
      return { success: false, message: `HTTP ${response.status}: ${text}` };
    }
    const result = await response.json();
    return {
      success: true,
      recordId: result.task_id || result.id,
      message: '✅ 已提交到本地 ytb2bili 服务',
    };
  } catch (error) {
    return {
      success: false,
      message: `❌ 连接本地服务失败: ${error instanceof Error ? error.message : String(error)}。请确保已执行 \`ytb start\` 启动服务`,
    };
  }
}

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

  const result = await browserApi.storage.local.get('savedUrls');
  const savedUrls: StoredUrl[] = (result.savedUrls as StoredUrl[] | undefined) || [];
  
  savedUrls.unshift(storedUrl);
  
  if (savedUrls.length > 100) {
    savedUrls.pop();
  }
  
  await browserApi.storage.local.set({ savedUrls });
  
  console.log('URL saved:', storedUrl);
  return storedUrl;
}

/**
 * 获取视频字幕
 */
async function handleGetTranscript(data: { url: string }): Promise<any> {
  throw new Error('Transcript fetching should be handled in content script');
}
