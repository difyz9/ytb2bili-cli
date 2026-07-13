## User request

下载完成的任务（task_1783911448168867129）没有自动提交到B站。日志显示视频已下载、转录、翻译完成，但后续的投稿和字幕投稿步骤没有执行。

日志：
2026/07/12 22:57:16    API: http://:8096/api/v1/submit
2026/07/12 22:57:16    健康检查: http://:8096/health
2026/07/12 22:57:28 开始处理任务: task_1783911448168867129 - https://www.youtube.com/watch?v=alEZ-XqZfeQ
2026/07/12 22:57:41 下载完成: data/downloads/alEZ-XqZfeQ/alEZ-XqZfeQ.mp4
2026/07/12 22:58:34 转录完成: data/downloads/alEZ-XqZfeQ/subtitle.srt
2026/07/12 22:58:54 翻译完成: ./data/downloads/alEZ-XqZfeQ/subtitle.zh.srt
2026/07/12 22:58:54 任务完成: task_1783911448168867129

后续的正常流程应该是：投稿视频 - 监听审核状态 - 审核通过后投稿字幕。但投稿视频这一步没有执行，任务直接标记为任务完成了。

## Context

视频：https://www.youtube.com/watch?v=alEZ-XqZfeQ
任务 ID：task_1783911448168867129
