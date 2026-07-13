# Hermes Agent Kanban — 完整学习笔记

> 文档版本：基于 Hermes Agent 官方文档 + CLI `hermes kanban --help`
> 日期：2026-07-12

---

## 一、什么是 Kanban？

Hermes Agent 的 **Kanban** 是一个**持久的 SQLite 任务看板**，用于**跨多个 Hermes 配置文件（Profile）的协作**。它不仅仅是待办清单，而是一个完整的**多 Agent 工作队列系统**。

### 核心特点

- ✅ **持久化** — 任务保存在 SQLite 数据库中，重启不丢失
- ✅ **跨 Profile** — 不同的 Hermes 实例（不同 Profile）可以共享同一个看板
- ✅ **原子操作** — 任务领取是原子性的，不会出现多个 Agent 抢同一个任务
- ✅ **依赖管理** — 任务可以有父/子依赖（DAG 图）
- ✅ **自动调度** — Dispatcher 自动分配任务、回收过期 claim、触发执行
- ✅ **工作隔离** — 每个任务在工作目录上有独立的 workspace
- ✅ **事件流** — 所有操作都有事件日志，可追溯

---

## 二、Kanban 架构概览

```
┌─────────────────────────────────────────────────────┐
│                     Kanban Board                      │
│                  (SQLite: kanban.db)                   │
│                                                       │
│  ┌──────┐  ┌──────┐  ┌──────┐  ┌──────┐  ┌──────┐  │
│  │ triage │→│  todo  │→│ ready │→│in_progress│→│ done  │  │
│  └──────┘  └──────┘  └──────┘  └──────┘  └──────┘  │
│               ↑                         ↓            │
│            ┌──────┐                  ┌──────┐        │
│            │blocked│                  │archived│      │
│            └──────┘                  └──────┘        │
└─────────────────────────────────────────────────────┘
         ↑                          ↑
    ┌─────────┐               ┌─────────┐
    │Profile A │               │Profile B │
    │(Orchestrator)│           │(Worker)   │
    └─────────┘               └─────────┘
```

### 任务生命周期

```
triage → todo → ready → in_progress → done → archived
                      ↓
                  blocked/scheduled
                      ↓
                  ready (unblock)
```

| 状态 | 含义 |
|------|------|
| `triage` | 新创建的未分类任务，待 `specify` 转为具体规格 |
| `todo` | 已明确规格但还没准备好执行（可能等前置依赖） |
| `ready` | **可以领取执行的任务**（依赖已满足） |
| `in_progress` | 已被某个 Agent 通过 `claim` 领取，正在执行 |
| `blocked` | 被阻塞（外部依赖），手动 `unblock` 可恢复 |
| `scheduled` | 定时任务，到达预定时间后自动变为 `ready` |
| `done` | 已完成 |
| `archived` | 归档，可被 GC 清理 |

---

## 三、Kanban 的核心能力

### 3.1 看板管理

```bash
# 创建/初始化看板
hermes kanban init                    # 创建 kanban.db（幂等操作）
hermes kanban boards list             # 列出所有看板
hermes kanban boards create <slug>    # 创建新看板
hermes kanban boards switch <slug>    # 切换到指定看板
hermes kanban boards show             # 显示当前看板信息
```

### 3.2 任务操作

```bash
# 创建任务
hermes kanban create --title "任务标题" \
  --body "详细描述\n支持多行" \
  --assignee "profile-a"            # 指定执行者
  --depends-on "task-id"            # 指定依赖
  --tags "backend,api"              # 标签
  --board production                 # 指定看板

# 查看任务
hermes kanban list                   # 列出所有任务（可过滤 --status）
hermes kanban show <id>              # 查看任务详情 + 评论 + 事件
hermes kanban tail <id>              # 实时追踪任务事件流

# 任务状态管理
hermes kanban claim <id>             # 领取一个 ready 任务
hermes kanban complete <id>          # 标记完成
hermes kanban block <id>             # 标记阻塞
hermes kanban unblock <id>           # 解除阻塞
hermes kanban schedule <id> --at "2026-07-15 09:00"  # 定时任务
hermes kanban promote <id>           # 手动提升到 ready（恢复路径）
hermes kanban archive <id>           # 归档已完成任务
hermes kanban reclaim <id>           # 释放 worker 的 claim
hermes kanban reassign <id> --to "profile-c"  # 重新分配
```

### 3.3 任务依赖（DAG）

```bash
# 任务 A 依赖任务 B 完成
hermes kanban link <parent_id> <child_id>    # A ← B（B完成后A才能ready）
hermes kanban unlink <parent_id> <child_id>  # 取消依赖

# 只有所有父任务完成后，子任务才会变为 ready 状态
```

### 3.4 评论与上下文

