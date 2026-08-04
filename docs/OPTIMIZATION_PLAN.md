# ytb2bili-cli 优化建议：智能频道搬运（参考 ytsubs）

> 分析日期：2026-08-04
> 参考项目：https://github.com/shayne/ytsubs（YouTube nowcast 排名算法）
> 目标：从"搬运频道所有视频"升级为"用算法自动挑选高价值视频搬运"

---

## 一、现状分析

### 已有的能力（ytb2bili-cli）

| 能力 | 状态 | 说明 |
|------|------|------|
| 频道订阅 | ✅ | `channel add/list/remove`（RSS 方式） |
| 频道同步 | ✅ | `channel sync --lookback N --queue` 自动入队 |
| 定时检测 | ✅ | `yt-oauth watch`（OAuth 订阅 + 每日检测） |
| 评分器 | ✅ | popular / fresh / balanced / **nowcast** |
| nowcast 雏形 | ⚠️ 简化版 | `ScoreVideosNowcast()` 已实现 4 分量评分 |
| 频道基线 | ⚠️ 粗糙 | `channel rank` 生成 `channel_scores.json`，数据源仅 RSS 播放量 |
| 完整流水线 | ✅ | 下载→转录→翻译→TTS→音画同步→投稿→字幕 |

### 与 ytsubs 的差距（核心）

| 维度 | ytsubs（完整） | ytb2bili-cli（当前） |
|------|---------------|---------------------|
| **频道基线** | 访问频道页抓最近 30 视频，**去掉最高/最低 3 个**取平均（抗异常） | RSS 简单播放量，无异常剔除 |
| **年龄曲线** | 分段曲线：0-8h 线性爬升 → 8-48h 缓升 → 48h+ 平台（0.95） | 无（默认 ageHours=24 常数） |
| **Velocity 斜率** | 分段期望斜率（0.075 → 0.00875 → 0.001） | 简化：基线/48 常数 |
| **置信度** | 解析置信度 × 基线新鲜度 → 0.75-1.05 乘数 | 无 |
| **Early breakout** | 新视频 + nowcast 强 + velocity 强 → 额外 +0.12 | 无 |
| **订阅采集** | 登录 YouTube 抓 `/feed/subscriptions` DOM | OAuth API / RSS |
| **数据留存** | SQLite 存历史观测（可追溯） | JSON 无历史 |

---

## 二、优化建议（按优先级排序）

### 🔴 P0：完善 nowcast 算法（对标 ytsubs）

**文件**：`internal/search/search.go`（`nowcastComponents`）

1. **引入年龄曲线 `_age_curve_fraction_48h`**
   ```go
   func ageCurveFraction48h(ageHours float64) float64 {
       switch {
       case ageHours <= 0:  return 0.03
       case ageHours <= 8:  return max(0.03, 0.6*(ageHours/8.0))
       case ageHours < 48:  return 0.6 + 0.35*((ageHours-8.0)/40.0)
       default:             return 0.95
       }
   }
   ```
   → 现在：新视频 2 小时就该有 15% 的基线播放；而不是简单按 24h 平均。

2. **引入期望斜率曲线**
   ```go
   func expectedSlope(ageHours float64) float64 {
       switch {
       case ageHours <= 8:  return 0.075
       case ageHours < 48:  return 0.00875
       default:             return 0.001
       }
   }
   ```
   → velocity = 实际播放速率 / 期望速率，更精确捕捉"起势"。

3. **加入置信度乘数（0.75~1.05）**
   - 基线新鲜度：`baseline_updated_at` 距今 <24h → 1.0；>7 天 → 0.75
   - 数据完整性：缺时长/缺频道 → 降权

4. **加入 Early Breakout Boost（+0.12）**
   - 条件：`ageHours < 24 && nowcast > 1.2 && velocity > 1.2`
   - 公式：`0.03 * ln(1 + nowcast*velocity)`，封顶 0.12
   - 效果：捕捉"刚发布就爆"的潜力视频，适合快速搬运

5. **统一权重对齐 ytsubs**
   ```
   当前: nowcast 0.55 / velocity 0.25 / views 0.15 / duration 0.05
   ytsubs: nowcast 0.55 / velocity 0.20 / reach 0.15 / duration 0.05（+boost）
   ```
   → 把 `viewScore`（绝对播放量）换成 `subscriberReach`（播放/粉丝数，对数饱和），
     小频道高转化视频也能浮上来。

