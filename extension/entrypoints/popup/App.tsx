import React, { useEffect, useState } from 'react';
import { BACKEND_CONFIG, VIDEO_SUBMIT_PATH, normalizeBackendBaseUrl } from '../../utils/config';
import { getExtensionContextInvalidatedMessage, isExtensionContextInvalidated } from '../../utils/extension-context';
import type { LicenseType } from '../../types';

interface Settings {
  backendUrl: string;
}

type LocalStorageChange = {
  oldValue?: unknown;
  newValue?: unknown;
};

const browserApi: any = (globalThis as any).browser || (globalThis as any).chrome;

function validateBackendUrl(input: string): string {
  const trimmedInput = input.trim();
  if (!trimmedInput) {
    return '请输入提交服务地址';
  }

  let parsedUrl: URL;
  try {
    parsedUrl = new URL(trimmedInput);
  } catch {
    return '请输入完整的服务地址，包含 http:// 或 https://';
  }

  if (parsedUrl.protocol === 'https:') {
    return '';
  }

  if (parsedUrl.protocol === 'http:' && parsedUrl.hostname.toLowerCase() === 'localhost') {
    return '';
  }

  return '提交服务地址必须使用 HTTPS；本地联调仅支持 http://localhost';
}

function App() {
  const [settings, setSettings] = useState<Settings>({
    backendUrl: normalizeBackendBaseUrl(BACKEND_CONFIG.BASE_URL),
  });
  const [saveStatus, setSaveStatus] = useState<'idle' | 'saving' | 'saved'>('idle');
  const [backendUrlError, setBackendUrlError] = useState('');

  useEffect(() => {
    void loadSettings();

    const handleStorageChange = (
      changes: Record<string, LocalStorageChange>,
      areaName: string,
    ) => {
      if (areaName !== 'local') {
        return;
      }

      if (changes.backendUrl?.newValue) {
        const nextBackendUrl = normalizeBackendBaseUrl(String(changes.backendUrl.newValue));
        setSettings({
          backendUrl: nextBackendUrl,
        });
        setBackendUrlError(validateBackendUrl(nextBackendUrl));
      }
    };

    browserApi?.storage?.onChanged?.addListener?.(handleStorageChange);
    return () => {
      browserApi?.storage?.onChanged?.removeListener?.(handleStorageChange);
    };
  }, []);

  const loadSettings = async () => {
    try {
      const result = await browserApi.storage.local.get(['backendUrl']);
      const savedBackendUrl = result.backendUrl as string | undefined;
      const nextBackendUrl = normalizeBackendBaseUrl(savedBackendUrl || BACKEND_CONFIG.BASE_URL);

      setSettings({
        backendUrl: nextBackendUrl,
      });
      setBackendUrlError(validateBackendUrl(nextBackendUrl));
    } catch (error) {
      if (isExtensionContextInvalidated(error)) {
        setBackendUrlError(getExtensionContextInvalidatedMessage());
        return;
      }
      throw error;
    }
  };



  const handleSaveSettings = async () => {
    const normalizedBackendUrl = normalizeBackendBaseUrl(settings.backendUrl);
    const validationError = validateBackendUrl(normalizedBackendUrl);
    if (validationError) {
      setBackendUrlError(validationError);
      alert(validationError);
      return;
    }

    setSaveStatus('saving');

    try {
      await browserApi.storage.local.set({
        backendUrl: normalizedBackendUrl,
      });

      setSettings({ backendUrl: normalizedBackendUrl });
      setBackendUrlError('');
      setSaveStatus('saved');
      globalThis.setTimeout(() => setSaveStatus('idle'), 2000);
    } catch (error) {
      console.error('Failed to save settings:', error);
      setSaveStatus('idle');
      alert(isExtensionContextInvalidated(error) ? getExtensionContextInvalidatedMessage() : '保存设置失败，请重试');
    }
  };

  const handleResetBackendUrl = () => {
    const defaultBackendUrl = normalizeBackendBaseUrl(BACKEND_CONFIG.BASE_URL);
    setSettings({
      backendUrl: defaultBackendUrl,
    });
    setBackendUrlError(validateBackendUrl(defaultBackendUrl));
  };

  const normalizedBackendUrl = normalizeBackendBaseUrl(settings.backendUrl || BACKEND_CONFIG.BASE_URL);
  const backendUrlValidationError = validateBackendUrl(normalizedBackendUrl);
  const submitEndpointPreview = `${normalizedBackendUrl}${VIDEO_SUBMIT_PATH}`;

  return (
    <div style={{ width: '380px', minHeight: '520px' }} className="bg-[#f5f7fb]">
      <div className="overflow-hidden rounded-none bg-white shadow-sm">
        <div className="bg-gradient-to-r from-sky-600 via-cyan-600 to-teal-500 px-6 py-5 text-white">
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-2xl bg-white/15">
              <svg className="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.8} d="M7 4v16M17 4v16M3 8h18M3 12h18M3 16h10" />
              </svg>
            </div>
            <div>
              <h1 className="text-lg font-semibold">YTB2BILI Extension</h1>
              <p className="text-xs text-cyan-50/90">轻松提交视频到你的后端服务。</p>
            </div>
          </div>
        </div>

        <div className="space-y-5 p-5">
          <section className="rounded-2xl border border-slate-200 bg-white p-4">
            <div className="flex items-start justify-between gap-3">
              <div>
                <h2 className="text-sm font-semibold text-slate-900">后端设置</h2>
                <p className="mt-1 text-xs text-slate-500">用于视频提交的服务根地址。</p>
              </div>
            </div>

            <div className="mt-4 space-y-2">
              <label className="block text-xs font-medium text-slate-700">提交服务地址</label>
              <input
                type="text"
                value={settings.backendUrl}
                onChange={(event) => {
                  const nextValue = event.target.value;
                  setSettings({ backendUrl: nextValue });
                  setBackendUrlError(validateBackendUrl(normalizeBackendBaseUrl(nextValue)));
                }}
                placeholder="https://example.com/ytb2bili"
                className={`w-full rounded-xl px-3 py-2 text-sm text-slate-900 outline-none transition focus:ring-2 ${backendUrlError ? 'border border-rose-400 focus:border-rose-500 focus:ring-rose-500/15' : 'border border-slate-300 focus:border-sky-500 focus:ring-sky-500/15'}`}
              />
              <p className="text-[11px] leading-5 text-amber-700">
                默认仅支持 https:// 服务地址；本地联调可使用 http://localhost，扩展会自动拼接提交接口路径。
              </p>
              {backendUrlError ? (
                <p className="text-[11px] leading-5 text-rose-600">{backendUrlError}</p>
              ) : null}
              <div className="flex items-center justify-between gap-2 text-[11px] text-slate-500">
                <span className="truncate">提交接口：{submitEndpointPreview}</span>
                <button
                  type="button"
                  onClick={handleResetBackendUrl}
                  className="shrink-0 rounded-md border border-slate-300 px-2 py-1 font-medium text-slate-600 transition hover:bg-slate-50"
                >
                  恢复默认
                </button>
              </div>
            </div>



            <button
              type="button"
              onClick={handleSaveSettings}
              disabled={saveStatus === 'saving' || Boolean(backendUrlValidationError)}
              className={`mt-4 w-full rounded-xl px-4 py-2.5 text-sm font-medium text-white transition ${
                saveStatus === 'saved'
                  ? 'bg-emerald-500'
                  : saveStatus === 'saving'
                    ? 'cursor-not-allowed bg-slate-400'
                    : backendUrlValidationError
                      ? 'cursor-not-allowed bg-slate-300 text-slate-500'
                      : 'bg-sky-600 hover:bg-sky-700'
              }`}
            >
              {saveStatus === 'saving' ? '保存中...' : saveStatus === 'saved' ? '已保存' : '保存设置'}
            </button>
          </section>

          <section className="rounded-2xl border border-sky-100 bg-sky-50 p-4 text-xs text-sky-900">
            <h2 className="text-sm font-semibold">使用说明</h2>
            <ul className="mt-2 space-y-1.5 text-sky-900/90">
              <li>1. 配置视频提交后端地址（可选，默认已配置）。</li>
              <li>2. 打开 YouTube 视频页，点击播放器上的扩展入口。</li>
              <li>3. 选择提交操作，扩展会提取视频信息和字幕。</li>
              <li>4. 提交成功后，可在你的后端服务中查看处理结果。</li>
            </ul>
          </section>
        </div>
      </div>
    </div>
  );
}

export default App;
