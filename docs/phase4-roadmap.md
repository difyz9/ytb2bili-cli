# Phase 4 路线：统一状态源 + SQLite + 可观测性

> 评审原文将本阶段定义为「中长期存储演进与可观测性」。评审自身的实施顺序（第八节）要求：
> **先在保留现有 JSON 格式的前提下统一服务/状态源，再迁移 SQLite**，避免把三套割裂模型原样搬进数据库。
> 本文档是该阶段的落地路线与里程碑验收标准。

## 现状：三套任务模型并存（割裂的根因）

| 入口 | 任务模型 | 状态载体 | 问题 |
|------|----------|----------|------|
| CLI/daemon/queue work | `queue.Video` | `queue.json`（已加固：独立锁/损坏拒写/租约/审计） | 最完整，作为主线 |
| HTTP/Feishu | `server.VideoTask` + 内存 `taskChan` | 仅内存 + `tasks/*.json` 步骤台账 | **服务重启丢在途任务**；与队列互不可见 |
| pipeline（各入口共用） | `storage.Task` | `tasks/<id>.json`（每轮随机 id） | 仅步骤观察，非任务主记录 |

影响：同一视频的提交/去重/BVID/重试在不同入口生命周期不一致；扩展新入口需重读多套逻辑。

## 目标（唯一状态源）

- 一个应用级任务状态机：`job_id + video_id(唯一) + status + step + worker + lease + bvid + error_class + retries`。
- CLI、daemon、HTTP、Feishu 只是适配器，只调用同一组服务，不再直接读写 `queue.json`/`tasks/*.json`。
- 文件产物仍在文件系统，业务状态不依赖「文件是否存在」的隐式判断。

## 里程碑与顺序（每步独立可交付、可回滚）

### M0 ✅ 可观测性地基（已完成，本轮）
- `data/audit/events.jsonl`：队列状态转移（queued/claimed/completed/failed/retry/requeued/reset）自动落事件，含 video_id/source/worker/bvid/error/error_class/retry/duration_ms。best-effort，不阻塞业务。
- 失败分类 `ClassifyError`：auth / network / local / resource / external / unknown。
- CLI `ytb queue audit [--recent N]` 输出分类统计与最近失败。
- 事件模型即未来 SQLite 表字段蓝本。
- **验收**：任务失败后能按类聚合、能回答「1600+ 失败里有多少是 auth/network/external」。

### M1 统一 QueueService/WorkerService（保留 JSON）
- 抽 `internal/app`：`SubmitService{ Submit(video) → job }`、`QueueService`（现在的 queue 能力收敛）、`WorkerService`（claim/renew/complete 循环）。
- daemon、CLI、`queue work` 改调 service；行为与现有 JSON 完全一致（影子运行期双写对比可选项）。
- 端到端测试：同一服务在不同入口的提交走同一条去重/BVID/重试路径。
- **验收**：`internal/cli` 不再直接持有队列读写细节；CLI/daemon 对同一 video 得到同一 job。

### M2 HTTP/Feishu 提交持久化（消除重启丢任务）
- server 接受任务先落盘 durable pending（与 M0 审计同构的 jsonl 或 queue.json），服务重启后重放未完成项。
- Feishu 进度卡：持久化 msg 关联，重放时可恢复回复。
- **验收**：server 处理中重启，任务不丢、不重复投稿（复用 Phase1 pending 幂等）。

### M3 SQLite 替换（Phase 4 正文）
- schema（由 M0 事件模型演化，唯一索引/事务）：
  ```sql
  jobs(id TEXT PK, video_id TEXT NOT NULL UNIQUE, source TEXT, status TEXT,
       worker TEXT, claimed_at TEXT, lease_until TEXT, retry_count INT, max_retries INT,
       bvid TEXT, error_class TEXT, error TEXT, created_at TEXT, updated_at TEXT);
  job_steps(job_id → jobs, step TEXT, status TEXT, started_at, completed_at, error);
  submissions(video_id TEXT UNIQUE, bvid TEXT, title, submitted_at);  -- 替代 history.json
  subtitle_tracks(...);
  ```
- 迁移：`--dry-run` 影子库先对现存 queue/history 全量导入并对账（数量/状态/BVID 一致才切换）；保留 JSON 为只读回滚快照 30 天。
- 接入：Queue/History/TaskStore 全部收敛为 repository 接口 + sqlite 实现，事务 + `video_id` 唯一索引把「防重」从代码约定变成数据库约束。
- **验收**：并发入队/claim 由 SQLite 事务保证；重复投稿在数据库层不可能；损坏不再可能（WAL）。

### M4 可观测性消费
- `ytb queue audit` 升级为按失败分类/时间窗/来源过滤；心跳加当前租约/最近续租/最近错误摘要；失败原因进入 `ytb debug`。

## 纪律
- 每步保留现有行为（JSON 格式不变直到 M3 双写验证）。
- 目录搬迁（`internal/app|domain|adapters|persistence`）放最后，只有 service 边界被测试固定后才动，避免「先搬目录后发现职责仍混杂」。
- 普通 `go test ./...` 不依赖真实凭证；跨入口集成用 fake adapter。
