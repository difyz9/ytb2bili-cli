import React from 'react';
import ReactDOM from 'react-dom/client';
import toast, { Toaster } from 'react-hot-toast';

import { VideoDataExtractor } from '../utils/video-data';
import { checkLoginStatus, videoApi } from '../utils/api';
import type { VideoSubmissionData } from '../utils/api';
import './content-styles.css';

const browser: any = (globalThis as any).browser || (globalThis as any).chrome;

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
      mainButton.setAttribute('title', '提交视频到后端');
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
      
      // 主按钮点击事件 - 获取数据并提交到后端
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
          
          // 准备提交数据（字幕数据可以为空）
          const submissionData: VideoSubmissionData = {
            platform: videoData.platform,
            video_id: videoData.videoId,
            title: videoData.title,
            description: videoData.description || '',
            duration: videoData.duration,
            uploader_name: videoData.uploader?.name,
            uploader_id: videoData.uploader?.id,
            url: videoData.url,
            thumbnail_url: videoData.thumbnailUrl,
            // 字幕数据可选，如果没有字幕则不传递subtitles字段
            ...(subtitles.body && subtitles.body.length > 0 ? {
              subtitles: {
                title: subtitles.title || videoData.title,
                language: subtitles.language || 'Unknown',
                language_code: subtitles.languageCode || 'unknown',
                content: subtitles.body
              }
            } : {}),
            timestamp: new Date().toISOString(),
            source: 'ytb2bili-extension'
          };

          showNotification({
            message: '正在提交到后端...',
            type: 'loading'
          });

          // 提交到后端API
          const result = await videoApi.submitVideoData(submissionData);
          
          if (result.success) {
            const hasSubtitles = subtitles.body && subtitles.body.length > 0;
            const submissionId = result.submission_id || result.task_id;
            showNotification({
              message: `提交成功！${hasSubtitles ? '包含字幕' : '无字幕'}${submissionId ? ` | 提交ID: ${submissionId}` : ''}`,
              type: 'success'
            });
          } else {
            showNotification({
              message: `提交失败: ${result.message}`,
              type: 'error'
            });
          }
          
        } catch (error) {
          showNotification({
            message: error instanceof Error ? error.message : String(error),
            type: 'error'
          });
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