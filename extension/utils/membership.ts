/**
 * 会员功能限制工具模块
 * 用于检查和限制功能访问权限
 */

import { licenseApi } from './api';
import type { LicenseType } from '../types';

// 功能权限映射
const FEATURE_PERMISSIONS: Record<string, LicenseType[]> = {
  // 基础功能 - 所有用户
  'video_submit': ['free', 'basic', 'standard', 'pro', 'enterprise'],
  'subtitle_download': ['free', 'basic', 'standard', 'pro', 'enterprise'],
  'custom_backend_url': ['basic', 'standard', 'pro', 'enterprise'],
  
  // 高级功能 - 付费用户
  'batch_submit': ['basic', 'standard', 'pro', 'enterprise'],
  'ai_translation': ['basic', 'standard', 'pro', 'enterprise'],
  'auto_upload': ['pro', 'enterprise'],
  'priority_queue': ['pro', 'enterprise'],
  
  // 企业功能
  'api_access': ['enterprise'],
  'team_collaboration': ['enterprise'],
};

// 功能限制配置
const FEATURE_LIMITS: Record<LicenseType, {
  maxVideosPerDay?: number;
  maxBatchSize?: number;
  priorityProcessing?: boolean;
  aiTranslation?: boolean;
  autoUpload?: boolean;
}> = {
  free: {
    maxVideosPerDay: 5,
    maxBatchSize: 1,
    priorityProcessing: false,
    aiTranslation: false,
    autoUpload: false,
  },
  basic: {
    maxVideosPerDay: 20,
    maxBatchSize: 5,
    priorityProcessing: false,
    aiTranslation: true,
    autoUpload: false,
  },
  standard: {
    maxVideosPerDay: 50,
    maxBatchSize: 10,
    priorityProcessing: true,
    aiTranslation: true,
    autoUpload: false,
  },
  pro: {
    maxVideosPerDay: 100,
    maxBatchSize: 20,
    priorityProcessing: true,
    aiTranslation: true,
    autoUpload: true,
  },
  enterprise: {
    maxVideosPerDay: -1, // 无限制
    maxBatchSize: 100,
    priorityProcessing: true,
    aiTranslation: true,
    autoUpload: true,
  },
};

/**
 * 获取当前 License 状态
 */
export async function getCurrentLicenseStatus() {
  try {
    const status = await licenseApi.verifyLicense();
    return status;
  } catch (error) {
    console.error('Failed to get license status:', error);
    return null;
  }
}

/**
 * 检查是否有权限访问某个功能
 * @param featureName 功能名称
 * @returns Promise<{ allowed: boolean; reason?: string; licenseType?: string }>
 */
export async function checkFeatureAccess(featureName: string): Promise<{
  allowed: boolean;
  reason?: string;
  licenseType?: string;
  upgradeRequired?: LicenseType;
}> {
  try {
    const status = await getCurrentLicenseStatus();
    
    if (!status || !status.is_valid) {
      return {
        allowed: false,
        reason: '请先购买或激活会员',
        licenseType: 'free',
        upgradeRequired: 'basic',
      };
    }

    const licenseType = status.license_type as LicenseType;
    const allowedTypes = FEATURE_PERMISSIONS[featureName];

    if (!allowedTypes) {
      // 功能不在权限映射中，默认允许
      return { allowed: true, licenseType };
    }

    if (allowedTypes.includes(licenseType)) {
      return { allowed: true, licenseType };
    }

    // 找出所需的最低会员等级
    const typeOrder: LicenseType[] = ['free', 'basic', 'standard', 'pro', 'enterprise'];
    const requiredType = allowedTypes
      .map(type => ({ type, index: typeOrder.indexOf(type) }))
      .filter(item => item.index !== -1)
      .sort((a, b) => a.index - b.index)[0]?.type;

    return {
      allowed: false,
      reason: `此功能需要 ${getTypeName(requiredType)} 或更高会员`,
      licenseType,
      upgradeRequired: requiredType,
    };
  } catch (error) {
    console.error('Failed to check feature access:', error);
    return {
      allowed: false,
      reason: '检查权限失败',
    };
  }
}

/**
 * 获取功能限制配置
 * @param licenseType 会员类型，不传则自动获取
 */
export async function getFeatureLimits(licenseType?: LicenseType) {
  if (!licenseType) {
    const status = await getCurrentLicenseStatus();
    licenseType = (status?.license_type as LicenseType) || 'free';
  }
  
  return FEATURE_LIMITS[licenseType] || FEATURE_LIMITS.free;
}

/**
 * 检查是否达到使用限制
 * @param limitType 限制类型 (如: 'maxVideosPerDay')
 * @param currentUsage 当前使用量
 */
