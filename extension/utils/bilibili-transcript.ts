export class BilibiliTranscript {
  /**
   * 获取用户认证信息 (从浏览器环境中自动获取)
   */
  static async getUserInfo() {
    try {
      // 从页面中获取用户信息
      const userInfo = (window as any).__INITIAL_STATE__?.userInfo;
      if (userInfo && userInfo.isLogin) {
        console.log(`[👤 用户信息] 已登录用户: ${userInfo.uname} (UID: ${userInfo.mid})`);
        return userInfo;
      }
      
      // 尝试从API获取用户信息
      const response = await fetch('https://api.bilibili.com/x/web-interface/nav', {
        credentials: 'include',
        headers: {
          'Referer': 'https://www.bilibili.com/',
          'User-Agent': navigator.userAgent
        }
      });
      
      const data = await response.json();
      if (data.code === 0 && data.data.isLogin) {
        console.log(`[👤 用户信息] 已登录用户: ${data.data.uname} (UID: ${data.data.mid})`);
        return data.data;
      }
      
      console.log('[👤 用户信息] 用户未登录，使用游客模式');
      return null;
    } catch (error) {
      console.warn('[👤 用户信息] 获取用户信息失败，使用游客模式:', error);
      return null;
    }
  }

  /**
   * 获取当前视频信息
   */
  static async getCurrentVideoInfo() {
    try {
      // 获取用户信息
      const userInfo = await this.getUserInfo();
      
      // 从URL参数获取当前分P
      const urlParams = new URLSearchParams(window.location.search);
      const p = urlParams.get('p') || '1';
      const currentPart = parseInt(p);
      
      // 从 URL 提取视频 ID
      const videoId = this.extractVideoId(window.location.href);
      if (!videoId) {
        throw new Error('无法从URL中提取视频ID');
      }

      let aid: string;
      let cid: string;
      let videoData: any;

      // 构建请求头，包含完整的浏览器环境信息
      const headers = {
        'Referer': 'https://www.bilibili.com/',
        'User-Agent': navigator.userAgent,
        'Accept': 'application/json, text/plain, */*',
        'Accept-Language': 'zh-CN,zh;q=0.9,en;q=0.8',
        'Origin': 'https://www.bilibili.com',
        'Sec-Fetch-Dest': 'empty',
        'Sec-Fetch-Mode': 'cors',
        'Sec-Fetch-Site': 'same-site'
      };

      if (videoId.type === 'bvid') {
        // 通过 bvid 获取视频信息
        const response = await fetch(
          `https://api.bilibili.com/x/web-interface/view?bvid=${videoId.id}`,
          { 
            credentials: 'include',
            headers
          }
        );
        const data = await response.json();
        
        if (data.code !== 0 || !data.data) {
          throw new Error(`获取视频信息失败: ${data.message || '未知错误'}`);
        }

        videoData = data.data;
        aid = videoData.aid;
        const pages = videoData.pages || [];
        
        if (pages.length > 0) {
          // 根据当前分P号查找对应的CID
          const targetPage = pages.find((page: any) => page.page === currentPart);
          cid = targetPage ? targetPage.cid : pages[0].cid;
        } else {
          cid = videoData.cid;
        }
      } else {
        // 通过 aid 获取视频信息
        aid = videoId.id;
        
        const response = await fetch(
          `https://api.bilibili.com/x/web-interface/view?aid=${aid}`,
          { 
            credentials: 'include',
            headers
          }
        );
        const data = await response.json();
        
        if (data.code !== 0 || !data.data) {
          throw new Error(`获取视频信息失败: ${data.message || '未知错误'}`);
        }

        videoData = data.data;
        const pages = videoData.pages || [];
        
        if (pages.length > 0) {
          // 根据当前分P号查找对应的CID
          const targetPage = pages.find((page: any) => page.page === currentPart);
          cid = targetPage ? targetPage.cid : pages[0].cid;
        } else {
          cid = videoData.cid;
        }
      }

      if (!aid || !cid) {
        throw new Error('无法获取视频aid或cid');
      }

      console.log(`[📱 视频信息] 视频ID: ${videoId.id} | AID: ${aid} | CID: ${cid} | 分P: ${currentPart}`);
      
      return {
        bvid: videoData.bvid,
        aid: parseInt(aid),
        cid: parseInt(cid),
        title: videoData.title,
        description: videoData.desc,
        duration: videoData.duration,
        uploader: {
          name: videoData.owner?.name || '',
          mid: videoData.owner?.mid || ''
        },
        currentPage: currentPart,
        totalPages: videoData.pages?.length || 1,
        userInfo: userInfo
      };
    } catch (error) {
      console.error('[❌ 视频信息] 获取视频信息失败:', error);
      throw error;
    }
  }

  /**
   * 从URL中提取视频ID
   */
  static extractVideoId(url: string): { type: 'bvid' | 'aid'; id: string } | null {
    // 匹配 BV 号
    const bvidMatch = url.match(/\/video\/(BV[a-zA-Z0-9]+)/);
    if (bvidMatch) {
      return { type: 'bvid', id: bvidMatch[1] };
    }
    
    // 匹配 aid
    const aidMatch = url.match(/\/video\/av(\d+)/);
    if (aidMatch) {
      return { type: 'aid', id: aidMatch[1] };
    }
    
    return null;
  }

  /**
   * 获取视频字幕
   */
  static async getSubtitles(aid: number, cid: number, userInfo?: any) {
    try {
      console.log(`[📝 字幕获取] 开始获取字幕 - AID: ${aid}, CID: ${cid}`);
      
      const headers = {
        'Referer': 'https://www.bilibili.com/',
        'User-Agent': navigator.userAgent,
        'Accept': 'application/json, text/plain, */*',
        'Accept-Language': 'zh-CN,zh;q=0.9,en;q=0.8'
      };
      
      // 获取字幕列表
      const subtitleListUrl = `https://api.bilibili.com/x/player/v2?aid=${aid}&cid=${cid}`;
      
      console.log(`[📝 字幕获取] 请求字幕列表: ${subtitleListUrl}`);
      
      const response = await fetch(subtitleListUrl, {
        credentials: 'include',
        headers
      });
      
      if (!response.ok) {
        throw new Error(`字幕列表请求失败: HTTP ${response.status}`);
      }
      
      const data = await response.json();
      
      if (data.code !== 0) {
        throw new Error(`字幕列表获取失败: ${data.message || 'API返回错误'}`);
      }
      
      const subtitles = data.data?.subtitle?.subtitles || [];
      
      if (subtitles.length === 0) {
        console.log('[📝 字幕获取] 该视频没有字幕');
        return { title: '', language: '', languageCode: '', body: [] };
      }
      
      // 选择第一个可用字幕
      const selectedSubtitle = subtitles[0];
      console.log(`[📝 字幕获取] 选择字幕: ${selectedSubtitle.lan_doc || selectedSubtitle.lan} (${selectedSubtitle.lan})`);
      
      // 获取字幕内容
      const subtitleResponse = await fetch(selectedSubtitle.subtitle_url, {
        headers: {
          'Referer': 'https://www.bilibili.com/',
          'User-Agent': navigator.userAgent,
          'Accept': 'application/json, text/plain, */*'
        }
      });
      
      if (!subtitleResponse.ok) {
        throw new Error(`获取字幕内容失败: HTTP ${subtitleResponse.status}`);
      }
      
      const subtitleData = await subtitleResponse.json();
      
      if (!subtitleData.body || !Array.isArray(subtitleData.body)) {
        throw new Error('字幕数据格式错误或为空');
      }
      
      console.log(`[📝 字幕获取] 成功获取 ${subtitleData.body.length} 条字幕条目`);
      
      return {
        title: selectedSubtitle.lan_doc || selectedSubtitle.lan || '字幕',
        language: selectedSubtitle.lan_doc || selectedSubtitle.lan || '未知语言',
        languageCode: selectedSubtitle.lan || 'unknown',
        body: subtitleData.body,
        subtitles: subtitles // 返回所有可用字幕选项
      };
    } catch (error) {
      console.error('[❌ 字幕获取] 获取字幕失败:', error);
      throw error;
    }
  }
}