### 🟡 P1：频道基线采集升级（`channel rank`）

**文件**：`internal/cmd/pipeline.go` + `internal/channel/`

1. **基线 = 最近 30 视频去掉最高/最低 3 个的平均**
   - 当前 RSS 数据容易被单条爆款污染
   - 建议：新增 `ytb channel baseline <channel_id>` 命令，用 yt-dlp 抓频道页
     最近 30 视频播放量，**trimmed mean** 计算基线，写入 `channel_scores.json`

2. **基线按 48h 归一**
   - 记录 `baseline_48h`（48 小时预期播放量）而非原始平均
   - 存储 `baseline_updated_at` 时间戳供置信度计算

3. **定时刷新**
   - `yt-oauth watch` 每日检测时顺带刷新频道基线（可配 `--refresh-baseline`）

### 🟡 P2：RoboNuggets 类频道接入示例

**场景**：用户给 `https://www.youtube.com/@RoboNuggets`，期望自动挑选高价值视频。

```bash
# 1. 添加频道（解析 @handle）
ytb channel add --handle RoboNuggets

# 2. 生成基线（抓最近 30 视频算 trimmed mean）
ytb channel baseline RoboNuggets

# 3. 智能评分同步（nowcast 排序，只入队 top N）
ytb channel sync --scorer nowcast --top 5 --queue

# 4. 或全自动：每日检测 → nowcast 评分 → top N 入队 → 流水线搬运
ytb yt-oauth watch --scorer nowcast --top 5
```

**实现要点**：
- `channel add --handle`：解析 `@handle` → channel_id（调 YouTube resolve API 或 yt-dlp）
- `channel sync --scorer nowcast`：sync 时对发现的视频跑 `ScoreVideosNowcast`，只入队前 N
- 复用现有 `loadChannelBaselines()` 缓存

### 🟢 P3：订阅采集增强

1. **支持从 YouTube 已登录会话导入订阅**（现有 `yt-oauth sync` 已做 OAuth 方式）
2. **批量添加**：`ytb channel add --bulk channels.txt`（每行一个 URL/@handle）
3. **频道健康度**：`channel rank` 增加"活跃度"（近 30 天发布频率）和"基线稳定性"指标，
   过滤掉停更/数据不可靠的频道

### 🟢 P4：数据留存与复盘

1. **观测历史**：每次 `channel sync` 记录视频播放量快照（JSON 追加），
   支持"搬运后 N 天播放量"复盘
2. **评分透明度**：`--dry-run` 输出每个视频的评分明细（nowcast/velocity/reach/duration/boost），
   类似 ytsubs 的 `performance_details`
3. **搬运阈值**：`--min-score` 过滤低分视频，避免浪费流水线资源

---

## 三、预期收益

| 指标 | 现在 | 优化后 |
|------|------|--------|
| 搬运质量 | 频道全部视频（含扑街） | 只搬 nowcast 高分（潜力股） |
| 资源利用率 | 100% 视频走完整流水线 | 仅 ~30% 高价值视频 |
| 搬运时效 | 发布后手动/定时全量 | 发布几小时内捕捉 breakout |
| B站限流风险 | 全量搬运易触发 | 精选 + 20min 间隔双保险 |

---

## 四、实施路线

```
Phase 1（0.5 天）: nowcast 算法完善（年龄曲线/斜率/置信度/boost）
Phase 2（0.5 天）: channel baseline 命令（trimmed mean 基线）
Phase 3（0.5 天）: channel sync --scorer nowcast --top N 集成
Phase 4（1 天）:   RoboNuggets 示例全链路验证 + 数据留存
```

---

## 五、参考资源

- ytsubs 算法源码：`generate_feed.py`（评分）、`scrape_channel_stats.py`（基线）、`scrape_videos.py`（订阅采集）
- 本地克隆：`/tmp/ytsubs/`
- 现有实现：`internal/search/search.go` L915-999（`ScoreVideosNowcast`）
- 现有基线：`internal/cmd/pipeline.go` L1328-1346（`loadChannelBaselines`）
