const RE_YOUTUBE =
  /(?:youtube\.com\/(?:[^\/]+\/.+\/|(?:v|e(?:mbed)?)\/|.*[?&]v=)|youtu\.be\/)([^"&?\/\s]{11})/i;
const USER_AGENT =
  'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_4) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/85.0.4183.83 Safari/537.36,gzip(gfe)';
const RE_XML_TRANSCRIPT =
  /<text start="([^"]*)" dur="([^"]*)">([^<]*)<\/text>/g;

// 存储截取到的字幕数据
const transcriptCache = new Map<string, { text: string; url: string; timestamp: number }>();

// 拦截fetch请求来直接获取字幕响应
const originalFetch = window.fetch;
window.fetch = function(...args: Parameters<typeof fetch>) {
  const [resource] = args;
  const url = typeof resource === 'string' ? resource : (resource as Request).url;
  
  if (url.includes('youtube.com/api/timedtext')) {
    return originalFetch.apply(this, args).then(response => {
      // 克隆响应以便我们可以读取它而不影响原始请求
      const clonedResponse = response.clone();
      
      // 从URL中提取视频ID
      try {
        const urlObj = new URL(url);
        const videoId = urlObj.searchParams.get('v');
        
        if (videoId) {
          // 异步存储响应内容
          clonedResponse.text().then(text => {
            transcriptCache.set(videoId, {
              text: text,
              url: url,
              timestamp: Date.now()
            });
            console.log(`Intercepted transcript for video: ${videoId}`);
          }).catch(err => {
            console.error('Error reading transcript response:', err);
          });
        }
      } catch (err) {
        console.error('Error parsing timedtext URL:', err);
      }
      
      return response;
    });
  }
  
  return originalFetch.apply(this, args);
};

// 从缓存获取字幕数据的辅助函数
function getCachedTranscript(videoId: string): string | null {
  const cached = transcriptCache.get(videoId);
  if (cached) {
    // 检查缓存是否过期（5分钟）
    const isExpired = Date.now() - cached.timestamp > 5 * 60 * 1000;
    if (!isExpired) {
      return cached.text;
    } else {
      transcriptCache.delete(videoId);
    }
  }
  return null;
}

// 从多个来源获取pot参数的辅助函数
async function getPotParameter(videoId: string): Promise<string | null> {
  return new Promise((resolve) => {
    // 首先尝试从content script的页面拦截器获取
    if (typeof window !== 'undefined' && typeof (window as any).WXT_getPotParameter === 'function') {
      const pagePot = (window as any).WXT_getPotParameter(videoId);
      if (pagePot) {
        console.log(`[WXT Subtitle] 从页面拦截器获取到pot参数: ${pagePot}`);
        resolve(pagePot);
        return;
      }
    }
    
    // 备用方案：从background script获取
    if (typeof (globalThis as any).chrome !== 'undefined' && (globalThis as any).chrome.runtime) {
      (globalThis as any).chrome.runtime.sendMessage(
        { action: "getPotParameter", videoId: videoId }, 
        (response: any) => {
          const bgPot = response?.pot;
          if (bgPot) {
            console.log(`[WXT Subtitle] 从background获取到pot参数: ${bgPot}`);
          } else {
            console.log(`[WXT Subtitle] 未找到视频 ${videoId} 的pot参数`);
          }
          resolve(bgPot);
        }
      );
    } else {
      console.log(`[WXT Subtitle] Chrome runtime不可用，无法获取pot参数`);
      resolve(null);
    }
  });
}

export class YoutubeTranscriptError extends Error {
  constructor(message: string) {
    super(`[YoutubeTranscript] 🚨 ${message}`);
  }
}

export class YoutubeTranscriptTooManyRequestError extends YoutubeTranscriptError {
  constructor() {
    super(
      'YouTube is receiving too many requests from this IP and now requires solving a captcha to continue'
    );
  }
}

export class YoutubeTranscriptVideoUnavailableError extends YoutubeTranscriptError {
  constructor(videoId: string) {
    super(`The video is no longer available (${videoId})`);
  }
}

export class YoutubeTranscriptDisabledError extends YoutubeTranscriptError {
  constructor(videoId: string) {
    super(`Transcript is disabled on this video (${videoId})`);
  }
}

export class YoutubeTranscriptNotAvailableError extends YoutubeTranscriptError {
  constructor(videoId: string) {
    super(`No transcripts are available for this video (${videoId})`);
  }
}

export class YoutubeTranscriptNotAvailableLanguageError extends YoutubeTranscriptError {
  constructor(lang: string, availableLangs: string[], videoId: string) {
    super(
      `No transcripts are available in ${lang} for this video (${videoId}). Available languages: ${availableLangs.join(
        ', '
      )}`
    );
  }
}

export interface TranscriptConfig {
  lang?: string;
}

export interface TranscriptItem {
  text: string;
  duration: number;
  offset: number;
  lang: string;
}

/**
 * Class to retrieve transcript if exist
 */
export class YoutubeTranscript {
  /**
   * Fetch transcript from YouTube Video
   * @param videoId Video url or video identifier
   * @param config Get transcript in a specific language ISO
   */
  static async fetchTranscript(videoId: string, config?: TranscriptConfig): Promise<TranscriptItem[]> {
    const identifier = this.retrieveVideoId(videoId);
    
    // 首先尝试从缓存获取字幕数据
    const cachedTranscript = getCachedTranscript(identifier);
    if (cachedTranscript) {
      console.log(`Using cached transcript for video: ${identifier}`);
      return this.parseTranscriptResponse(cachedTranscript, config, identifier);
    }
    
    const videoPageResponse = await fetch(
      `https://www.youtube.com/watch?v=${identifier}`,
      {
        headers: {
          ...(config?.lang && { 'Accept-Language': config.lang }),
          'User-Agent': USER_AGENT,
        },
      }
    );
    const videoPageBody = await videoPageResponse.text();

    const splittedHTML = videoPageBody.split('"captions":');

    if (splittedHTML.length <= 1) {
      if (videoPageBody.includes('class="g-recaptcha"')) {
        throw new YoutubeTranscriptTooManyRequestError();
      }
      if (!videoPageBody.includes('"playabilityStatus":')) {
        throw new YoutubeTranscriptVideoUnavailableError(videoId);
      }
      throw new YoutubeTranscriptDisabledError(videoId);
    }

    const captions = (() => {
      try {
        return JSON.parse(
          splittedHTML[1].split(',"videoDetails')[0].replace('\n', '')
        );
      } catch (e) {
        return undefined;
      }
    })()?.['playerCaptionsTracklistRenderer'];

    if (!captions) {
      throw new YoutubeTranscriptDisabledError(videoId);
    }

    if (!('captionTracks' in captions)) {
      throw new YoutubeTranscriptNotAvailableError(videoId);
    }

    // 优先选择指定语言的字幕，否则选择第一个可用的
    let selectedTrack;
    if (config?.lang) {
      selectedTrack = captions.captionTracks.find(
        (track: any) => track.languageCode === config?.lang
      );
      if (!selectedTrack) {
        // 尝试部分匹配（例如 zh 匹配 zh-CN）
        selectedTrack = captions.captionTracks.find(
          (track: any) => track.languageCode.startsWith(config.lang!.split('-')[0])
        );
      }
      if (!selectedTrack) {
        throw new YoutubeTranscriptNotAvailableLanguageError(
          config?.lang,
          captions.captionTracks.map((track: any) => track.languageCode),
          videoId
        );
      }
    } else {
      selectedTrack = captions.captionTracks[0];
    }

    let transcriptURL = selectedTrack.baseUrl;

    // 添加pot参数到transcriptURL
    const cachedPot = await getPotParameter(identifier);
    if (cachedPot) {
      const url = new URL(transcriptURL);
      url.searchParams.set('pot', cachedPot);
      url.searchParams.set('fmt', "json3");
      url.searchParams.set('c', "WEB");
      transcriptURL = url.toString();
      console.log(`Using pot parameter: ${cachedPot} for video: ${identifier}`);
    } else {
      console.warn(`No pot parameter found for video: ${identifier}, using original URL`);
    }

    const transcriptResponse = await fetch(transcriptURL, {
      headers: {
        ...(config?.lang && { 'Accept-Language': config.lang }),
        'User-Agent': USER_AGENT,
      },
    });
    if (!transcriptResponse.ok) {
      throw new YoutubeTranscriptNotAvailableError(videoId);
    }
    const transcriptBody = await transcriptResponse.text();

    console.log('transcriptURL', transcriptURL);
    console.log('transcriptBody', transcriptBody);
    
    return this.parseTranscriptResponse(transcriptBody, config, identifier, captions);
  }

  /**
   * Parse transcript response (JSON or XML format)
   * @param transcriptBody Response body text
   * @param config Configuration object
   * @param identifier Video identifier
   * @param captions Caption tracks info (optional)
   */
  static parseTranscriptResponse(transcriptBody: string, config?: TranscriptConfig, identifier?: string, captions?: any): TranscriptItem[] {
    // 首先检查响应是否为空或无效
    if (!transcriptBody || transcriptBody.trim().length === 0) {
      throw new YoutubeTranscriptError('Empty transcript response');
    }
    
    try {
      // 尝试解析 JSON 格式 (新格式)
      const jsonData = JSON.parse(transcriptBody);
      if (jsonData.events) {
        const results: TranscriptItem[] = [];
        
        jsonData.events.forEach((event: any) => {
          if (event.segs) {
            // 合并所有文本片段
            let fullText = '';
            event.segs.forEach((seg: any) => {
              if (seg.utf8) {
                fullText += seg.utf8;
              }
            });
            
            if (fullText.trim() && fullText.trim() !== '\n') {
              results.push({
                text: fullText.trim(),
                duration: (event.dDurationMs || 0) / 1000, // 转换为秒
                offset: (event.tStartMs || 0) / 1000, // 转换为秒
                lang: config?.lang ?? (captions ? captions.captionTracks[0].languageCode : 'unknown'),
              });
            }
          }
        });
        
        if (results.length > 0) {
          return results;
        }
      }
    } catch (e) {
      console.log('JSON parsing failed, trying XML parsing:', e);
    }
    
    // 如果 JSON 解析失败，尝试 XML 格式解析
    try {
      const results = [...transcriptBody.matchAll(RE_XML_TRANSCRIPT)];
      if (results.length > 0) {
        return results.map((result) => ({
          text: result[3] ? result[3].replace(/&amp;/g, '&').replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&quot;/g, '"') : '',
          duration: parseFloat(result[2]) || 0,
          offset: parseFloat(result[1]) || 0,
          lang: config?.lang ?? (captions ? captions.captionTracks[0].languageCode : 'unknown'),
        })).filter(item => item.text.trim().length > 0);
      }
    } catch (e) {
      console.log('XML parsing also failed:', e);
    }
    
    // 如果两种格式都解析失败，检查是否是HTML错误页面
    if (transcriptBody.includes('<!DOCTYPE html>') || transcriptBody.includes('<html')) {
      throw new YoutubeTranscriptError('Received HTML page instead of transcript data. This video may not have transcripts available.');
    }
    
    throw new YoutubeTranscriptError('Unable to parse transcript response. The response may be in an unexpected format.');
  }

  /**
   * Retrieve video id from url or string
   * @param videoId video url or video id
   */
  static retrieveVideoId(videoId: string): string {
    if (videoId.length === 11) {
      return videoId;
    }
    const matchId = videoId.match(RE_YOUTUBE);
    if (matchId && matchId.length) {
      return matchId[1];
    }
    throw new YoutubeTranscriptError(
      'Impossible to retrieve Youtube video ID.'
    );
  }
}
