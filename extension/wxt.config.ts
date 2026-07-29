import { defineConfig } from 'wxt';

// See https://wxt.dev/api/config.html
export default defineConfig({
  outDir: 'dist',
  modules: ['@wxt-dev/module-react'],
  manifest: {
    name: 'Ytb2Bili Extension',
    description: '保存 YouTube/Bilibili 视频 URL 和字幕的浏览器扩展',
    version: process.env.VERSION || '1.2.0',
    permissions: [
      'storage',
      'activeTab',
      'webRequest',
      'cookies',
      'alarms',
    ],
    host_permissions: [
    "https://*/",
    ],
    web_accessible_resources: [
      {
        resources: ['page-interceptor.js'],
        matches: ['*://*.youtube.com/*'],
      },
    ],
    icons: {
      16: '/icon/icon16.png',
      32: '/icon/icon32.png',
      48: '/icon/icon48.png',
      128: '/icon/icon128.png',
    },
  },
});
