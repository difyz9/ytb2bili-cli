import React, { useState, useEffect } from 'react';
import { licenseApi } from '../utils/api';

interface MembershipButtonProps {
  source?: string;
  className?: string;
  fullWidth?: boolean;
}

/**
 * 会员购买按钮组件
 * 点击后携带设备ID跳转到会员购买页面
 */
export function MembershipButton({ 
  source = 'popup', 
  className = '',
  fullWidth = false 
}: MembershipButtonProps) {
  const [deviceId, setDeviceId] = useState<string>('');
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    // 获取设备ID
    try {
      const id = licenseApi.getDeviceId();
      setDeviceId(id);
    } catch (error) {
      console.error('Failed to get device ID:', error);
    }
  }, []);

  const handleClick = async () => {
    setLoading(true);
    try {
      await licenseApi.openMembershipPage(source);
    } catch (error) {
      console.error('Failed to open membership page:', error);
    } finally {
      setLoading(false);
    }
  };

  return (
    <button
      onClick={handleClick}
      disabled={loading}
      className={`
        inline-flex items-center justify-center
        px-4 py-2 rounded-lg
        bg-gradient-to-r from-purple-600 to-pink-600 
        hover:from-purple-700 hover:to-pink-700
        text-white font-medium
        transition-all duration-200
        disabled:opacity-50 disabled:cursor-not-allowed
        ${fullWidth ? 'w-full' : ''}
        ${className}
      `}
    >
      <svg 
        className="w-5 h-5 mr-2" 
        fill="none" 
        stroke="currentColor" 
        viewBox="0 0 24 24"
      >
        <path 
          strokeLinecap="round" 
          strokeLinejoin="round" 
          strokeWidth={2} 
          d="M5 3v4M3 5h4M6 17v4m-2-2h4m5-16l2.286 6.857L21 12l-5.714 2.143L13 21l-2.286-6.857L5 12l5.714-2.143L13 3z" 
        />
      </svg>
      {loading ? '打开中...' : '升级会员'}
    </button>
  );
}

interface LicenseStatusBadgeProps {
  licenseType?: string;
  isValid?: boolean;
  className?: string;
}

/**
 * 会员状态徽章组件
 */
export function LicenseStatusBadge({ 
  licenseType = 'free', 
  isValid = false,
  className = '' 
}: LicenseStatusBadgeProps) {
  const badges = {
    free: { text: '免费版', color: 'bg-gray-100 text-gray-800' },
    basic: { text: '基础版', color: 'bg-blue-100 text-blue-800' },
    standard: { text: '标准版', color: 'bg-indigo-100 text-indigo-800' },
    pro: { text: '专业版', color: 'bg-purple-100 text-purple-800' },
    enterprise: { text: '企业版', color: 'bg-orange-100 text-orange-800' },
  };

  const badge = badges[licenseType as keyof typeof badges] || badges.free;

  return (
    <span 
      className={`
        inline-flex items-center px-2.5 py-0.5 rounded-full 
        text-xs font-medium
        ${badge.color}
        ${!isValid ? 'opacity-50' : ''}
        ${className}
      `}
    >
      {badge.text}
      {!isValid && ' (已过期)'}
    </span>
  );
}

interface MembershipCardProps {
  licenseStatus?: any;
  onUpgrade?: () => void;
  className?: string;
}

/**
 * 会员卡片组件
 * 显示会员状态和升级按钮
 */
export function MembershipCard({ 
  licenseStatus, 
  onUpgrade,
  className = '' 
}: MembershipCardProps) {
  const [status, setStatus] = useState<any>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (licenseStatus) {
      setStatus(licenseStatus);
      setLoading(false);
    } else {
      loadLicenseStatus();
    }
  }, [licenseStatus]);

  const loadLicenseStatus = async () => {
    setLoading(true);
    try {
      const result = await licenseApi.verifyLicense();
      setStatus(result);
    } catch (error) {
      console.error('Failed to load license status:', error);
    } finally {
      setLoading(false);
    }
  };

  if (loading) {
    return (
      <div className={`bg-white rounded-lg shadow p-4 ${className}`}>
        <div className="animate-pulse">
          <div className="h-4 bg-gray-200 rounded w-1/3 mb-2"></div>
          <div className="h-3 bg-gray-200 rounded w-1/2"></div>
        </div>
      </div>
    );
  }

  const isFree = !status || status.license_type === 'free';
  const isExpired = status && !status.is_valid;

  return (
    <div className={`bg-gradient-to-br from-purple-50 to-pink-50 rounded-lg shadow-sm p-4 ${className}`}>
      <div className="flex items-center justify-between mb-3">
        <div>
          <h3 className="text-sm font-medium text-gray-900 mb-1">会员状态</h3>
          <LicenseStatusBadge 
            licenseType={status?.license_type || 'free'} 
            isValid={status?.is_valid || false}
          />
        </div>
        <svg 
          className="w-8 h-8 text-purple-600 opacity-50" 
          fill="currentColor" 
          viewBox="0 0 20 20"
        >
          <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
        </svg>
      </div>

      {status?.features && status.features.length > 0 && (
        <div className="mb-3">
          <p className="text-xs text-gray-600 mb-1">当前权益：</p>
          <ul className="text-xs text-gray-700 space-y-1">
            {status.features.slice(0, 3).map((feature: string, index: number) => (
              <li key={index} className="flex items-center">
                <svg className="w-3 h-3 mr-1 text-green-500" fill="currentColor" viewBox="0 0 20 20">
                  <path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" />
                </svg>
                {feature}
              </li>
            ))}
          </ul>
        </div>
      )}

      {(isFree || isExpired) && (
        <div className="mt-3 pt-3 border-t border-purple-200">
          <p className="text-xs text-gray-600 mb-2">
            {isExpired ? '会员已过期，续费解锁更多功能' : '升级会员解锁更多高级功能'}
          </p>
          <MembershipButton 
            source="membership_card" 
            fullWidth 
            className="text-sm"
          />
        </div>
      )}

      {status?.expired_at && status.is_valid && (
        <p className="mt-3 text-xs text-gray-500 text-center">
          到期时间：{new Date(status.expired_at).toLocaleDateString()}
        </p>
      )}
    </div>
  );
}

export default {
  MembershipButton,
  LicenseStatusBadge,
  MembershipCard,
};
