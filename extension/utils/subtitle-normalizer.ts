/**
 * 字幕格式统一转换器
 * 将YouTube字幕格式转换为与Bilibili一致的格式
 */

import { BilibiliSubtitle, YouTubeSubtitle } from '../types';

/**
 * 字幕格式统一转换器类
 * 统一方式：将YouTube字幕转换为Bilibili格式
 */
export class SubtitleNormalizer {
  
  /**
   * 将YouTube字幕转换为Bilibili格式
   * @param youtubeSubtitles YouTube原始字幕数组
   * @returns Bilibili格式字幕数组
   */
  static convertYouTubeToBilibili(youtubeSubtitles: YouTubeSubtitle[]): BilibiliSubtitle[] {
    if (!Array.isArray(youtubeSubtitles)) {
      console.warn('[字幕转换器] YouTube字幕数据不是数组格式');
      return [];
    }

    console.log(`[字幕转换器] 开始转换YouTube字幕 → Bilibili格式，原始数量: ${youtubeSubtitles.length}`);

    const converted = youtubeSubtitles.map((subtitle, index) => {
      const from = subtitle.offset || 0;
      const to = from + (subtitle.duration || 0);
      
      return {
        sid: index + 1,           // 字幕ID从1开始
        from: from,               // 开始时间
        to: to,                   // 结束时间
        content: subtitle.text || '', // 字幕内容
        location: 2               // 默认位置
      };
    });

    console.log(`[字幕转换器] YouTube → Bilibili 转换完成，转换数量: ${converted.length}`);
    
    // 显示转换示例
    if (converted.length > 0) {
      console.log('[字幕转换器] 转换示例:');
      console.log('  原始格式:', youtubeSubtitles[0]);
      console.log('  转换后:', converted[0]);
    }

    return converted;
  }
  
  /**
   * 验证并修复Bilibili字幕格式
   * @param bilibiliSubtitles Bilibili原始字幕数组
   * @returns 验证后的Bilibili格式字幕数组
   */
  static validateBilibiliFormat(bilibiliSubtitles: any[]): BilibiliSubtitle[] {
    if (!Array.isArray(bilibiliSubtitles)) {
      console.warn('[字幕转换器] Bilibili字幕数据不是数组格式');
      return [];
    }

    console.log(`[字幕转换器] 验证Bilibili字幕格式，数量: ${bilibiliSubtitles.length}`);

    const validated = bilibiliSubtitles.map((subtitle, index) => {
      // 确保所有必要字段存在且格式正确
      return {
        sid: subtitle.sid || (index + 1),
        from: typeof subtitle.from === 'number' ? subtitle.from : 0,
        to: typeof subtitle.to === 'number' ? subtitle.to : (subtitle.from || 0),
        content: subtitle.content || '',
        location: subtitle.location || 2
      };
    });

    console.log(`[字幕转换器] Bilibili字幕验证完成，有效数量: ${validated.length}`);
    return validated;
  }
  
  /**
   * 统一字幕格式转换入口
   * 将不同平台的字幕统一转换为Bilibili格式
   * @param subtitles 原始字幕数组
   * @param platform 平台类型
   * @returns Bilibili格式字幕数组
   */
  static normalizeSubtitles(subtitles: any[], platform: string): BilibiliSubtitle[] {
    if (!subtitles || subtitles.length === 0) {
      console.log('[字幕转换器] 字幕数组为空，返回空数组');
      return [];
    }
    
    console.log(`[字幕转换器] 开始统一字幕格式，平台: ${platform}，原始数量: ${subtitles.length}`);
    
    try {
      switch (platform.toLowerCase()) {
        case 'youtube':
          return this.convertYouTubeToBilibili(subtitles as YouTubeSubtitle[]);
          
        case 'bilibili':
          console.log('[字幕转换器] Bilibili字幕已是目标格式，进行验证');
          return this.validateBilibiliFormat(subtitles);
          
        default:
          console.warn(`[字幕转换器] 不支持的平台: ${platform}，返回空数组`);
          return [];
      }
    } catch (error) {
      console.error(`[字幕转换器] 字幕格式转换失败:`, error);
      console.error(`[字幕转换器] 原始字幕数据示例:`, subtitles.slice(0, 2));
      return [];
    }
  }
  
  /**
   * 检查字幕格式类型
   * @param subtitles 字幕数组
   * @returns 字幕格式类型
   */
  static detectSubtitleFormat(subtitles: any[]): 'youtube' | 'bilibili' | 'unknown' {
    if (!Array.isArray(subtitles) || subtitles.length === 0) {
      return 'unknown';
    }

    const firstItem = subtitles[0];
    
    // 检查YouTube格式特征
    if (firstItem.hasOwnProperty('text') && 
        firstItem.hasOwnProperty('duration') && 
        firstItem.hasOwnProperty('offset')) {
      return 'youtube';
    }
    
    // 检查Bilibili格式特征
    if (firstItem.hasOwnProperty('content') && 
        firstItem.hasOwnProperty('from') && 
        firstItem.hasOwnProperty('to')) {
      return 'bilibili';
    }
    
    return 'unknown';
  }
  
