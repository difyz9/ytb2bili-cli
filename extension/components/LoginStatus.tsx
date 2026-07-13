import React, { useState, useEffect } from 'react';
import { checkLoginStatus } from '../utils/api';
import { getExtensionContextInvalidatedMessage, isExtensionContextInvalidated } from '../utils/extension-context';
import type { User } from '../types';

interface LoginStatusProps {
  onStatusChange?: (isLoggedIn: boolean, user?: User) => void;
  showDetails?: boolean;
  className?: string;
}

export const LoginStatus: React.FC<LoginStatusProps> = ({ 
  onStatusChange, 
  showDetails = true,
  className = '' 
}) => {
  const [isLoggedIn, setIsLoggedIn] = useState(false);
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const refreshStatus = async () => {
    setLoading(true);
    setError(null);
    
    try {
      const status = await checkLoginStatus();
      const shouldShowError = !status.isLoggedIn && status.error === '未登录，请先在插件中完成登录'
        ? null
        : status.error || null;
      
      setIsLoggedIn(status.isLoggedIn);
      setUser(status.user || null);
      setError(shouldShowError);
      
      // 通知父组件状态变化
      onStatusChange?.(status.isLoggedIn, status.user);
      
    } catch (err) {
      console.error('检查登录状态失败:', err);
      setError(isExtensionContextInvalidated(err) ? getExtensionContextInvalidatedMessage() : '检查登录状态失败');
      setIsLoggedIn(false);
      setUser(null);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    refreshStatus();
    
    // 每30秒自动刷新一次状态
    const interval = setInterval(refreshStatus, 30000);
    
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className={`flex items-center space-x-2 ${className}`}>
        <div className="w-4 h-4 border-2 border-blue-500 border-t-transparent rounded-full animate-spin"></div>
        <span className="text-sm text-gray-600">检查登录状态...</span>
      </div>
    );
  }

  if (error) {
    return (
      <div className={`flex items-center space-x-2 ${className}`}>
        <div className="w-4 h-4 bg-red-500 rounded-full flex items-center justify-center">
          <span className="text-white text-xs">!</span>
        </div>
        <span className="text-sm text-red-600">{error}</span>
        <button
          onClick={refreshStatus}
          className="text-xs text-blue-500 hover:text-blue-700 underline"
        >
          重试
        </button>
      </div>
    );
  }

  if (!isLoggedIn) {
    return (
      <div className={`flex items-center space-x-2 ${className}`}>
        <div className="w-4 h-4 bg-gray-400 rounded-full"></div>
        <span className="text-sm text-gray-600">未登录</span>
        {showDetails && (
          <span className="text-xs text-gray-500">
            可直接在插件内登录，或改用网页登录授权
          </span>
        )}
      </div>
    );
  }

  return (
    <div className={`flex items-center space-x-2 ${className}`}>
      <div className="w-4 h-4 bg-green-500 rounded-full"></div>
      <div className="flex items-center space-x-2">
        {user?.avatar && (
          <img 
            src={user.avatar} 
            alt={user.name}
            className="w-6 h-6 rounded-full"
          />
        )}
        <div>
          <span className="text-sm font-medium text-gray-900">
            {user?.name || '已登录'}
          </span>
          {showDetails && user?.mid && (
            <span className="text-xs text-gray-500 block">
              MID: {user.mid}
            </span>
          )}
        </div>
      </div>
      <button
        onClick={refreshStatus}
        className="text-xs text-blue-500 hover:text-blue-700"
        title="刷新状态"
      >
        ↻
      </button>
    </div>
  );
};

export default LoginStatus;