```bash
hermes kanban comment <id> --msg "这是评论内容"
hermes kanban context <id>      # 打印 worker 看到的完整上下文
hermes kanban log <id>          # 查看 worker 执行日志
hermes kanban runs <id>         # 查看任务执行历史（每次尝试）
hermes kanban heartbeat <id>    # 发送心跳信号（worker 存活证明）
```

### 3.5 高级功能

```bash
# 分解任务（AI 辅助）
hermes kanban specify <id>      # 将 triage 任务转为具体规格（用 LLM 自动补全）
hermes kanban decompose <id>    # 将 triage 任务分解为子任务 DAG

# 统计
hermes kanban stats             # 按状态/执行者统计 + 最旧就绪任务年龄
hermes kanban assignees         # 列出所有已知 Profile + 任务数

# 通知
hermes kanban notify-subscribe <id> --source "telegram:1234"  # 订阅任务事件通知
hermes kanban notify-list       # 列出通知订阅
hermes kanban notify-unsubscribe <id>  # 取消订阅

# 诊断与维护
hermes kanban diagnostics       # 当前看板诊断
hermes kanban gc                # 垃圾回收（归档工作区、旧事件、旧日志）
```

### 3.6 Swarm 模式（并行工作流）

```bash
# 一键创建并行 worker → verifier → synthesizer 工作流
hermes kanban swarm --name "分析数据" \
  --workers "profile-a,profile-b,profile-c" \
  --verifier "profile-d" \
  --synthesizer "profile-e" \
  --prompt "分析这份数据并生成报告"
```

---

## 四、如何用 Kanban 实现多 Agent 协作

### 4.1 核心概念

| 角色 | 说明 | 配置 |
|------|------|------|
| **Orchestrator（调度器）** | 创建任务、管理看板、监控进度 | Profile 启用完整 `kanban` toolsets |
| **Worker（工作者）** | 领取任务、执行、标记完成 | 通过 Dispatcher 自动启动，受限 toolsets |
| **Dispatcher（分派器）** | 自动回收 stale claim、promote ready任务、spawn worker | 默认在 Gateway 中运行 |

### 4.2 多 Agent 协作流程

```
Step 1: Orchestrator 创建看板并添加任务
  └── hermes kanban init
  └── hermes kanban create --title "开发用户系统" ...

Step 2: 任务被分解为子任务 DAG
  └── hermes kanban decompose <triage-id>
  └── 或手动创建任务并 link 依赖关系

Step 3: Dispatcher 定期扫描看板
  └── 回收过期的 claim（超时未完成的任务）
  └── 将已满足依赖的任务 promote 到 ready
  └── 为 ready 任务 spawn 对应的 Worker Profile

Step 4: Worker 领取并执行任务
  └── Worker 看到 task_id + context
  └── 拥有受限的 kanban_* 工具集
  └── 完成任务后标记 complete

Step 5: Orchestrator 检查进度
  └── hermes kanban list
  └── hermes kanban stats
  └── hermes kanban log <id>
```

### 4.3 Worker 看到的工具集

当一个 Profile 被 Dispatcher spawn 执行任务时，它能看到的工具是受限的：

```
kanban_show       — 查看任务详情和上下文
kanban_complete   — 标记任务完成
kanban_block      — 标记任务阻塞
kanban_heartbeat  — 发送心跳（存活信号）
kanban_comment    — 添加评论
kanban_create     — 创建子任务
kanban_link       — 添加依赖
```

> Orchestrator Profile 如果启用了完整 `kanban` toolsets，还能看到：
> `kanban_list`, `kanban_unblock` — 用于看板路由和管理

### 4.4 实战示例

#### 场景：开发一个 Web 应用

```bash
# 1. Orchestrator 创建看板
hermes kanban boards create "web-app"
hermes kanban boards switch "web-app"

# 2. 创建顶层任务
hermes kanban create --title "设计数据库 Schema" --assignee "profile-backend"
hermes kanban create --title "实现用户认证 API" --assignee "profile-backend"
hermes kanban create --title "构建登录页面" --assignee "profile-frontend"
hermes kanban create --title "集成测试" --assignee "profile-qa"

# 3. 建立依赖关系
hermes kanban link "设计数据库 Schema" "实现用户认证 API"
hermes kanban link "实现用户认证 API" "构建登录页面"
hermes kanban link "设计数据库 Schema" "集成测试"
hermes kanban link "实现用户认证 API" "集成测试"
hermes kanban link "构建登录页面" "集成测试"

# 4. 查看任务状态
hermes kanban stats
# ready: 1  todo: 3  in_progress: 0

# 5. 启动 Dispatcher（Gateway 中自动运行）
# 它会自动找到 ready 的任务并 spawn Worker

# 6. 查看进度
hermes kanban list
hermes kanban tail "实现用户认证 API"

# 7. 检查结果
hermes kanban log "实现用户认证 API"
```

