/**
 * Cookies 工具模块
 * 参考: https://github.com/kairi003/Get-cookies.txt-LOCALLY
 */

// 定义 Cookie 类型
interface Cookie {
  name: string;
  value: string;
  domain?: string;
  path?: string;
  secure?: boolean;
  httpOnly?: boolean;
  sameSite?: string;
  expirationDate?: number;
  storeId?: string;
}

interface GetAllDetails {
  url?: string;
  domain?: string;
  name?: string;
  storeId?: string;
  partitionKey?: any;
}

const browser: any = (globalThis as any).browser || (globalThis as any).chrome;

/**
 * 获取当前 Cookie Store ID
 */
async function getCurrentCookieStoreId(): Promise<string | undefined> {
  // 如果扩展处于分离隐身模式，返回 undefined 以选择默认存储
  if (browser.runtime.getManifest().incognito === 'split') return undefined;

  // Firefox 支持 tab.cookieStoreId 属性
  const [tab] = await browser.tabs.query({ active: true, currentWindow: true });
  
  // 如果没有找到活动标签页，返回 undefined 使用默认存储
  if (!tab) return undefined;
  
  if (tab.cookieStoreId) return tab.cookieStoreId;

  // Chrome 不支持 tab.cookieStoreId 属性
  const stores = await browser.cookies.getAllCookieStores();
  return stores.find((store: any) => store.tabIds.includes(tab.id))?.id;
}

/**
 * 获取所有匹配条件的 cookies
 */
async function getAllCookies(details: GetAllDetails): Promise<Cookie[]> {
  details.storeId = details.storeId || await getCurrentCookieStoreId();
  
  const { partitionKey, ...detailsWithoutPartitionKey } = details as any;
  
  // 处理不支持 partitionKey 的浏览器，例如 chrome < 119
  const cookiesWithPartitionKey = partitionKey
    ? await Promise.resolve()
        .then(() => browser.cookies.getAll(details))
        .catch(() => [])
    : [];
  
  const cookies = await browser.cookies.getAll(detailsWithoutPartitionKey);
  return [...cookies, ...cookiesWithPartitionKey];
}

function dedupeCookies(cookies: Cookie[]): Cookie[] {
  const seen = new Set<string>();
  const unique: Cookie[] = [];

  for (const cookie of cookies) {
    const key = [
      cookie.storeId || '',
      cookie.domain || '',
      cookie.path || '',
      cookie.name,
    ].join('\t');

    if (seen.has(key)) {
      continue;
    }

    seen.add(key);
    unique.push(cookie);
  }

  return unique;
}

function isYouTubeUrl(url: URL): boolean {
  return /(^|\.)youtube\.com$/i.test(url.hostname) || /^youtu\.be$/i.test(url.hostname);
}

/**
 * 获取指定URL的cookies
 */
export async function getCookiesForUrl(url: string): Promise<Cookie[]> {
  try {
    const urlObj = new URL(url);
    let cookies: Cookie[];

    if (isYouTubeUrl(urlObj)) {
      const domains = ['youtube.com', '.youtube.com', 'google.com', '.google.com'];
      const cookieGroups = await Promise.all(domains.map((domain) => getAllCookies({ domain })));
      cookies = dedupeCookies(cookieGroups.flat());
    } else {
      const details: GetAllDetails = {
        url: urlObj.href,
        // @ts-ignore - partitionKey 可能不存在于某些版本
        partitionKey: { topLevelSite: urlObj.origin },
      };
      cookies = dedupeCookies(await getAllCookies(details));
    }

    console.log(`[Cookies] 获取到 ${cookies.length} 个 cookies for ${urlObj.hostname}`);
    return cookies;
  } catch (error) {
    console.error('[Cookies] 获取失败:', error);
    return [];
  }
}

/**
 * 将 cookies 转换为 Netscape 格式字符串
 */
export function cookiesToNetscapeFormat(cookies: Cookie[]): string {
  const netscapeRows = cookies.map(({ domain, expirationDate, path, secure, name, value }) => {
    const includeSubDomain = domain?.startsWith('.') ? 'TRUE' : 'FALSE';
    const expiry = expirationDate?.toFixed() || '0';
    const secureFlag = secure ? 'TRUE' : 'FALSE';
    return [domain, includeSubDomain, path, secureFlag, expiry, name, value].join('\t');
  });

  return [
    '# Netscape HTTP Cookie File',
    '# https://curl.haxx.se/rfc/cookie_spec.html',
    '# This is a generated file! Do not edit.',
    '',
    ...netscapeRows,
    '', // 末尾添加一个空行
  ].join('\n');
}

/**
 * 将 cookies 转换为 Header 字符串格式
 */
export function cookiesToHeaderFormat(cookies: Cookie[]): string {
  return cookies.map(({ name, value }) => `${name}=${value}`).join('; ');
}

/**
 * 将 cookies 转换为简单对象格式
 */
export function cookiesToObject(cookies: Cookie[]): Record<string, string> {
  const result: Record<string, string> = {};
  cookies.forEach(({ name, value }) => {
    result[name] = value;
  });
  return result;
}
