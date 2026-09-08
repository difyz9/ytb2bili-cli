# 自主调度（auto / daemon / queue）

## 自主批量搬运 auto

自动搜索多关键词 → 多维评分筛选 → 全局去重（跳过已提交历史）→ 接入队列（默认）或直接处理（`--submit`）。

```bash
ytb auto "AI tutorial" "machine learning"        # 默认 popular 评分，入队
ytb auto --scorer balanced --min-views 1000 "python"   # 均衡评分 + 播放量门槛
ytb auto --dry-run --scorer fresh "flutter"      # 只看评分结果，不写数据
ytb auto --duration long "golang backend"        # 时长档位过滤
ytb auto --max-duration 40 --max-videos 5 --submit --skip-translate "music production"
ytb auto --upload-date this_week "devops"
```

### 评分策略

| 策略 | 说明 |
|------|------|
| `popular`（默认） | 播放量 × 0.7 + 时效 × 0.3 |
| `fresh` | 时效 × 0.8 + 播放量 × 0.2 |
| `balanced` | 播放/时效/时长均衡 |
| `nowcast` | ytsubs 式频道基线对比（播放 vs 频道常态，捕捉正在起势的视频，需先 `channel baseline`/`rank` 生成基线缓存） |

常用 flag：`--dry-run`（仅搜索）、`--scorer`、`--min-views`、`--max-videos`（默认 3）、`--max-duration`、`--duration`、`--upload-date`、`--submit`（入队后立即处理）、`--skip-translate`。

**工作模式**：无 `--submit` 时只搜索评分入队，由 `queue work` 或 daemon 消费；`--submit` 入队后立即处理；`--dry-run` 仅展示评分结果，不写入任何数据。

## 常驻守护进程 daemon

`daemon` 取代旧 `batch_loop.sh`：搜索 → 评分 → 去重 → 入队 → 串行处理，无限循环。关键词与调度参数全部来自 `config.yaml`（`search:` / `daemon:` 段），**改关键词 = 改 yaml 后重启服务**，无需改任何脚本。

```bash
ytb daemon                     # 前台无限循环（生产由 systemd 托管，勿重复手动启动）
ytb daemon --once              # 只跑一批后退出（等价 --max-batches 1）
ytb daemon --max-batches 10 --interval 30
ytb daemon --keywords "AI tutorial" --dry-run     # 只搜索评分不入队
ytb daemon status              # 查看心跳（状态/批次/当前任务/队列统计）
ytb daemon check-heartbeat     # 心跳新鲜度检查（>15min 未更新则告警并 exit 1，cron 用）
```

其余可覆盖 flag：`--scorer`、`--upload-date`、`--min-views`、`--max-videos`、`--max-duration`、`--skip-translate`、`--consume-per-batch`。

### 内置可靠性

- 失败任务自动重试（`daemon.max_retries`，默认 3 次）后停止，不再无限重试堵队列；
- 步骤级超时自动 kill 重试（默认 download/transcribe/translate/audio-sync 30min、tts 60min 等，可配 `daemon.step_timeout_sec`）；
- 心跳文件 `data/daemon/heartbeat.json` 每 30s 刷新，供外部监控；
- 任务重试达上限或心跳过期 → 飞书告警（`daemon.alert_webhook`）；
- 收到 SIGTERM/SIGINT 等当前任务完成再退出（优雅重启）。

### systemd 部署

```ini
# ~/.config/systemd/user/ytb-batch-loop.service
[Unit]
Description=ytb2bili batch loop
[Service]
ExecStart=/home/USER/.local/bin/ytb daemon
Restart=always
[Install]
WantedBy=default.target
```

```bash
systemctl --user daemon-reload
systemctl --user enable --now ytb-batch-loop
systemctl --user restart ytb-batch-loop   # 改关键词/配置后重启
journalctl --user -u ytb-batch-loop -f    # 实时日志
```

> 已注册别名 `ytb.service`（软链到同名单元），因此 `systemctl --user {start,stop,restart,status} ytb` 与长名效果完全一致。
> **不要手动再起一个 `ytb daemon`**（会和 systemd 服务抢队列）；单次手动搬运请用 `queue add` 或前台 `submit`。

## 作业队列与任务管理

队列是文件状态机（`data/queue/`），daemon / `queue work` 从队首串行消费。

```bash
ytb queue add "<YouTube URL>"     # 入队
ytb queue status                  # 统计：排队中/处理中/已完成/失败
ytb queue list [--json]           # 明细（含失败原因）
ytb queue remove <videoID>        # 移除单条
ytb queue clear                   # 清空队列
ytb queue retry-failed            # 将失败任务重新排队（先修复根因）
ytb queue work --once             # 手动消费一个后退出
ytb queue work                    # 持续消费（Ctrl+C 停止）
ytb queue audit [--recent N]      # 失败分类统计审计（来自 data/audit/events.jsonl）
```

任务管理（幂等续跑，从失败步骤接着做）：

```bash
ytb task list [--json]            # 任务列表（显示真实步骤进度 [n/7]）
ytb task show <task_id>           # 单任务详情（各步骤状态/错误定位）
ytb task retry <task_id>          # 幂等重试（从失败步骤续跑）
ytb task retry --dry-run <task_id>  # 重试但不投稿
```
