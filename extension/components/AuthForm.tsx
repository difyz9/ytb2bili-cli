import React, { useState } from 'react';
import { authApi, type LoginInfo } from '../utils/api';

interface AuthFormProps {
  onAuthSuccess?: (loginInfo: LoginInfo) => void;
  onRefreshStatus?: () => void;
}

type AuthMode = 'login' | 'register';

export const AuthForm: React.FC<AuthFormProps> = ({ onAuthSuccess, onRefreshStatus }) => {
  const [mode, setMode] = useState<AuthMode>('login');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [name, setName] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  const isRegister = mode === 'register';

  const resetMessages = () => {
    setError(null);
    setSuccess(null);
  };

  const switchMode = (nextMode: AuthMode) => {
    setMode(nextMode);
    resetMessages();
  };

  const handleSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    resetMessages();
    setSubmitting(true);

    try {
      const loginInfo = isRegister
        ? await authApi.register(email.trim(), password, name.trim())
        : await authApi.login(email.trim(), password);

      setSuccess(isRegister ? '注册成功，已自动登录' : '登录成功');
      setPassword('');
      onRefreshStatus?.();
      onAuthSuccess?.(loginInfo);
    } catch (submitError) {
      setError(submitError instanceof Error ? submitError.message : '登录失败');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="rounded-lg border border-gray-200 bg-white p-4">
      <div className="mb-4 flex rounded-lg bg-gray-100 p-1">
        <button
          type="button"
          onClick={() => switchMode('login')}
          className={`flex-1 rounded-md px-3 py-2 text-sm font-medium transition-colors ${
            mode === 'login' ? 'bg-white text-blue-600 shadow-sm' : 'text-gray-600'
          }`}
        >
          登录
        </button>
        <button
          type="button"
          onClick={() => switchMode('register')}
          className={`flex-1 rounded-md px-3 py-2 text-sm font-medium transition-colors ${
            mode === 'register' ? 'bg-white text-blue-600 shadow-sm' : 'text-gray-600'
          }`}
        >
          注册
        </button>
      </div>

      <form className="space-y-3" onSubmit={handleSubmit}>
        {isRegister && (
          <div>
            <label className="mb-1 block text-xs font-medium text-gray-700">昵称</label>
            <input
              type="text"
              value={name}
              onChange={(event) => setName(event.target.value)}
              placeholder="选填，默认使用邮箱前缀"
              className="w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-2 focus:ring-blue-500/20"
            />
          </div>
        )}

        <div>
          <label className="mb-1 block text-xs font-medium text-gray-700">邮箱</label>
          <input
            type="email"
            value={email}
            onChange={(event) => setEmail(event.target.value)}
            placeholder="you@example.com"
            autoComplete="email"
            required
            className="w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-2 focus:ring-blue-500/20"
          />
        </div>

        <div>
          <label className="mb-1 block text-xs font-medium text-gray-700">密码</label>
          <input
            type="password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            placeholder={isRegister ? '至少 8 位，包含大小写字母和数字' : '输入账号密码'}
            autoComplete={isRegister ? 'new-password' : 'current-password'}
            required
            className="w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-2 focus:ring-blue-500/20"
          />
        </div>

        {error && <p className="text-xs text-red-600">{error}</p>}
        {success && <p className="text-xs text-green-600">{success}</p>}

        <button
          type="submit"
          disabled={submitting}
          className={`w-full rounded-md px-4 py-2 text-sm font-medium text-white transition-colors ${
            submitting ? 'cursor-not-allowed bg-gray-400' : 'bg-blue-600 hover:bg-blue-700'
          }`}
        >
          {submitting ? (isRegister ? '注册中...' : '登录中...') : (isRegister ? '注册并登录' : '登录')}
        </button>
      </form>

      <p className="mt-3 text-xs text-gray-500">
        {isRegister ? '注册成功后会自动登录，并可直接提交视频到当前配置的提交地址。' : '登录成功后，扩展会使用会员服务颁发的 JWT 调用视频提交接口。'}
      </p>
    </div>
  );
};

export default AuthForm;