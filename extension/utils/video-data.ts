import { YoutubeTranscript, TranscriptItem } from './youtube-transcript';
import { BilibiliTranscript } from './bilibili-transcript';
import { SubtitleNormalizer } from './subtitle-normalizer';

export interface VideoData {
  platform: 'youtube' | 'bilibili';
  videoId: string;
  title: string;
  description?: string;
  duration?: number;
  uploader?: {
    name: string;
    id?: string;
  };
  url: string;
  thumbnailUrl?: string;
  tags?: string[];
}

export interface ExtractedSubtitles {
  title: string;
  language: string;
  languageCode: string;
  body: any[]; // 统一的字幕数据格式
  raw?: any; // 原始字幕数据
}

export class VideoDataExtractor {
  /**
   * 从当前页面提取视频数据和字幕
   */
  static async extractFromCurrentPage(): Promise<{
    videoData: VideoData;
    subtitles: ExtractedSubtitles;
  }> {
    const url = window.location.href;
    
    if (url.includes('youtube.com/watch') || url.includes('youtu.be/')) {
      return await this.extractYouTubeData(url);
    } else if (url.includes('bilibili.com/video/')) {
      return await this.extractBilibiliData(url);
    } else {
      throw new Error('不支持的视频平台');
    }
  }

  /**
   * 转换字幕格式为后端期望的格式
   */
  private static convertSubtitlesToBackendFormat(subtitles: any[], platform: string): Array<{
    text: string;
    duration: number;
    offset: number;
    lang: string;
  }> {
    if (!Array.isArray(subtitles) || subtitles.length === 0) {
      return [];
    }

    return subtitles.map((subtitle) => ({
      text: subtitle.text || subtitle.content || '',
      duration: subtitle.duration || (subtitle.to - subtitle.from) || 0,
      offset: subtitle.offset || subtitle.from || 0,
      lang: subtitle.lang || subtitle.languageCode || 'unknown'
    }));
  }