  /**
   * 验证字幕格式是否正确
   * @param subtitles 字幕数组
   * @param platform 平台类型
   * @returns 是否为正确格式
   */
  static validateSubtitleFormat(subtitles: any[], platform: string): boolean {
    if (!subtitles || subtitles.length === 0) {
      return true; // 空数组认为是有效的
    }
    
    const firstSubtitle = subtitles[0];
    
    switch (platform.toLowerCase()) {
      case 'youtube':
        return this.isYouTubeSubtitle(firstSubtitle);
        
      case 'bilibili':
        return this.isBilibiliSubtitle(firstSubtitle);
        
      default:
        return false;
    }
  }
  
  /**
   * 检查是否为YouTube字幕格式
   */
  private static isYouTubeSubtitle(subtitle: any): boolean {
    return subtitle &&
           typeof subtitle.text === 'string' &&
           typeof subtitle.duration === 'number' &&
           typeof subtitle.offset === 'number';
  }
  
  /**
   * 检查是否为Bilibili字幕格式
   */
  private static isBilibiliSubtitle(subtitle: any): boolean {
    return subtitle &&
           typeof subtitle.to === 'number' &&
           typeof subtitle.from === 'number' &&
           typeof subtitle.content === 'string' &&
           typeof subtitle.sid === 'number';
  }
  
  /**
   * 获取字幕统计信息
   * @param subtitles Bilibili格式字幕数组
   * @returns 统计信息
   */
  static getSubtitleStats(subtitles: BilibiliSubtitle[]): {
    totalCount: number;
    totalDuration: number;
    averageDuration: number;
    firstSubtitle?: BilibiliSubtitle;
    lastSubtitle?: BilibiliSubtitle;
  } {
    if (!subtitles || subtitles.length === 0) {
      return {
        totalCount: 0,
        totalDuration: 0,
        averageDuration: 0
      };
    }
    
    const totalDuration = subtitles.reduce((sum, subtitle) => sum + (subtitle.to - subtitle.from), 0);
    const averageDuration = totalDuration / subtitles.length;
    
    return {
      totalCount: subtitles.length,
      totalDuration,
      averageDuration,
      firstSubtitle: subtitles[0],
      lastSubtitle: subtitles[subtitles.length - 1]
    };
  }
  
  /**
   * 格式化时间显示
   * @param seconds 秒数
   * @returns 格式化的时间字符串
   */
  static formatTime(seconds: number): string {
    if (isNaN(seconds) || seconds < 0) return '00:00';
    
    const hours = Math.floor(seconds / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    const secs = Math.floor(seconds % 60);
    
    if (hours > 0) {
      return `${hours.toString().padStart(2, '0')}:${minutes.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`;
    } else {
      return `${minutes.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`;
    }
  }
  
  /**
   * 在控制台打印字幕转换摘要
   * @param originalSubtitles 原始字幕
   * @param normalizedSubtitles 转换后字幕
   * @param platform 平台类型
   */
  static logConversionSummary(
    originalSubtitles: any[], 
    normalizedSubtitles: BilibiliSubtitle[], 
    platform: string
  ): void {
    const stats = this.getSubtitleStats(normalizedSubtitles);
    
    console.group(`[字幕转换器] ${platform.toUpperCase()} → Bilibili 格式转换摘要`);
    console.log(`📊 原始字幕数量: ${originalSubtitles.length}`);
    console.log(`📊 转换后数量: ${stats.totalCount}`);
    console.log(`⏱️ 总时长: ${this.formatTime(stats.totalDuration)}`);
    console.log(`⏱️ 平均时长: ${stats.averageDuration.toFixed(2)}秒`);
    
    if (stats.firstSubtitle) {
      console.log(`📝 首字幕: [${this.formatTime(stats.firstSubtitle.from)}-${this.formatTime(stats.firstSubtitle.to)}] ${stats.firstSubtitle.content.substring(0, 30)}...`);
    }
    
    if (stats.lastSubtitle) {
      console.log(`📝 末字幕: [${this.formatTime(stats.lastSubtitle.from)}-${this.formatTime(stats.lastSubtitle.to)}] ${stats.lastSubtitle.content.substring(0, 30)}...`);
    }
    
    console.log(`✅ 转换状态: ${stats.totalCount > 0 ? '成功' : '失败'}`);
    console.groupEnd();
  }

  /**
   * 获取转换摘要信息（用于返回给调用者）
   */
  static getConversionSummary(originalSubtitles: any[], convertedSubtitles: BilibiliSubtitle[], platform: string) {
    const stats = this.getSubtitleStats(convertedSubtitles);
    
    return {
      platform,
      originalCount: originalSubtitles.length,
      convertedCount: stats.totalCount,
      success: stats.totalCount > 0,
      totalDuration: Math.round(stats.totalDuration * 100) / 100,
      averageDuration: Math.round(stats.averageDuration * 100) / 100,
      firstContent: stats.firstSubtitle?.content.substring(0, 50) || '',
      lastContent: stats.lastSubtitle?.content.substring(0, 50) || '',
      format: 'bilibili' // 目标格式
    };
  }
}

export default SubtitleNormalizer;