### 4.5 Swarm 模式（高级并行）

Swarm 模式是 Kanban 的一个特殊工作流，用于**并行处理 → 验证 → 综合**：

```
     ┌──────────┐
     │ Worker A  │─┐
     └──────────┘ │  ┌──────────┐  ┌─────────────┐
     ┌──────────┐ ├─▶│ Verifier  │─▶│ Synthesizer │─▶ Done
     │ Worker B  │─┤  └──────────┘  └─────────────┘
     └──────────┘ │
     ┌──────────┐ │
     │ Worker C  │─┘
     └──────────┘
```

使用方式：
```bash
hermes kanban swarm \
  --name "并行分析报告" \
  --workers "agent-alpha,agent-beta,agent-gamma" \
  --verifier "agent-delta" \
  --synthesizer "agent-omega" \
  --prompt "分析这份年度数据，找出增长趋势、风险点和建议"
```

---

## 五、配置与最佳实践

### 5.1 配置项

在 `~/.hermes/config.yaml` 中：

```yaml
kanban:
  dispatch_in_gateway: true   # Dispatcher 在 Gateway 中运行（推荐）
  failure_limit: 2            # 任务连续失败 N 次后自动 block
  heartbeat_timeout: 300      # Worker 心跳超时（秒），超时自动 reclaim
```

### 5.2 最佳实践

1. **一个看板一个项目** — 不要把所有任务塞到一个看板
2. **先分解再执行** — 对于复杂任务，先用 `decompose` 智能分解
3. **Worker Profile 使用受限工具集** — 防止 Worker 做超出任务范围的事
4. **合理设置依赖** — Task DAG 确保执行顺序正确
5. **定期 GC** — 清理已归档的任务空间
6. **使用 heartbeat** — 长时间运行的任务定期发心跳防止被回收
7. **Swarm 适合并行探索型任务** — 数据探索、方案调研、多角度分析

### 5.3 与 delegate_task 的区别

| 特性 | Kanban | delegate_task |
|------|--------|---------------|
| **持久性** | ✅ 持久化，重启不丢 | ❌ 父进程中断即取消 |
| **跨 Profile** | ✅ 不同配置文件的多个实例 | ❌ 同一进程内 |
| **执行时间** | ✅ 小时/天级别 | ❌ 分钟级别 |
| **依赖管理** | ✅ DAG 依赖图 | ❌ 无 |
| **自动重试** | ✅ Dispatcher 自动 | ❌ 需手动 retry |
| **适用场景** | 大型项目分阶段开发 | 快速并行子任务 |

---

## 六、任务状态转换总图

```
create → triage
                 │
          ┌──────┴──────┐
          │  specify     │  decompose
          ▼              ▼
        todo ────→ 子任务 DAG
          │
    ┌─────┘──────────┐──────┐
    │  依赖满足后     │      │
    ▼                │      │
  ready              │      │
    │                │      │
 ┌──┴───────┐        │      │
 │  claim   │        │      │
 ▼          │        │      │
in_progress │        │      │
 │          │        │      │
 ├─complete─┤        │      │
 ├─block────┤────────┘      │
 ├─schedule─┤              │
 │          │               │
 ▼          ▼               ▼
done     blocked        scheduled
 │          │               │
 │     unblock         时间到
 │          │               │
 │          ▼               ▼
 │        ready           ready
 │
 ├→ archive → GC
```

---

## 七、快速参考命令表

| 你想做什么 | 命令 |
|-----------|------|
| 创建看板 | `hermes kanban boards create project-x` |
| 切换到看板 | `hermes kanban boards switch project-x` |
| 创建任务 | `hermes kanban create --title "..." --body "..."` |
| 列出任务 | `hermes kanban ls` |
| 查看任务 | `hermes kanban show <id>` |
| 分解任务 | `hermes kanban decompose <id>` |
| 领取任务 | `hermes kanban claim <id>` |
| 完成任务 | `hermes kanban complete <id>` |
| 阻塞/解阻塞 | `hermes kanban block\|unblock <id>` |
| 添加依赖 | `hermes kanban link <parent> <child>` |
| 设置定时 | `hermes kanban schedule <id> --at "2026-07-15 09:00"` |
| 查看统计 | `hermes kanban stats` |
| 查看日志 | `hermes kanban log <id>` |
| 实时追踪 | `hermes kanban tail <id>` |
| 创建 Swarm | `hermes kanban swarm --name "..." --workers "a,b,c" ...` |
| 垃圾回收 | `hermes kanban gc` |
| 诊断问题 | `hermes kanban diagnostics` |
