# B站账号与投稿

## 登录与账号

```bash
ytb login                          # B站扫码登录（打印二维码）
ytb login --account "账号名"       # 多账号登录（投稿时按类型路由）
ytb accounts                       # 列出已登录账号
ytb whoami                         # 当前账号信息
```

### 多账号路由

按稿件标题/标签关键词匹配投稿账号：命中某账号 `type_rule` 中任意关键词 → 该账号投稿；未匹配 → `is_default: true` 账号兜底（账号列表在 `config.yaml` 的 `accounts` 段）。

```bash
ytb publish ... --account X     # 显式指定投稿账号
```

## 投稿本地视频

```bash
ytb publish video.mp4 --title "我的视频" --tags "科技,评测" \
        --desc "简介" --tid 122 --cover cover.jpg --source "https://..." --account "某账号"
```

`publish` 别名 `upload`；标题默认取文件名，`--source` 可标注源站 URL。经 YouTube 流水线的投稿会自动返回 BVID 并记入历史。

## 审核与历史

```bash
ytb review BV1xx123                # 查看审核状态
ytb review --wait BV1xx123         # 轮询直到审核通过（最长 24h）
ytb history [--json]               # 已提交投稿历史（重复检测依据）
```

## 字幕上传

投稿后 pipeline 会**异步监听审核**（30 秒轮询，最长 24 小时），审核通过自动上传中文字幕；状态持久化在 `data/subtitles/`，重启不丢失。手动管理：

```bash
ytb subtitle upload BV1xx123 subtitle.zh-Hans.srt   # 手动上传（--lang zh/zh-Hans/en，默认 zh）
ytb subtitle upload BV1xx123 subtitle.srt --lang en
ytb subtitle status                                  # 所有视频的字幕上传状态
```
