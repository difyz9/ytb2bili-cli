import React from 'react';
import type { VideoStatus } from '../types';

const STATUS_CONFIG = {
  '001': {
    label: '待处理',
    className: 'bg-gray-100 text-gray-800',
    icon: '⏱️',
    description: '视频已提交，等待处理'
  },
  '002': {
    label: '处理中',
    className: 'bg-blue-100 text-blue-800',
    icon: '🔄',
    description: '正在下载和处理视频'
  },
  '200': {
    label: '已完成',
    className: 'bg-green-100 text-green-800',
    icon: '✅',
    description: '视频处理完成并已上传'
  },
  '999': {
    label: '失败',
    className: 'bg-red-100 text-red-800',
    icon: '❌',
    description: '处理过程中出现错误'
  },
} as const;

interface StatusBadgeProps {
  status: VideoStatus;
  showDescription?: boolean;
}

export const StatusBadge: React.FC<StatusBadgeProps> = ({ status, showDescription = false }) => {
  const config = STATUS_CONFIG[status];
  
  if (!config) {
    return (
      <span className="bg-gray-100 text-gray-800 px-2 py-1 rounded-full text-xs flex items-center space-x-1">
        <span>❓</span>
        <span>未知状态</span>
      </span>
    );
  }

  return (
    <div className="flex flex-col">
      <span className={`${config.className} px-2 py-1 rounded-full text-xs flex items-center space-x-1 font-medium`}>
        <span className={status === '002' ? 'animate-spin' : ''}>{config.icon}</span>
        <span>{config.label}</span>
      </span>
      {showDescription && (
        <span className="text-xs text-gray-500 mt-1">
          {config.description}
        </span>
      )}
    </div>
  );
};

export default StatusBadge;
