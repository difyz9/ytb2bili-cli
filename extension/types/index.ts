// 消息类型
export enum MessageType {
  SAVE_URL = 'save_url',
  SAVE_SUBTITLE = 'save_subtitle',
  GET_TRANSCRIPT = 'get_transcript',
  DOWNLOAD_SUBTITLE = 'download_subtitle',
  SHOW_NOTIFICATION = 'show_notification',
}

// 操作类型
export enum ActionType {
  SAVE_URL = 'saveUrl',
  SAVE_SUBTITLE = 'saveSubtitle',
}

// 字幕行
export interface SubtitleLine {
  index: number;
  start: string;
  end: string;
  text: string;
}

// 统一视频数据接口
export interface VideoData {
  id: string;         // 视频ID
  url: string;
  title: string;
  channel: string;
  duration: number;
  description: string;
  thumbnail: string;
  subtitles: any[];
  operation: 'save' | 'download';
  format: 'srt' | 'txt' | 'json';
  platform: 'youtube' | 'bilibili';
  // 平台特定字段
  videoId: string; // youtube videoId or bilibili cid
  bvid?: string;     // Bilibili BV号
  aid?: string;      // Bilibili AV号
  cid?: string;      // Bilibili CID
}


// 视频信息
export interface VideoInfo {
  url: string;
  title: string;
  author?: string;
  videoId?: string;
  timestamp?: number;
}

// Bilibili标准字幕格式接口（作为统一格式）
export interface BilibiliSubtitle {
  to: number;      // 结束时间
  sid: number;     // 字幕ID/索引
  from: number;    // 开始时间
  content: string; // 字幕内容
  location: number; // 位置（通常为2）
}

// YouTube原始字幕格式接口
export interface YouTubeSubtitle {
  text: string;     // 字幕内容
  duration: number; // 持续时间
  offset: number;   // 开始时间偏移
  lang?: string;    // 语言
}

// 字幕信息
export interface SubtitleInfo {
  url: string;
  title: string;
  subtitles: SubtitleLine[];
  timestamp?: number;
}

// 消息负载
export interface MessagePayload {
  type: MessageType;
  data?: any;
  actionType?: ActionType;
}

// 通知选项
export interface NotificationOptions {
  message: string;
  type: 'success' | 'error' | 'info';
  duration?: number;
}

// 存储的数据结构
export interface StoredUrl {
  url: string;
  title: string;
  timestamp: number;
}

export interface StoredSubtitle {
  url: string;
  title: string;
  content: string;
  timestamp: number;
}

// 扩展设置
export interface ExtensionSettings {
  deepseekApiKey: string;
  backendUrl: string;
  autoSaveSubtitles: boolean;
}

// 登录信息
export interface LoginTokenInfo {
  mid?: number;
  uname?: string;
  face?: string;
  access_token?: string;
  accessToken?: string;
  refresh_token?: string;
  refreshToken?: string;
  expires_in?: number;
  expiresIn?: number;
}

export interface StoredLoginInfo {
  token_info?: LoginTokenInfo;
  login_time?: number;
  access_token?: string;
  accessToken?: string;
  refresh_token?: string;
  refreshToken?: string;
  token?: string;
  jwt?: string;
}

// ===== 视频列表相关类型 =====

export type VideoStatus = '001' | '002' | '200' | '999';
export type TaskStepStatus = 'pending' | 'running' | 'completed' | 'failed' | 'skipped';

export interface Video {
  id: number;
  video_id: string;
  title: string;
  url: string;
  status: VideoStatus;
  created_at: string;
  updated_at: string;
  generated_title?: string;
  generated_description?: string;
  generated_tags?: string;
  cover_image?: string;
  bili_bvid?: string;
}

export interface TaskStep {
  id: number;
  video_id: string;
  step_name: string;
  step_order: number;
  status: TaskStepStatus;
  start_time?: string;
  end_time?: string;
  duration?: number;
  error_msg?: string;
  result_data?: any;
  can_retry: boolean;
  created_at: string;
  updated_at: string;
}

export interface TaskProgress {
  total_steps: number;
  completed_steps: number;
  failed_steps: number;
  progress_percentage: number;
  current_step?: string;
  is_running: boolean;
}

export interface VideoFile {
  name: string;
  path: string;
  size: number;
  type: 'video' | 'subtitle' | 'cover' | 'metadata' | 'other';
  created_at: string;
}

export interface VideoDetail {
  id: number;
  video_id: string;
  title: string;
  url: string;
  status: VideoStatus;
  created_at: string;
  updated_at: string;
  generated_title?: string;
  generated_description?: string;
  generated_tags?: string;
  cover_image?: string;
  task_steps: TaskStep[];
  progress: TaskProgress;
  files: VideoFile[];
}

export const VIDEO_STATUS_MAP = {
  '001': { label: '待处理', className: 'bg-gray-100 text-gray-800' },
  '002': { label: '处理中', className: 'bg-blue-100 text-blue-800' },
  '200': { label: '已完成', className: 'bg-green-100 text-green-800' },
  '999': { label: '失败', className: 'bg-red-100 text-red-800' },
} as const;

export const TASK_STEP_STATUS_MAP = {
  'pending': { label: '等待中', className: 'bg-gray-100 text-gray-800', color: 'gray' },
  'running': { label: '运行中', className: 'bg-blue-100 text-blue-800', color: 'blue' },
  'completed': { label: '已完成', className: 'bg-green-100 text-green-800', color: 'green' },
  'failed': { label: '失败', className: 'bg-red-100 text-red-800', color: 'red' },
  'skipped': { label: '已跳过', className: 'bg-yellow-100 text-yellow-800', color: 'yellow' },
} as const;

export const TASK_STEP_NAMES = {
  'download_video': '下载视频',
  'generate_subtitles': '生成字幕',
  'translate_subtitles': '翻译字幕',
  'generate_metadata': '生成元数据',
  'upload_to_bilibili': '上传到B站',
  'upload_subtitles': '上传字幕',
} as const;

// 用户信息
export interface User {
  id: string;
  name: string;
  mid: string;
  avatar: string;
  level?: number;
  fans?: number;
  attention?: number;
}

// 认证状态响应
export interface AuthStatusResponse {
  code: number;
  message: string;
  is_logged_in: boolean;
  user?: User;
}

// License/会员类型
export type LicenseType = 'free' | 'basic' | 'standard' | 'pro' | 'enterprise';

// License状态
export interface LicenseStatus {
  is_valid: boolean;
  license_type: LicenseType;
  device_id: string;
  launch_count?: number;
  expired_at?: string;
  features?: string[];
}

// License验证响应
export interface LicenseVerifyResponse {
  code: number;
  message: string;
  data?: LicenseStatus;
}


