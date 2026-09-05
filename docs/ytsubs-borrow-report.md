# ytsubs 借鉴报告

> 调研对象：[shayne/ytsubs](https://github.com/shayne/ytsubs)（MIT 协议）
> 撰写日期：2026-08-04
> 目的：评估 ytsubs 的「订阅视频评分排序」设计，提取可迁移到本项目的思路。
> 对应需求：`channel rank` 频道质量评分、`auto` 视频评分、订阅搬运流水线的质量门槛。

---

## 一、项目概览

**ytsubs 是什么**：一个本地 Web 应用，把 YouTube 订阅频道的视频按「nowcast 排名算法」排序，绕过 YouTube 官方算法，生成一个自包含的 HTML 订阅源（`ytsubs_feed.html`）。

**技术栈**：Python 3.12+、SQLite、Playwright（驱动 Chrome，持久 profile 登录一次）、Preact 前端、`uv` 打包、Makefile。

**核心管线（4 部分）**：

```
1. scrape-channels（低频，如每月）→ 采集订阅数 + 每个频道的 baseline_48h（期望播放表现）
2. scrape-videos（高频，如每日）→ 采集订阅频道的新视频
3. observation 追踪 → 每次抓取记录快照，支持"当时可知道"的排名
4. 静态页生成 → 按评分输出 HTML
```

---

## 二、核心亮点与可借鉴性评估

| 亮点 | 说明 | 可借鉴性 |
|------|------|----------|
| **Nowcast 排名算法** | 4 个加权分 + 2 个修正项，见第三节 | ★★★★★ 直接可用于我们的 `auto`/`channel rank` |
| **baseline_48h 频道基线** | 每个频道记录"期望播放表现"，用于判断某条视频是否超常 | ★★★★★ 是"高质量频道"最本质的信号，比订阅数更有用 |
| **Point-in-time 观察** | 每次抓取存快照，排名反映当时可知的数据 | ★★★☆☆ 我们目前只存最新状态 |
| **双节奏抓取** | 视频每日、频道统计低频 | ★★★☆☆ 我们 `channel sync` 可配合定期刷新频道统计 |
| **持久 Chrome profile** | Playwright 登录一次、复用会话 | ★★☆☆☆ 我们已有等价物（data/chrome-profile + cookies refresh） |
| **Facet 时间窗排序** | day/week/twoweeks/month 各窗内最强 | ★★☆☆☆ 可做 `auto --window week` 之类 |

**我们不缺的部分**：抓取手段（我们有 yt-dlp + RSS + OAuth Data API，比 Playwright 刮 DOM 更稳）；输出形式（我们输出 B 站投稿元数据，不是 HTML feed）。

---

## 三、Nowcast 算法详解（核心借鉴对象）

```
核心分 = 55%·Nowcast vs Expected
       + 20%·Velocity Shock
       + 15%·Subscriber Reach
       + 5%·Duration Prior

修正：
  × Confidence multiplier（0.75–1.05）弱解析/过期基线降权
  + Early breakout boost（最多 +0.12）很新且 nowcast/velocity 双强的视频
```

各分量含义：

| 分量 | 权重 | 含义 | 对我们的价值 |
|------|------|------|--------------|
| **Nowcast vs Expected** | 55% | 当前播放 vs 按视频年龄调整后的频道期望播放（来自 baseline_48h） | 判断"这条视频是否超常发挥"——比绝对播放量更能挑出潜力视频 |
| **Velocity Shock** | 20% | 当前播放/小时 vs 该年龄的期望斜率 | 捕捉"正在起势"的视频 |
| **Subscriber Reach** | 15% | 播放/订阅数，带边际递减 | 识别"僵尸订阅"频道 |
| **Duration Prior** | 5% | 偏向历史上表现稳定的时长 | 与我们刚做的 Short 过滤互补（ytsubs 是软偏好，我们是硬过滤） |

**关键洞察**：ytsubs 用「频道基线」而非「频道规模」衡量频道质量。两个同为 10 万订阅的频道，一个常态 5 万播放、一个常态 5 千播放，后者是"僵尸订阅"，前者才是搬运目标。

---

## 四、映射到本项目的具体借鉴建议

### 1. `channel rank`：引入 baseline 概念（最高优先级）

之前讨论的 `channel rank`（基于订阅数/活跃度/真实触达）可以升级为 ytsubs 式：

```
频道质量分 = 活跃度(30%)   × 近 N 天更新数（封顶）
           + baseline 命中(30%) × 近期视频平均播放 / baseline_48h
           + 真实触达(25%)  × log(平均播放/订阅数)
           + 内容契合(15%)  × 标题/描述命中关键词
```

其中 `baseline_48h` 即每个频道过去 48h 视频的期望播放——**有 OAuth 后用 Data API `videos.list` 批量拉近期视频播放量即可计算**，与我们的 `ytoauth.FetchVideoDurations`（本次已加）同一批调用可以顺带取 `statistics.viewCount`。

### 2. `auto` 评分策略升级

现有 `internal/search/search.go` 的 `ScorerType`（popular/fresh/balanced）是静态权重。可加一种 `scorer=nowcast`：

- Nowcast vs Expected：用频道基线（来自 channel rank 缓存）判断单条视频是否超常
- Velocity Shock：对 `auto` 场景尤其实用——识别正在起势、适合抢搬的视频
- 数据来源：OAuth Data API `videos.list`（播放量 + 发布时间的斜率），与 Short 过滤共用基础设施

### 3. Point-in-time 观察（中等优先级）

`data/monitored_videos/videos.json` 目前只存最新状态。可参考 ytsubs 为每次 `channel sync` 记录观察快照（`data/observations/<date>.json`），使历史排名可复现、可做"上次比这次表现如何"的趋势判断。轻量实现：每次 sync 把各视频的 `views` 快照追加到一行 JSON 数组。

### 4. 双节奏与缓存（低优先级）

- `channel sync`（RSS，高频）保持现状
- 新增低频 `channel rank --refresh`（Data API，拉订阅数/基线），结果缓存到 `data/channel_scores.json`
- 与 `yt-oauth watch` 的定时节奏天然配合

### 5. 与本次 Short 过滤的衔接

ytsubs 的 Duration Prior 是「软偏好」，我们把 `min_duration_sec` 做成「硬门槛」——两者互补。未来可在评分里再加一档"时长与频道历史表现匹配度"的软信号，但现阶段硬门槛已够用。

---

## 五、明确不借鉴的部分

| ytsubs 做法 | 为什么不借鉴 |
|-------------|--------------|
| Playwright 刮 DOM | 我们已有更稳的 yt-dlp/RSS/OAuth Data API，且无需登录态管理 |
| 静态 HTML feed 输出 | 我们的产出是 B 站投稿，不是个人阅读 feed |
| SQLite 存储 | 项目沿用 JSON 文件 + 现有 storage/queue 结构，迁移成本大于收益；point-in-time 用 JSON 追加即可 |
| `uv`/`mise` 工具链 | 与 Go 构建体系无关 |

---

## 六、落地优先级建议

1. **P0**：`channel rank` 引入 baseline（对接本次 Data API 基建，直接回答"高质量频道"问题）
2. **P1**：`auto` 增加 `scorer=nowcast`（复用 channel rank 的基线缓存）
3. **P2**：观察快照（point-in-time）用于趋势分析
4. **P3**：双节奏抓取与缓存

> 一句话结论：ytsubs 最值得借鉴的不是抓取或存储，而是**「用频道基线衡量视频/频道质量」的评分哲学**——这恰好是「从订阅里筛高质量频道」的最优解。
