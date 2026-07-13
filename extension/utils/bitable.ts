/**
 * 飞书多维表格集成模块
 * 用于将视频信息写入飞书多维表格
 */

export interface BitableConfig {
  appId?: string;
  appSecret?: string;
  appToken?: string;
  tableId?: string;
}

export interface VideoSubmitData {
  url: string;
  title: string;
  channel: string;
  videoId: string;
  cookies: string;
}

/**
 * 向多维表格写入视频提交记录（通过 background script）
 */
export async function submitToBitable(data: VideoSubmitData): Promise<{ success: boolean; recordId?: string; message: string }> {
  const browserApi: any = (globalThis as any).browser || (globalThis as any).chrome;
  
  return new Promise((resolve) => {
    browserApi.runtime.sendMessage(
      { action: 'submitToBitable', data },
      (response: any) => {
        resolve(response || { success: false, message: '无响应' });
      }
    );
  });
}

/**
 * 从存储中获取多维表格配置
 */
export async function getBitableConfig(): Promise<BitableConfig | null> {
  const browserApi: any = (globalThis as any).browser || (globalThis as any).chrome;
  if (!browserApi?.storage?.local) {
    return null;
  }

  const result = await browserApi.storage.local.get(['bitableConfig']);
  return result.bitableConfig || null;
}

/**
 * 保存多维表格配置
 */
export async function saveBitableConfig(config: BitableConfig): Promise<void> {
  const browserApi: any = (globalThis as any).browser || (globalThis as any).chrome;
  if (!browserApi?.storage?.local) {
    return;
  }

  await browserApi.storage.local.set({ bitableConfig: config });
}

/**
 * 清除多维表格配置
 */
export async function clearBitableConfig(): Promise<void> {
  const browserApi: any = (globalThis as any).browser || (globalThis as any).chrome;
  if (!browserApi?.storage?.local) {
    return;
  }

  await browserApi.storage.local.remove(['bitableConfig']);
}
