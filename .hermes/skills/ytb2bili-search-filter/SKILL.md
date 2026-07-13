---
name: ytb2bili-search-filter
description: YouTube 搜索双层安全过滤规则 — 用于 ytb2bili-go 自动搜索时的关键词过滤、内容黑名单、后置校验
category: software-development
---

# ytb2bili-go 搜索过滤规则

调用 `search`、`auto` 相关命令时自动加载此技能，确保搜索结果只包含纯技术/智能体/开发类视频。

## 一、搜索 Query 构建规则

### 1.1 正向关键词模板

每条搜索 query 格式：

```
[核心技术词] AND (tutorial OR deep dive OR architecture OR development OR coding) -politics -news -war -election -documentary
```

### 1.2 负向屏蔽后缀（自动追加）

每次搜索统一追加：

```
-politics -political -election -government -war -military -news -documentary -protest -conflict -adult -nsfw -violence -crime -religion -debate -history -documentary
```

## 二、标准搜索关键词库（四大类）

### 2.1 AI智能体核心框架

| ID | 关键词 |
|----|--------|
| ai-1 | Hermes Agent OS Obsidian memory workflow tutorial |
| ai-2 | Multi-agent orchestration kanban task system |
| ai-3 | AI Agent DAG workflow editor React Flow |
| ai-4 | AutoGen CrewAI LangGraph agent collaboration |
| ai-5 | Model Context Protocol MCP agent skill development |
| ai-6 | Local open source AI agent deployment |
| ai-7 | Agent memory layer long term retrieval Obsidian |
| ai-8 | AI video generation agent pipeline workflow |
| ai-9 | build autonomous AI agent from scratch |
| ai-10 | multi-agent task decomposition system design |
| ai-11 | agent kanban task scheduling open source |
| ai-12 | AI agent persistent memory implementation |

### 2.2 前端/全栈IT开发

| ID | 关键词 |
|----|--------|
| web-1 | Next.js React Flow workflow editor App Router |
| web-2 | xyflow drag drop agent dashboard development |
| web-3 | FastAPI backend agent API design tutorial |
| web-4 | TypeScript AI workflow frontend architecture |
| web-5 | Docker Hermes agent deployment guide |
| web-6 | Full stack AI media generation pipeline |
| web-7 | Tailwind CSS agent UI design tutorial |
| web-8 | Python async agent backend architecture |

### 2.3 底层大模型/前沿技术

| ID | 关键词 |
|----|--------|
| llm-1 | Grok agent coding benchmark comparison |
| llm-2 | GPT agent system architecture deep dive |
| llm-3 | Local LLM agent Ollama integration |
| llm-4 | RAG multi-agent knowledge base implementation |
| llm-5 | Open source agent engine performance optimization |
| llm-6 | fine tuning LLM for tool calling |
| llm-7 | open source LLM deployment tutorial |
| llm-8 | AI model inference optimization |

### 2.4 媒体自动化智能体

| ID | 关键词 |
|----|--------|
| media-1 | AI video agent workflow intermediate asset management |
| media-2 | Multi-step media generation agent pipeline |
| media-3 | AI script storyboard voice synthesis agent |
| media-4 | Automated video production agent orchestration |
| media-5 | ComfyUI agent workflow automation |
| media-6 | AI image generation pipeline API tutorial |

## 三、后置内容过滤规则

### 3.1 时长过滤

| 规则 | 值 |
|------|-----|
| 最小时长 | 120 秒（2分钟，排除短视频） |
| 最长时长 | 3600 秒（60分钟，排除过长纪录片/课程） |
| 默认范围 | medium（4-20分钟）或 long（20-60分钟） |

### 3.2 标题/简介黑名单

如果标题或简介命中以下任一关键词，丢弃该视频：

```
# 政治/敏感
election, president, policy debate, country conflict, government,
protest, war, military, political, politics, documentary,

# 色情/暴力
nsfw, adult, gore, murder, violent, 18+

# 无关杂项
celebrity, movie review, sports, podcast debate, daily vlog,
reaction video, prank, challenge, mukbang, gaming, gameplay,
music video, trailer, review movie

# 低质内容
tiktok compilation, shorts compilation, funny moments,
best moments, highlights, vine, meme compilation
```

### 3.3 排序优先级

```
view_count（按观看数排序） > 过滤 > 取前 N 个
```

## 四、auto 命令调用时的默认过滤

当用户调用 `ytb2bili auto <keyword>` 时：

1. 如果 keyword 是短 ID（如 `ai-3`、`web-1`），展开为完整关键词
2. 追加负向屏蔽后缀
3. 指定 `sort=view_count`、`date=this_week` 过滤
4. 搜索结果经过 `FilterByContent(video)` 校验
5. 最后经过 view_count >= 1000、duration 120-3600s 过滤
