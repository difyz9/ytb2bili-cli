import React from 'react';
import ReactDOM from 'react-dom/client';
import toast, { Toaster } from 'react-hot-toast';

import { VideoDataExtractor } from '../utils/video-data';
import { submitToBitable } from '../utils/bitable';
import './content-styles.css';

const browser: any = (globalThis as any).browser || (globalThis as any).chrome;
const LOCAL_BACKEND_URL = 'http://localhost:8096/api/v1/submit';

export default defineContentScript({
  matches: ['*://*.youtube.com/*'],
  main() {
    
    // 创建通知容器并渲染 Toaster
    const notificationContainer = document.createElement('div');
    notificationContainer.id = 'ytb2bili-extension-notifications';
    document.body.appendChild(notificationContainer);

    // 使用 react-hot-toast 的 Toaster 组件
    const toasterRoot = ReactDOM.createRoot(notificationContainer);
    toasterRoot.render(React.createElement(Toaster, {
      position: 'top-right',
      toastOptions: {
        duration: 4000,
        style: {
          background: '#363636',
          color: '#fff',
        },
        success: {
          duration: 3000,
          iconTheme: {
            primary: '#4ade80',
            secondary: '#fff',
          },
        },
        error: {
          duration: 5000,
          iconTheme: {
            primary: '#ef4444',
            secondary: '#fff',
          },
        },
      },
    }));

    // 只支持 YouTube，注入播放器按钮
    if (window.location.hostname.includes('youtube.com')) {
      injectYouTubePlayerButton();
    }
  },
});


/**
 * 显示通知 - 使用 react-hot-toast
 */
function showNotification({ message, type }: { message: string; type: 'success' | 'error' | 'loading' }): void {
  switch (type) {
    case 'success':
      toast.success(message, { duration: 4000 });
      break;
    case 'error':
      toast.error(message, { duration: 5000 });
      break;
    case 'loading':
      toast.loading(message);
      break;
  }
}

/**
 * 直接提交到本地服务（不经过 background worker，避免 Extension context invalidated）
 */
async function submitToLocal(data: {
  url: string;
  title: string;
  channel: string;
  videoId: string;
}): Promise<{ success: boolean; message: string }> {
  try {
    const response = await fetch(LOCAL_BACKEND_URL, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        url: data.url,
        title: data.title,
        description: `[${data.channel}] ${data.title}`,
        operationType: 'manual',
        subtitles: [],
        playlistId: '',
        timestamp: new Date().toISOString(),
      }),
    });
    if (!response.ok) {
      const text = await response.text();
      return { success: false, message: `本地服务返回 ${response.status}` };
    }
    return { success: true, message: '提交成功' };
  } catch {
    return { success: false, message: '本地服务未运行' };
  }
}

/**
 * 获取 YouTube cookies
 */
async function getYoutubeCookies(): Promise<string> {
  return new Promise((resolve) => {
    browser.runtime.sendMessage(
      { action: 'getCookies', url: 'https://www.youtube.com' },
      (response: any) => {
        if (response?.success && response?.cookies) {
          resolve(response.cookies);
        } else {
          resolve('');
        }
      }
    );
  });
}

/**
 * 注入按钮到 YouTube 播放器控制栏
 */
