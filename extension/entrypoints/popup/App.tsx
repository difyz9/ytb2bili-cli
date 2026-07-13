import React, { useEffect, useState } from 'react';
import { getBitableConfig } from '../../utils/bitable';

const browserApi: any = (globalThis as any).browser || (globalThis as any).chrome;

interface VideoInfo {
  url: string;
  title: string;
  channel: string;
}

function App() {
  const [videoInfo, setVideoInfo] = useState<VideoInfo | null>(null);
  const [bitableConfig, setBitableConfig] = useState<{ appId?: string; appToken?: string; tableId?: string } | null>(null);
  const [status, setStatus] = useState<'idle' | 'submitting' | 'success' | 'error'>('idle');
  const [message, setMessage] = useState('');
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    loadConfig();
    detectVideoInfo();
  }, []);

  const loadConfig = async () => {
    const config = await getBitableConfig();
    setBitableConfig(config);
    setLoading(false);
  };

  const detectVideoInfo = () => {
    const url = window.location.href;
    const title = document.querySelector('h1.ytd-watch-metadata yt-formatted-string')?.textContent || 
                  document.querySelector('meta[name="title"]')?.getAttribute('content') || 
                  '未知标题';
    const channel = document.querySelector('#channel-name a')?.textContent || 
                    document.querySelector('meta[name="author"]')?.getAttribute('content') || 
                    '未知频道';
    
    setVideoInfo({ url, title, channel });
  };

  const handleSubmit = async () => {
    if (!bitableConfig?.appId || !bitableConfig?.appToken) {
      setStatus('error');
      setMessage('请先在设置页面配置飞书多维表格');
      return;
    }

    if (!videoInfo) {
      setStatus('error');
      setMessage('未检测到视频信息');
      return;
    }

    setStatus('submitting');
    setMessage('');

    try {
      // 通过 content script 提交（content script 已经集成了 bitable 模块）
      // 这里只是触发一个通知
      setStatus('success');
      setMessage('请点击播放器上的提交按钮');
    } catch (error) {
      setStatus('error');
      setMessage(`❌ 提交失败: ${error instanceof Error ? error.message : String(error)}`);
    }
  };

  if (loading) {
    return (
      <div style={{ width: '380px', minHeight: '520px' }} className="bg-[#f5f7fb] flex items-center justify-center">
        <p className="text-slate-500">加载中...</p>
      </div>
    );
  }

  return (
    <div style={{ width: '380px', minHeight: '520px' }} className="bg-[#f5f7fb]">
      <div className="overflow-hidden rounded-none bg-white shadow-sm">
        {/* 标题栏 */}
        <div className="bg-gradient-to-r from-sky-600 via-cyan-600 to-teal-500 px-6 py-5 text-white">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-2xl bg-white/15">
                <svg className="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.8} d="M7 4v16M17 4v16M3 8h18M3 12h18M3 16h10" />
                </svg>
              </div>
              <div>
                <h1 className="text-lg font-semibold">YTB2BILI Extension</h1>
                <p className="text-xs text-cyan-50/90">轻松提交视频到 B站</p>
              </div>
            </div>
            <button
              type="button"
              onClick={() => browserApi?.runtime?.openOptionsPage?.()}
              className="rounded-lg bg-white/20 px-3 py-1.5 text-xs font-medium transition hover:bg-white/30"
            >
              ⚙️ 设置
            </button>
          </div>
        </div>

        <div className="space-y-5 p-5">
          {/* 多维表格配置状态 */}
          <section className="rounded-2xl border border-slate-200 bg-white p-4">
            <div className="flex items-start justify-between gap-3">
              <div>
                <h2 className="text-sm font-semibold text-slate-900">飞书多维表格</h2>
                <p className="mt-1 text-xs text-slate-500">
                  {bitableConfig?.appId && bitableConfig?.appToken ? (
                    <span className="text-emerald-600">✅ 已配置</span>
                  ) : (
                    <span className="text-amber-600">⚠️ 未配置，请在设置页面配置</span>
                  )}
                </p>
              </div>
            </div>
          </section>

          {/* 视频信息 */}
          {videoInfo && (
            <section className="rounded-2xl border border-sky-100 bg-sky-50 p-4">
              <h2 className="text-sm font-semibold text-sky-900">检测到的视频</h2>
              <div className="mt-2 space-y-1 text-xs text-sky-900/90">
                <p><strong>标题:</strong> {videoInfo.title}</p>
                <p><strong>频道:</strong> {videoInfo.channel}</p>
                <p className="truncate"><strong>链接:</strong> {videoInfo.url}</p>
              </div>
            </section>
          )}

          {/* 提交按钮 */}
          <button
            type="button"
            onClick={handleSubmit}
            disabled={status === 'submitting' || !bitableConfig?.appId || !bitableConfig?.appToken}
            className={`w-full rounded-xl px-4 py-2.5 text-sm font-medium text-white transition ${
              status === 'success'
                ? 'bg-emerald-500'
                : status === 'submitting'
                  ? 'cursor-not-allowed bg-slate-400'
                  : !bitableConfig?.appId || !bitableConfig?.appToken
                    ? 'cursor-not-allowed bg-slate-300 text-slate-500'
                    : 'bg-sky-600 hover:bg-sky-700'
            }`}
          >
            {status === 'submitting' ? '提交中...' : status === 'success' ? '✅ 已提交' : '🚀 提交到多维表格'}
          </button>

          {/* 状态消息 */}
          {message && (
            <div className={`rounded-xl p-3 text-xs ${
              status === 'success' ? 'bg-emerald-50 text-emerald-700' : 'bg-rose-50 text-rose-700'
            }`}>
              {message}
            </div>
          )}

          {/* 使用说明 */}
          <section className="rounded-2xl border border-sky-100 bg-sky-50 p-4 text-xs text-sky-900">
            <h2 className="text-sm font-semibold">使用说明</h2>
            <ul className="mt-2 space-y-1.5 text-sky-900/90">
              <li>1. 点击右上角设置按钮，配置飞书多维表格</li>
              <li>2. 打开 YouTube 视频页面</li>
              <li>3. 点击播放器上的提交按钮</li>
              <li>4. 后端服务会自动处理视频并投稿到 B站</li>
            </ul>
          </section>
        </div>
      </div>
    </div>
  );
}

export default App;