  /**
   * 提取 YouTube 视频数据和字幕
   */
  static async extractYouTubeData(url: string): Promise<{
    videoData: VideoData;
    subtitles: ExtractedSubtitles;
  }> {
    try {
      // 提取视频ID
      const videoId = YoutubeTranscript.retrieveVideoId(url);
      console.log(`[YouTube] 提取视频ID: ${videoId}`);

      // 从页面获取视频基本信息（DOM 可能因 SPA 导航而滞后）
      const videoData = this.getYouTubeVideoDataFromPage(videoId, url);

      // 并行请求：oEmbed 获取 title / uploader / thumbnail，watch 页源码获取 description
      // 两者都基于 videoId 构造 URL，与 DOM 状态无关，不受 SPA 导航滞后影响
      await Promise.allSettled([
        // oEmbed：title / author / thumbnail
        fetch(`https://www.youtube.com/oembed?url=https://www.youtube.com/watch?v=${videoId}&format=json`)
          .then(r => r.ok ? r.json() : null)
          .then(meta => {
            if (!meta) return;
            if (meta.title) videoData.title = meta.title;
            if (meta.author_name) videoData.uploader = { name: meta.author_name, id: videoData.uploader?.id || '' };
            if (meta.thumbnail_url) videoData.thumbnailUrl = meta.thumbnail_url;
            console.log(`[YouTube] oEmbed 更新: title="${meta.title}", author="${meta.author_name}"`);
          })
          .catch(e => console.warn('[YouTube] oEmbed 获取失败:', e)),

        // watch 页源码：description（ytInitialPlayerResponse.videoDetails.shortDescription）
        fetch(`https://www.youtube.com/watch?v=${videoId}`, {
          headers: { 'Accept-Language': 'en-US,en;q=0.9' }
        })
          .then(r => r.ok ? r.text() : null)
          .then(html => {
            if (!html) return;
            // shortDescription 在 ytInitialPlayerResponse JSON 中，逐字符转义
            const match = html.match(/"shortDescription":"((?:[^"\\]|\\.)*)"/);
            if (match) {
              videoData.description = match[1]
                .replace(/\\n/g, '\n')
                .replace(/\\"/g, '"')
                .replace(/\\\\/g, '\\');
              console.log(`[YouTube] description 已更新 (${videoData.description.length} 字符)`);
            }
          })
          .catch(e => console.warn('[YouTube] description 获取失败:', e)),
      ]);

      // 获取字幕
      let subtitles: ExtractedSubtitles;
      try {
        const transcriptItems = await YoutubeTranscript.fetchTranscript(videoId);
        // 转换为后端期望的格式
        const backendFormatSubtitles = this.convertSubtitlesToBackendFormat(transcriptItems, 'youtube');
        
        subtitles = {
          title: `${videoData.title} - 字幕`,
          language: transcriptItems[0]?.lang || 'unknown',
          languageCode: transcriptItems[0]?.lang || 'unknown',
          body: backendFormatSubtitles,
          raw: transcriptItems
        };
        
        console.log(`[YouTube] 成功获取 ${backendFormatSubtitles.length} 条字幕`);
      } catch (error) {
        console.warn(`[YouTube] 字幕获取失败:`, error);
        subtitles = {
          title: '无字幕',
          language: '无',
          languageCode: 'none',
          body: []
        };
      }

      return { videoData, subtitles };
    } catch (error) {
      console.error('[YouTube] 数据提取失败:', error);
      throw new Error(`YouTube 数据提取失败: ${error instanceof Error ? error.message : String(error)}`);
    }
  }

  /**
   * 提取 Bilibili 视频数据和字幕
   */
  static async extractBilibiliData(url: string): Promise<{
    videoData: VideoData;
    subtitles: ExtractedSubtitles;
  }> {
    try {
      // 获取视频信息
      const videoInfo = await BilibiliTranscript.getCurrentVideoInfo();
      console.log(`[Bilibili] 获取视频信息:`, videoInfo);

      const videoData: VideoData = {
        platform: 'bilibili',
        videoId: videoInfo.bvid,
        title: videoInfo.title,
        description: videoInfo.description,
        duration: videoInfo.duration,
        uploader: {
          name: videoInfo.uploader.name,
          id: videoInfo.uploader.mid
        },
        url: url,
        thumbnailUrl: `https://i0.hdslb.com/bfs/archive/${videoInfo.bvid}.jpg`
      };

      // Bilibili视频不获取字幕，只获取基本视频信息
      const subtitles: ExtractedSubtitles = {
        title: '不获取Bilibili字幕',
        language: '不适用',
        languageCode: 'none',
        body: []
      };
      
      console.log(`[Bilibili] 跳过字幕获取，仅获取视频基本信息`);

      return { videoData, subtitles };
    } catch (error) {
      console.error('[Bilibili] 数据提取失败:', error);
      throw new Error(`Bilibili 数据提取失败: ${error instanceof Error ? error.message : String(error)}`);
    }
  }

  /**
   * 从页面获取 YouTube 视频基本信息
   */
  private static getYouTubeVideoDataFromPage(videoId: string, url: string): VideoData {
    let title = '';
    let description = '';
    let uploader = { name: '', id: '' };
    let duration = 0;

    try {
      // 优先使用 document.title：YouTube SPA 跳转时会动态更新，og:title 不会更新
      const docTitle = document.title.replace(' - YouTube', '').trim();
      if (docTitle && docTitle !== 'YouTube') {
        title = docTitle;
      }

      // 备用：从 YouTube 动态渲染的DOM元素中读取标题
      if (!title) {
        const ytTitle =
          document.querySelector('h1.ytd-watch-metadata yt-formatted-string') ||
          document.querySelector('h1.ytd-video-primary-info-renderer yt-formatted-string') ||
          document.querySelector('#title h1 yt-formatted-string');
        if (ytTitle) {
          title = ytTitle.textContent?.trim() || '';
        }
      }

      // 最后才尝试 og:title（仅在完整页面加载时准确，SPA 跳转后是陈旧数据）
      if (!title) {
        const titleElement = document.querySelector('meta[property="og:title"]') as HTMLMetaElement;
        if (titleElement) {
          title = titleElement.content;
        }
      }

      // 最终兜底
      if (!title) {
        title = `YouTube Video ${videoId}`;
      }

      // 尝试从 YouTube 动态渲染的 DOM 读取描述（og:description SPA 跳转后同样是陈旧数据）
      const descElement = document.querySelector(
        'ytd-expander #description-inline-expander, ytd-text-inline-expander #snippet-text'
      );
      if (descElement) {
        description = descElement.textContent?.trim() || '';
      }

      // 尝试获取频道信息
      const channelElement = document.querySelector('ytd-channel-name a, .ytd-video-owner-renderer a');
      if (channelElement) {
        uploader.name = channelElement.textContent?.trim() || '';
      }
    } catch (error) {
      console.warn('[YouTube] 从页面提取信息失败，使用默认值:', error);
      title = title || `YouTube Video ${videoId}`;
    }

    return {
      platform: 'youtube',
      videoId,
      title,
      description,
      duration,
      uploader,
      url,
      thumbnailUrl: `https://img.youtube.com/vi/${videoId}/maxresdefault.jpg`
    };
  }

  /**
   * 验证提取的数据
   */
  static validateVideoData(videoData: VideoData, subtitles: ExtractedSubtitles): boolean {
    if (!videoData.videoId || !videoData.title || !videoData.platform) {
      console.error('视频数据不完整:', videoData);
      return false;
    }

    if (!subtitles.languageCode) {
      console.warn('字幕数据不完整，但允许无字幕情况');
    }

    return true;
  }
}