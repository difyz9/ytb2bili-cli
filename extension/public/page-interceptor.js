// 页面拦截脚本 - 需要在页面上下文中执行以拦截原生请求
(function() {
    'use strict';
    
    // 存储拦截到的pot参数
    const potStorage = {};
    
    // 向content script发送消息的辅助函数
    function sendToContentScript(data) {
        window.postMessage({
            type: 'WEB_EXTENSION_POT_INTERCEPTED',
            source: 'page-interceptor',
            data: data
        }, '*');
    }
    
    // 拦截 fetch 方法
    const originalFetch = window.fetch;
    window.fetch = function(input, init) {
        const url = typeof input === 'string' ? input : input.url;
        
        if (url && (url.includes('/api/timedtext') || url.includes('timedtext'))) {
            try {
                const urlObj = new URL(url, location.href);
                const pot = urlObj.searchParams.get('pot');
                const videoId = urlObj.searchParams.get('v');
                
                if (pot && videoId) {
                    console.log('[YouTube Extension] Fetch拦截到字幕请求pot:', pot, 'videoId:', videoId);
                    potStorage[videoId] = pot;
                    
                    sendToContentScript({
                        type: 'pot_parameter',
                        videoId: videoId,
                        pot: pot,
                        url: url
                    });
                } else if (pot) {
                    const currentVideoId = getCurrentVideoId();
                    if (currentVideoId) {
                        console.log('[YouTube Extension] Fetch拦截到字幕请求pot:', pot, '推断videoId:', currentVideoId);
                        potStorage[currentVideoId] = pot;
                        
                        sendToContentScript({
                            type: 'pot_parameter',
                            videoId: currentVideoId,
                            pot: pot,
                            url: url
                        });
                    }
                }
            } catch(e) { 
                console.error('[YouTube Extension] 解析字幕URL出错', e); 
            }
        }
        
        return originalFetch.apply(this, arguments);
    };
    
    // 拦截 XMLHttpRequest 方法
    const originalOpen = XMLHttpRequest.prototype.open;
    XMLHttpRequest.prototype.open = function(method, url) {
        if (typeof url === 'string' && (url.includes('/api/timedtext') || url.includes('timedtext'))) {
            try {
                const urlObj = new URL(url, location.href);
                const pot = urlObj.searchParams.get('pot');
                const videoId = urlObj.searchParams.get('v');
                
                if (pot && videoId) {
                    console.log('[YouTube Extension] XHR拦截到字幕请求pot:', pot, 'videoId:', videoId);
                    potStorage[videoId] = pot;
                    
                    sendToContentScript({
                        type: 'pot_parameter',
                        videoId: videoId,
                        pot: pot,
                        url: url
                    });
                } else if (pot) {
                    const currentVideoId = getCurrentVideoId();
                    if (currentVideoId) {
                        console.log('[YouTube Extension] XHR拦截到字幕请求pot:', pot, '推断videoId:', currentVideoId);
                        potStorage[currentVideoId] = pot;
                        
                        sendToContentScript({
                            type: 'pot_parameter',
                            videoId: currentVideoId,
                            pot: pot,
                            url: url
                        });
                    }
                }
            } catch(e) { 
                console.error('[YouTube Extension] 解析字幕URL出错', e); 
            }
        }
        
        return originalOpen.apply(this, arguments);
    };
    
    // 从当前页面URL获取视频ID的辅助函数
    function getCurrentVideoId() {
        try {
            const url = new URL(window.location.href);
            return url.searchParams.get('v');
        } catch(e) {
            return null;
        }
    }
    
    // 提供获取pot参数的全局函数
    window.WebExtension_getPotParameter = function(videoId) {
        return potStorage[videoId] || null;
    };
    
    console.log('[YouTube Extension] 页面拦截器已注入，开始监听字幕请求');
    
    // 监听页面URL变化
    let currentVideoId = getCurrentVideoId();
    const observer = new MutationObserver(function(mutations) {
        const newVideoId = getCurrentVideoId();
        if (newVideoId && newVideoId !== currentVideoId) {
            console.log('[YouTube Extension] 检测到视频切换，从', currentVideoId, '到', newVideoId);
            currentVideoId = newVideoId;
        }
    });
    
    if (document.body) {
        observer.observe(document.body, { childList: true, subtree: true });
    } else {
        document.addEventListener('DOMContentLoaded', function() {
            if (document.body) {
                observer.observe(document.body, { childList: true, subtree: true });
            }
        });
    }
})();