function injectYouTubePlayerButton() {
  const checkAndInject = () => {
    // YouTube 播放器右侧控制按钮容器的选择器
    const rightControls = document.querySelector('.ytp-right-controls');
    
    if (rightControls && !document.getElementById('ytb2bili-extension-button')) {
      // 创建按钮容器
      const buttonContainer = document.createElement('div');
      buttonContainer.id = 'ytb2bili-extension-button';
      buttonContainer.className = 'ytp-button';
      buttonContainer.style.cssText = 'position: relative; display: inline-block;';
      
      // 创建主按钮
      const mainButton = document.createElement('button');
      mainButton.className = 'ytp-button';
      mainButton.setAttribute('aria-label', 'YTB2BILI Extension');
      mainButton.setAttribute('title', '提交视频到 ytb2bili');
      mainButton.style.cssText = `
        width: 48px; 
        height: 100%; 
        padding: 0;
        opacity: 0.9;
        transition: opacity 0.2s;
      `;
      mainButton.innerHTML = `
        <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="pointer-events: none; margin: auto; display: block;">
          <path d="M7 17L17 7M7 7h10v10" stroke-linecap="round" stroke-linejoin="round"/>
          <circle cx="7" cy="17" r="1.5" fill="currentColor"/>
        </svg>
      `;
      mainButton.onmouseenter = () => mainButton.style.opacity = '1';
      mainButton.onmouseleave = () => mainButton.style.opacity = '0.9';
      
      // 主按钮点击事件 - 优先提交到本地服务，降级到飞书多维表格
      mainButton.addEventListener('click', async (e) => {
        e.stopPropagation();
        
        try {
          showNotification({
            message: '正在获取视频信息...',
            type: 'loading'
          });

          const { videoData, subtitles } = await VideoDataExtractor.extractFromCurrentPage();
          
          // 验证必需的视频信息
          if (!videoData.url || !videoData.videoId || !videoData.title) {
            throw new Error('无法获取视频基本信息（URL、ID或标题缺失）');
          }
          
          showNotification({
            message: '正在提交到 ytb2bili...',
            type: 'loading'
          });

          // 优先直接提交到本地服务（不经过 background worker，避免 Extension context invalidated）
          const localResult = await submitToLocal({
            url: videoData.url,
            title: videoData.title,
            channel: videoData.uploader?.name || '未知',
            videoId: videoData.videoId,
          });

          if (localResult.success) {
            showNotification({
              message: '✅ 已提交到 ytb2bili 本地服务！',
              type: 'success'
            });
          } else {
            // 本地服务不可用，降级到 background script（飞书多维表格）
            console.warn('本地服务不可用，尝试通过 background 提交:', localResult.message);
            const cookies = await getYoutubeCookies();
            const result = await submitToBitable({
              url: videoData.url,
              title: videoData.title,
              channel: videoData.uploader?.name || '未知',
              videoId: videoData.videoId,
              cookies: cookies,
            });
            
            if (result.success) {
              showNotification({
                message: `✅ 提交成功！Record ID: ${result.recordId}`,
                type: 'success'
              });
            } else {
              showNotification({
                message: `❌ 提交失败: ${result.message}`,
                type: 'error'
              });
            }
          }
          
        } catch (error) {
          // 检测扩展上下文失效，提示刷新页面
          const errMsg = error instanceof Error ? error.message : String(error);
          if (/Extension context invalidated|context invalidated/i.test(errMsg)) {
            showNotification({
              message: '⚠️ 扩展已重载，请刷新页面后重试',
              type: 'error'
            });
          } else {
            showNotification({
              message: `❌ ${errMsg}`,
              type: 'error'
            });
          }
        }
      });
      
      buttonContainer.appendChild(mainButton);
      
      // 插入到播放器控制栏的最左边（在设置按钮之前）
      rightControls.insertBefore(buttonContainer, rightControls.firstChild);
    }
  };
  
  // 初始检查
  checkAndInject();
  
  // 使用 MutationObserver 监听 DOM 变化，以处理页面动态加载
  const observer = new MutationObserver(() => {
    checkAndInject();
  });
  
  observer.observe(document.body, {
    childList: true,
    subtree: true,
  });
  
  // 监听 YouTube 的页面导航（单页应用）
  let lastUrl = location.href;
  new MutationObserver(() => {
    const currentUrl = location.href;
    if (currentUrl !== lastUrl) {
      lastUrl = currentUrl;
      // URL 变化时重新注入
      setTimeout(checkAndInject, 500);
    }
  }).observe(document.querySelector('title')!, {
    childList: true,
  });
}
