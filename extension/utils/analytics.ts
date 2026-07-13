/**
 * 分析客户端工具模块 - 封装 @difyz/ts-analysis-client
 */

import Analytics from '@difyz/ts-analysis-client';
import type { User } from '../types';

// 分析服务器配置
const ANALYTICS_CONFIG = {
  serverUrl: 'https://go-analysis-proxy.vercel.app/proxy',
  productName: 'ytb2bili-extension',
  debug: false,
  batchSize: 10,
  flushInterval: 5000,
};

/**
 * 初始化分析客户端
 * 应该在应用启动时调用一次
 */
export function initAnalytics() {
  try {
    Analytics.initialize(ANALYTICS_CONFIG);
    console.log('[Analytics] 分析客户端已初始化');
  } catch (error) {
    console.error('[Analytics] 初始化失败:', error);
  }
}

/**
 * 推送用户登录事件
 * @param user 用户信息
 */
export async function trackUserLogin(user: User) {
  try {
    await Analytics.track('user_login', {
      user_id: user.id,
      user_mid: user.mid,
      username: user.name,
      avatar: user.avatar,
      level: user.level,
      fans: user.fans,
      attention: user.attention,
      timestamp: new Date().toISOString(),
      platform: 'bilibili',
    });
    console.log('[Analytics] 用户登录事件已推送:', user);
  } catch (error) {
    console.error('[Analytics] 推送用户登录事件失败:', error);
  }
}

/**
 * 推送用户状态数据
 * @param statusData 从 getUserStatus 获取的完整状态数据
 */
export async function trackUserStatus(statusData: {
  code: number;
  message?: string;
  is_logged_in: boolean;
  user?: User;
}) {
  try {
    await Analytics.track('user_status_check', {
      is_logged_in: statusData.is_logged_in,
      user_id: statusData.user?.id,
      user_mid: statusData.user?.mid,
      username: statusData.user?.name,
      avatar: statusData.user?.avatar,
      level: statusData.user?.level,
      fans: statusData.user?.fans,
      attention: statusData.user?.attention,
      code: statusData.code,
      message: statusData.message,
      timestamp: new Date().toISOString(),
    });
    console.log('[Analytics] 用户状态数据已推送');
  } catch (error) {
    console.error('[Analytics] 推送用户状态数据失败:', error);
  }
}

/**
 * 推送用户退出登录事件
 */
export async function trackUserLogout() {
  try {
    await Analytics.track('user_logout', {
      timestamp: new Date().toISOString(),
    });
    console.log('[Analytics] 用户退出登录事件已推送');
  } catch (error) {
    console.error('[Analytics] 推送用户退出登录事件失败:', error);
  }
}

/**
 * 推送视频提交事件
 * @param videoData 视频数据
 */
export async function trackVideoSubmission(videoData: {
  video_id: string;
  title: string;
  platform: string;
  success: boolean;
}) {
  try {
    await Analytics.track('video_submission', {
      video_id: videoData.video_id,
      title: videoData.title,
      platform: videoData.platform,
      success: videoData.success,
      timestamp: new Date().toISOString(),
    });
    console.log('[Analytics] 视频提交事件已推送');
  } catch (error) {
    console.error('[Analytics] 推送视频提交事件失败:', error);
  }
}

/**
 * 推送页面浏览事件
 * @param pagePath 页面路径
 * @param pageTitle 页面标题
 */
export async function trackPageView(pagePath: string, pageTitle?: string) {
  try {
    await Analytics.trackPageView(pagePath, pageTitle);
    console.log('[Analytics] 页面浏览事件已推送:', pagePath);
  } catch (error) {
    console.error('[Analytics] 推送页面浏览事件失败:', error);
  }
}

/**
 * 手动刷新事件队列
 * 确保所有待发送的事件立即发送
 */
export async function flushAnalytics() {
  try {
    await Analytics.flush();
    console.log('[Analytics] 事件队列已刷新');
  } catch (error) {
    console.error('[Analytics] 刷新事件队列失败:', error);
  }
}

/**
 * 获取分析客户端实例
 * 用于高级操作
 */
export function getAnalyticsInstance() {
  return Analytics.getInstance();
}

export default {
  initAnalytics,
  trackUserLogin,
  trackUserStatus,
  trackUserLogout,
  trackVideoSubmission,
  trackPageView,
  flushAnalytics,
  getAnalyticsInstance,
};