export async function checkUsageLimit(
  limitType: keyof typeof FEATURE_LIMITS.free,
  currentUsage: number
): Promise<{
  allowed: boolean;
  limit: number;
  remaining: number;
  reason?: string;
}> {
  const limits = await getFeatureLimits();
  const limit = limits[limitType];

  if (typeof limit !== 'number') {
    return { allowed: true, limit: -1, remaining: -1 };
  }

  if (limit === -1) {
    // 无限制
    return { allowed: true, limit: -1, remaining: -1 };
  }

  const remaining = limit - currentUsage;
  const allowed = remaining > 0;

  return {
    allowed,
    limit,
    remaining: Math.max(0, remaining),
    reason: allowed ? undefined : `已达到每日限制 (${limit})`,
  };
}

/**
 * 功能受限提示
 * 返回可用于UI显示的提示信息
 */
export function getFeatureRestrictedMessage(featureName: string, upgradeRequired?: LicenseType): string {
  const messages: Record<string, string> = {
    custom_backend_url: '自定义提交地址需要会员权限',
    batch_submit: '批量提交功能需要付费会员',
    ai_translation: 'AI 字幕翻译功能需要付费会员',
    auto_upload: '自动上传功能需要专业版或企业版',
    priority_queue: '优先处理队列需要专业版或企业版',
    api_access: 'API 访问需要企业版',
    team_collaboration: '团队协作功能需要企业版',
  };

  let message = messages[featureName] || '此功能需要更高级别的会员';
  
  if (upgradeRequired) {
    message += `，请升级到 ${getTypeName(upgradeRequired)}`;
  }

  return message;
}

/**
 * 获取会员类型的显示名称
 */
export function getTypeName(type?: LicenseType): string {
  const names: Record<LicenseType, string> = {
    free: '免费版',
    basic: '基础版',
    standard: '标准版',
    pro: '专业版',
    enterprise: '企业版',
  };
  return names[type || 'free'];
}

/**
 * 功能使用统计（本地缓存）
 */
class UsageTracker {
  private storageKey = 'feature_usage_stats';

  async getUsage(featureName: string, date?: string): Promise<number> {
    const today = date || new Date().toISOString().split('T')[0];
    const browserApi = (globalThis as any).browser || (globalThis as any).chrome;
    const result = await browserApi.storage.local.get(this.storageKey);
    const stats = result[this.storageKey] || {};
    return stats[`${featureName}_${today}`] || 0;
  }

  async incrementUsage(featureName: string, date?: string): Promise<number> {
    const today = date || new Date().toISOString().split('T')[0];
    const browserApi = (globalThis as any).browser || (globalThis as any).chrome;
    const result = await browserApi.storage.local.get(this.storageKey);
    const stats = result[this.storageKey] || {};
    const key = `${featureName}_${today}`;
    const newCount = (stats[key] || 0) + 1;
    stats[key] = newCount;
    await browserApi.storage.local.set({ [this.storageKey]: stats });
    return newCount;
  }

  async resetUsage(featureName?: string): Promise<void> {
    const browserApi = (globalThis as any).browser || (globalThis as any).chrome;
    if (featureName) {
      const result = await browserApi.storage.local.get(this.storageKey);
      const stats = result[this.storageKey] || {};
      const today = new Date().toISOString().split('T')[0];
      delete stats[`${featureName}_${today}`];
      await browserApi.storage.local.set({ [this.storageKey]: stats });
    } else {
      await browserApi.storage.local.remove(this.storageKey);
    }
  }
}

export const usageTracker = new UsageTracker();

/**
 * 在执行功能前检查权限和使用限制
 * 示例用法：
 * 
 * const result = await checkBeforeFeatureUse('video_submit');
 * if (!result.allowed) {
 *   alert(result.reason);
 *   if (result.shouldUpgrade) {
 *     licenseApi.openMembershipPage();
 *   }
 *   return;
 * }
 */
export async function checkBeforeFeatureUse(featureName: string): Promise<{
  allowed: boolean;
  reason?: string;
  shouldUpgrade?: boolean;
  upgradeRequired?: LicenseType;
}> {
  // 1. 检查功能权限
  const accessCheck = await checkFeatureAccess(featureName);
  if (!accessCheck.allowed) {
    return {
      allowed: false,
      reason: accessCheck.reason,
      shouldUpgrade: true,
      upgradeRequired: accessCheck.upgradeRequired,
    };
  }

  // 2. 检查使用限制（如果有）
  if (featureName === 'video_submit') {
    const currentUsage = await usageTracker.getUsage('video_submit');
    const limitCheck = await checkUsageLimit('maxVideosPerDay', currentUsage);
    
    if (!limitCheck.allowed) {
      return {
        allowed: false,
        reason: `${limitCheck.reason}，升级会员可提高限额`,
        shouldUpgrade: true,
        upgradeRequired: 'basic',
      };
    }
  }

  return { allowed: true };
}

export default {
  getCurrentLicenseStatus,
  checkFeatureAccess,
  getFeatureLimits,
  checkUsageLimit,
  getFeatureRestrictedMessage,
  getTypeName,
  usageTracker,
  checkBeforeFeatureUse,
};
