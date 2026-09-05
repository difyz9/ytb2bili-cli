# 当前项目代码评审与优化方案

> 评审日期：2026-09-04  
> 评审范围：Go CLI、daemon、文件队列、任务存储、流水线、HTTP 服务及关键外部服务调用  
> 评审目标：降低重复投稿、任务丢失、并发状态错乱和长任务失控的生产风险

## 一、结论摘要

项目已经具备较完整的 YouTube → Bilibili 流水线，关键包的现有测试通过，且代码中已经使用临时文件 + `rename`、任务状态字段、步骤超时和 daemon 心跳等机制。当前主要风险集中在“多个进程同时操作文件状态”以及“外部副作用成功后本地落盘失败”这两个边界，而不是普通单元逻辑。

建议优先完成以下三项：

1. 将队列锁从 `queue.json` 移到不会被替换的独立锁文件，并增加跨进程并发测试。
2. 为 daemon 增加单实例锁；将崩溃恢复改为基于租约过期和 worker 身份的回收。
3. 建立投稿后的幂等收敛流程，避免 B 站投稿成功但历史写入失败时重复投稿。

## 二、已确认问题

### P0：队列文件锁会因原子替换失效

位置：[internal/queue/queue.go](../internal/queue/queue.go#L89)、[internal/queue/queue.go](../internal/queue/queue.go#L127)

`lock()` 锁定的是当前 `queue.json` 的 inode，但 `writeAll()` 使用临时文件 `rename` 替换该 inode。第一个进程仍持有旧文件锁时，第二个进程可以打开新的 `queue.json` 并成功加锁。两个 CLI 或 daemon 可能基于不同快照写回，导致新增任务丢失、状态回退或重复认领。

**优化方案**

- 使用固定路径 `queue.lock` 执行 `flock`，业务文件仍可通过临时文件 + `rename` 原子更新。
- 所有读改写操作都必须持有该锁。
- 在测试中启动两个独立进程并发执行 `Add`、`Next`、`Complete`，验证最终任务数量和状态。
- 中长期将队列迁移到 SQLite，使用唯一索引和事务替代手工文件状态机。

### P0：daemon 恢复逻辑可能造成重复处理

位置：[internal/cli/daemon.go](../internal/cli/daemon.go#L205)、[internal/queue/queue.go](../internal/queue/queue.go#L311)

daemon 启动时无条件调用 `RequeueClaimed()`，会把所有 `claimed` 任务重置为 `queued`。如果旧实例仍在运行、systemd 重启发生重叠，或者用户误启动第二个实例，活动任务会被再次认领，可能重复下载、转录甚至投稿。

**优化方案**

- 启动前获取固定的 `daemon.lock`，获取失败时直接退出并提示已有实例运行。
- 给 claim 增加 `lease_id`、`claimed_by` 和最后心跳时间。
- 只回收确认超过租约时间的任务；不要按“daemon 是否重启”无条件回收。
- 正常退出时主动释放当前 claim，异常退出依靠租约过期恢复。

### P0：投稿成功与历史写入不是原子操作

位置：[internal/pipeline/steps.go](../internal/pipeline/steps.go#L438)

B 站投稿成功后才写任务 BVID 和历史。如果投稿成功、随后历史写入失败，流水线会返回失败，daemon 重试时可能再次投稿同一视频。历史检查也是先读后写，多个进程之间没有唯一约束。

**优化方案**

- 将投稿结果拆成明确状态：`uploading`、`uploaded_pending_record`、`completed`。
- 投稿返回 BVID 后立即持久化任务记录；历史写入失败时保留 BVID，不再重新上传。
- 重试前按 YouTube ID 查询本地任务/历史；若已存在 BVID，则进入补偿流程而不是再次调用上传接口。
- 历史存储增加跨进程锁或迁移到 SQLite，并对 `youtube_id` 建唯一索引。
- 增加“投稿成功、历史写入失败”的故障注入测试。

### P1：`daemon.max_retries` 没有控制队列实际重试次数

位置：[internal/config/config.go](../internal/config/config.go#L164)、[internal/cli/daemon.go](../internal/cli/daemon.go#L241)、[internal/queue/queue.go](../internal/queue/queue.go#L182)

配置层可以读取 `MaxRetries`，但 `queue.Add()` 始终使用 `DefaultMaxRetries`。日志显示的重试上限可能与任务实际状态机不一致。

**优化方案**

- 让 `Queue.Add` 接收 `maxRetries`，或让 `Queue` 持有明确配置。
- 对小于 1 的值统一使用默认值，并在入队时写入任务快照。
- 增加测试：配置为 1、5 时分别验证任务在对应次数后进入 `failed`。

### P1：claim 没有续租，长任务可能被重复处理

位置：[internal/queue/queue.go](../internal/queue/queue.go#L39)、[internal/queue/queue.go](../internal/queue/queue.go#L223)

固定 30 分钟超时覆盖不了长时间 TTS、上传或网络重试。原 worker 仍在运行时，另一个 worker 可能把任务回收并再次处理。

**优化方案**

- 增加 `RenewClaim(videoID, workerID)`，daemon 每 30 秒续租。
- 回收时同时校验 `claimed_by` 和租约时间。
- 将 claim 租约、步骤超时和 worker 心跳配置化，但保留合理的最大值保护。
- 测试“原 worker 仍运行但 claim 已过期”的场景，确保不会双重执行。

### P1：损坏的队列文件会被当成空队列

位置：[internal/queue/queue.go](../internal/queue/queue.go#L109)

JSON 解码失败时直接初始化空数据。磁盘异常或文件损坏会在下一次写入时覆盖原队列，造成不可逆任务丢失。

**优化方案**

- 空文件可初始化；非空文件解析失败必须返回错误。
- 写入前保留 `.bak`，或保留带版本号的最近快照。
- 启动检查失败时停止消费，并输出恢复路径和诊断信息。
- 增加空文件、截断 JSON、非法版本和备份恢复测试。

### P1：TaskStore 读改写存在丢更新，更新错误无法传递

位置：[internal/storage/storage.go](../internal/storage/storage.go#L154)

`UpdateStep()` 先读取并释放锁，再重新加锁保存。并发更新可能用旧快照覆盖新字段；读取或保存失败也不会返回给调用方。

**优化方案**

- 实现持锁的 `Update(id, func(*Task) error) error`，在同一临界区完成读、改、写。
- 让 `UpdateStep`、`SetBVID` 等更新 API 返回 `error`。
- 对任务文件使用独立锁文件；中长期与队列一起迁移到 SQLite 事务。
- 为并发更新增加竞态测试和独立进程测试。

### P1：HTTP 超时中间件可能出现并发写响应

位置：[internal/server/server.go](../internal/server/server.go#L718)

中间件在 goroutine 中执行 handler，超时后向同一个 `ResponseWriter` 写 503，但 handler 仍可能继续执行副作用或写响应。context 取消并不能强制停止 handler。

**优化方案**

- 普通 handler 直接在当前 goroutine 中执行，并让业务代码遵守 `r.Context()`。
- 对下载、投稿等长操作采用“创建任务并立即返回任务 ID”，不要让 HTTP 请求持有整个流水线。
- 需要超时响应的接口由业务层控制状态，确保只有一个响应写入者。
- 增加超时后无重复响应、无继续副作用的测试。

### P2：取消和资源生命周期处理不完整

位置：[internal/transcriber/transcriber.go](../internal/transcriber/transcriber.go#L161)、[internal/transcriber/transcriber.go](../internal/transcriber/transcriber.go#L283)、[internal/bili/bili.go](../internal/bili/bili.go#L62)

Bcut 轮询使用不可取消的 `time.Sleep`；部分请求构造错误被忽略；封面上传通过 goroutine + 超时等待实现，但超时后后台请求仍可能继续。

**优化方案**

- 用 `select` 监听 `ctx.Done()` 和定时器，保证轮询可快速取消。
- 显式处理 `json.Marshal`、`http.NewRequestWithContext` 和 HTTP 状态码错误。
- 优先使用支持 context 的 HTTP/SDK 调用；不能取消的 SDK 调用应隔离并记录结果，不要假装已经停止。
- 为取消、请求超时、子进程退出增加测试。

### P2：源码包含内置 app secret fallback

位置：[internal/auth/auth.go](../internal/auth/auth.go#L22)

即使有环境变量覆盖，内置 fallback 仍会进入源码和二进制，可能被提取和滥用。

**优化方案**

- 删除敏感凭证的源码默认值，只允许运行时配置或凭证文件注入。
- 轮换已经暴露的 secret，并检查提交历史和构建产物。
- `check` 命令明确区分缺少配置、无效凭证和权限不足。

## 三、实施路线

### Phase 1：先消除重复投稿和跨进程破坏

- 固定 `queue.lock` 和 `daemon.lock`。
- 引入 daemon 单实例保护。
- 修复投稿后的 BVID 持久化与恢复流程。
- 增加跨进程队列测试、重复投稿故障注入测试。

**完成标准**：两个 daemon 不能同时运行；并发入队不丢任务；投稿成功后任意一次本地写入失败都不会再次上传。

### Phase 2：完善任务租约和存储一致性

- claim 续租、过期回收和 worker 身份校验。
- 修复 `max_retries` 配置传递。
- TaskStore 改为持锁读改写并返回错误。
- 队列损坏时拒绝静默覆盖，并增加备份恢复。

**完成标准**：长于 30 分钟的任务不会被健康 worker 重复认领；配置重试次数与实际状态一致；损坏数据可诊断、可恢复。

### Phase 3：收敛 HTTP 和外部调用生命周期

- 移除 handler 超时 goroutine 写响应模式。
- 所有轮询和外部请求支持 context 取消。
- 长任务接口统一返回任务 ID，并通过任务状态查询结果。
- 清理源码凭证 fallback，补充 secret 扫描。

**完成标准**：请求超时后不会发生双重响应；取消信号能传递到轮询、上传和子进程；日志可定位每个外部调用。

### Phase 4：中长期存储演进与可观测性

- 将队列、任务、历史迁移到 SQLite，使用事务和唯一索引。
- 记录 `task_id`、`video_id`、`worker_id`、`bvid`、步骤耗时和重试原因。
- 增加失败分类统计：网络、认证、外部 API、资源、数据损坏。
- 心跳中增加当前租约、最近续租时间和最近错误摘要。

## 四、建议补充的测试矩阵

```text
队列：两个独立进程并发 Add / Next / Complete，不丢任务、不重复认领
队列：claim 续租、过期回收、worker 身份不匹配
队列：空文件、截断 JSON、备份恢复
任务：并发 UpdateStep / SetBVID，不覆盖字段，错误可返回
配置：max_retries = 1、5 对状态机产生不同结果
投稿：Upload 成功但 history.Add 失败，重试不再次 Upload
HTTP：handler 超时后只有一个响应写入者，context 能取消业务
外部调用：Bcut 轮询取消、HTTP 请求超时、封面上传取消
安全：无 secret 环境启动失败；源码和构建产物不含敏感默认值
```

## 五、当前验证结果与残余风险

已执行：

```bash
go test ./internal/queue ./internal/storage ./internal/pipeline ./internal/server ./internal/cli
```

结果：全部通过。

这只能证明现有单进程单元测试未发现回归，不能证明跨进程锁、崩溃恢复、外部副作用幂等和 HTTP 超时竞态正确。下一轮改动应优先补上述测试，再进行实现调整；完成后建议执行：

```bash
go test -race ./internal/queue ./internal/storage ./internal/server ./internal/pipeline
go test ./...
```

## 六、项目结构与可扩展性专项评审

### 6.1 当前结构的判断

当前目录已经完成了历史布局中的关键迁移：主入口位于 `cmd/ytb`，命令实现位于 `internal/cli`，业务能力分布在 `pipeline`、`workflow`、`queue`、`storage` 及各外部服务包中；仓库也已经有 CI 和 `make verify`。因此不建议再次进行大规模目录搬迁，下一步重点应放在职责边界和唯一状态源。

### P0：存在三套任务模型和调度路径

目前同一个视频可能同时拥有：

```text
HTTP/Feishu：server.VideoTask + taskChan
CLI daemon： queue.Video + queue.json
Pipeline：   storage.Task + tasks/*.json
```

证据：[internal/server/server.go](../internal/server/server.go#L29)、[internal/queue/queue.go](../internal/queue/queue.go#L51)、[internal/pipeline/pipeline.go](../internal/pipeline/pipeline.go#L91)。

这会造成任务 ID、状态、错误和 BVID 在不同存储中分叉。服务重启、HTTP 提交、daemon 消费和手动 `submit` 的恢复路径也不完全一致，扩展新入口时必须重复理解多套生命周期。

**建议**

- 定义唯一的应用任务模型和状态机，统一保存任务 ID、video ID、步骤状态、BVID、错误和重试信息。
- 抽取 `SubmitService`、`QueueService`、`WorkerService`，HTTP、CLI、daemon、Feishu 只负责适配输入和输出。
- 迁移期可以保留现有 JSON 文件，但必须由一个 service 负责读写；禁止新入口直接操作 `queue.json` 或 `tasks/*.json`。
- 中期迁移到 SQLite，以事务、唯一索引和租约字段统一队列、任务和历史。

### P0：`internal/cli` 仍是应用编排层

`internal/cli` 当前直接依赖约 18 个内部包。`daemon.go`、`auto.go`、`queue.go`、`submit.go` 和 `chain.go` 分别包含搜索、评分、入队、消费、重试、流水线执行和报告逻辑。这样新增 HTTP、定时任务或其他客户端时容易复制行为，随后出现 CLI 与服务端规则不一致。

**建议**

将职责分成三层：

```text
internal/cli/       Cobra 命令、参数校验、终端输出
internal/app/       submit、queue、worker、daemon 等应用用例
internal/domain/    Job、Submission、Artifact 等稳定业务模型
internal/adapters/  YouTube、Bilibili、翻译、TTS、文件/数据库实现
```

第一步只抽取 service，不要同时改包名和存储格式。等 CLI、HTTP、daemon 共用同一 service 后，再按实际依赖移动目录。

### P1：pipeline 对具体基础设施耦合较深

`pipeline` 的 registry 和步骤直接创建下载、转录、翻译、TTS、音画同步、元数据、Bilibili 及存储实现。扩展新的转录器、翻译 provider 或发布平台时，通常需要修改 pipeline，而不是添加一个 adapter。

**建议**

只抽取真正稳定的端口接口：

```go
type Downloader interface { Download(context.Context, Request) (Artifact, error) }
type Transcriber interface { Transcribe(context.Context, Artifact) (Subtitle, error) }
type Translator interface { Translate(context.Context, Subtitle) (Subtitle, error) }
type Publisher interface { Publish(context.Context, Publication) (Submission, error) }
```

具体实现和 provider 选择放到 `internal/app/bootstrap` 或 CLI 组合根；`pipeline` 只依赖接口、领域对象和 workflow，不接收完整的基础设施配置。

### P1：配置对象已经成为跨域 God Object

`internal/config/config.go` 同时承载存储、YouTube、Bilibili、LLM、翻译、TTS、daemon、Feishu、HTTP 和 Chrome 调试配置。业务包普遍接收完整的 `*config.Config`，新增一个字段会扩大隐式耦合和测试准备成本。

**建议**

保留统一加载入口，但拆出领域配置：`StorageConfig`、`YouTubeConfig`、`BilibiliConfig`、`TranslationConfig`、`TTSConfig`、`DaemonConfig`、`ServerConfig`。启动阶段将其转换为各服务的窄构造参数，业务包不再持有完整根配置。

另需修正依赖方向：`config` 不应依赖 `queue` 获取默认重试次数；默认值应属于配置层，队列只接收最终配置。

### P1：文件存储边界过多，完成状态缺少统一事务

任务状态分散在 `queue/queue.json`、`storage/tasks`、history、subtitle store、下载产物和 daemon heartbeat 中。视频文件继续放文件系统是合理的，但任务、投稿和步骤状态应有一个明确的 manifest/数据库边界。

**建议**

- 短期统一 repository 接口和状态更新入口，文件实现放在 `internal/persistence/file`。
- 中期使用 SQLite 表：`jobs`、`job_steps`、`submissions`、`subtitle_tracks`，对 `video_id` 建唯一约束。
- 文件产物保存路径和校验信息，不让业务状态依赖“某个文件是否存在”的隐式判断。

### P2：extension 与 Go 服务缺少显式契约

`extension/` 是独立的 TypeScript/WXT 项目，Go 服务和扩展目前主要依靠手写路径和 JSON 字段协作。扩展不应直接依赖 Go 源码，但 API 字段变更需要版本策略和契约测试。

**建议**

- 增加 `contracts/http/`，维护 submit request、task response 等 JSON Schema。
- 在 Go 端增加 handler 契约测试，在 TypeScript 端从 schema 生成或校验类型。
- 短期继续 monorepo，但为 extension 使用独立的 package script、CI job 和 README 边界说明；只有发布节奏或权限明显冲突时再拆库。

### P2：运行时 skills 目录应被视为插件资源

`skills/audio-video-sync/` 同时包含说明文档、Python 脚本、依赖和 agent 配置，实际是运行时资源包。若由 pipeline 直接拼接路径，目录调整会影响执行。

**建议**

- 由 `internal/resource` 统一解析资源根目录，pipeline 不直接判断 cwd。
- 为每个 skill 增加 manifest，声明入口脚本、依赖和版本。
- 目录迁移到 `assets/skills` 只有在路径解析和部署配置先稳定后进行。

### P2：测试目录分布正常，但缺少结构级测试

测试与 Go 源文件同目录符合惯例；当前核心 queue、storage、pipeline、server 有测试，但外部 adapter、CLI 命令和入口之间的集成覆盖不足。

建议分三层：

```text
单元测试：领域状态转换、解析、评分和格式化
适配器契约测试：HTTP fake server、provider fake、文件实现
集成测试：CLI / HTTP / daemon 共用 application service
```

真实凭证测试应使用 `//go:build live`，避免普通 `go test ./...` 依赖环境变量或通过 `-skip` 字符串规避。

### P2：命名和文档存在漂移

仓库目录名是 `ytb2bili-cli`，Go module 是 `github.com/zolagz/ytb2bili-go`，二进制是 `ytb`，历史文档还出现 `ytb2bili`、`internal/cmd` 等名称。`docs/repo-layout-refactor-plan.md` 的历史现状部分也不应继续作为当前结构说明。

**建议**

- 在 README 或架构文档中明确：Repository、Go module、Binary 三者的正式名称。
- 将已完成的布局迁移记录保留在历史文档，另建一份“当前架构”作为唯一事实来源。
- 文档示例统一使用 `ytb` 和 `cmd/ytb`，旧名称仅在迁移说明中出现。

## 七、推荐目标结构

```text
ytb2bili-cli/
├── cmd/ytb/main.go
├── internal/
│   ├── cli/                 # Cobra 适配层
│   ├── app/                 # submit、queue、worker、daemon 用例
│   ├── domain/              # Job、Submission、Artifact
│   ├── ports/               # 稳定外部接口
│   ├── pipeline/ workflow/  # 流程编排和步骤规划
│   ├── adapters/            # youtube、bilibili、translation、tts 等
│   ├── persistence/         # file 过渡实现、sqlite 中期实现
│   ├── server/              # HTTP presentation 层
│   ├── config/ resource/
│   └── feishu/              # 外部通知/入口 adapter
├── contracts/http/
├── extension/               # 独立 Node package
├── skills/                  # 明确为运行时资源包
├── configs/ scripts/ docs/
└── data/                    # 运行时数据，不入库
```

不建议创建公共 `pkg/`：当前是应用而非供外部 import 的库，继续使用 `internal/` 更合适；也不建议现在改 Go module 名称，收益主要是观感，迁移成本却是全量 import 变更。

## 八、结构重构实施顺序

1. **建立唯一状态服务**：先抽取 `SubmitService`、`QueueService`、`WorkerService`，让 CLI、HTTP、daemon、Feishu 复用；保留现有文件格式。
2. **收窄配置和依赖**：将完整 `*config.Config` 转换为领域构造参数，去除 `config -> queue` 反向依赖。
3. **抽取稳定端口**：为 publisher、translator、transcriber、task repository 增加最小接口，并在组合根注入具体实现。
4. **治理契约与资源**：增加 HTTP schema、live build tag、skill manifest 和资源路径测试。
5. **迁移持久化**：在结构稳定后引入 SQLite；完成数据迁移、唯一索引和回滚方案后，再删除重复 JSON 状态。
6. **最后再移动目录**：只有当 service 边界被测试固定后，才将实现归入 `app`、`domain`、`adapters`、`persistence`，避免“先搬目录、后发现职责仍然混杂”。

### 结构重构验收标准

- CLI、HTTP、daemon、Feishu 对同一视频使用同一个任务 ID 和状态源。
- 新增一个翻译 provider 或发布 adapter 不需要修改核心 pipeline 状态机。
- `internal/cli` 不直接读写队列和任务文件。
- 普通 `go test ./...` 不需要真实凭证；跨入口集成测试可使用 fake adapter。
- 从非项目根目录运行时，skills、配置和数据路径仍由显式配置解析。
