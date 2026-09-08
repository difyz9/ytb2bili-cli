import * as path from 'node:path';
import { defineConfig } from '@rspress/core';

export default defineConfig({
  root: path.join(__dirname, 'docs'),
  lang: 'zh',
  title: 'ytb2bili-go',
  description: 'YouTube → Bilibili 视频搬运流水线工具（搜索、下载、转录、翻译、配音、投稿、字幕上传）',
  themeConfig: {
    socialLinks: [
      {
        icon: 'github',
        mode: 'link',
        content: 'https://github.com/zolagz/ytb2bili-go',
      },
    ],
  },
});
