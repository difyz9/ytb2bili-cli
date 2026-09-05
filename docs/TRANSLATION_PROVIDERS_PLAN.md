# ytb2bili-cli 多翻译服务接入计划

> 制定日期：2026-08-04
> 参考项目：https://github.com/difyz9/translation-tool（支持百度/腾讯/火山/Microsoft/Google/Ollama/自定义）
> 背景：DeepSeek LLM 批量翻译在高峰时段不稳定（"25 条返回 1 条"截断响应），需要接入
>       多个翻译服务做**主备切换**和**容灾降级**

---

## 一、现状分析

### 当前翻译架构（ytb2bili-cli）

```
SRT 字幕 → 分组(batch=15) → 单 worker 串行
         → LLM 批量翻译（DeepSeek chat/completions，JSON 格式）
         → 严格校验数量/索引 → 写 zh-Hans.srt
```

| 维度 | 现状 |
|------|------|
| Provider | 仅 DeepSeek LLM（`llm_api_key`/`llm_base_url`/`llm_model`） |
| 批量方式 | 15 条字幕一批，LLM 一次性返回 JSON |
| 校验 | 数量严格匹配（多/少都失败）+ 索引连续 + 非空 |
| 限流处理 | 降并发(1) + 降批大小(15) + 重试3次（已优化） |
| 失败影响 | 整组失败 → 整视频翻译失败 → 跳过投稿 |

### 痛点

1. **单点依赖 DeepSeek**：高峰限流时响应被截断，5 条视频连续失败
2. **LLM 批量 JSON 不稳定**：严格校验放大了 LLM 输出不稳定的影响
3. **无降级路径**：DeepSeek 挂 = 翻译全挂 = 搬运停摆

---

## 二、目标架构

### 翻译 Provider 抽象层

```
┌─────────────────────────────────────────────────────────┐
│                  translator.Provider 接口               │
├─────────────────────────────────────────────────────────┤
│  TranslateBatch(ctx, texts []string, src, dst string)   │
│     ([]string, error)                                   │
│  Name() string                                          │
│  Health() bool        // 快速健康检查（可选）            │
└─────────────────────────────────────────────────────────┘
```

### Provider 实现（6 种）

| Provider | 类型 | 批量方式 | 特点 |
|----------|------|----------|------|
| `deepseek` | LLM | 15条/批 JSON | 质量最高，但高峰不稳定 |
| `baidu` | 专用API | 单条并发 | 免费额度大（QPS 1），稳定 |
| `tencent` | 专用API | 单条并发 | 已有腾讯云凭证（TTS 在用）|
| `volcano` | 专用API | 单条并发 | 字节火山引擎，有免费额度 |
| `google` | 专用API | 单条并发 | 质量好，需 API key |
| `microsoft` | 专用API | 单条并发 | Azure 翻译，免费层 2M 字符/月 |
| `ollama` | 本地LLM | 15条/批 | 本地模型，无网络依赖 |

### 路由策略（核心）

```
primary = config.translation.primary        # 默认 deepseek
fallbacks = config.translation.fallbacks    # [baidu, tencent, ...]

翻译组失败时:
  1. 重试 primary（config.translation.retries 次）
  2. 仍失败 → 依次尝试 fallbacks（每个最多试 1 次）
  3. 全部失败 → 该组标记失败（不中断其他组）
  4. 组失败数 < 阈值(20%) → 视频继续，记录降级日志
  5. 组失败数 ≥ 阈值 → 视频翻译失败
```

**专用 API 的批量适配**：专用翻译 API 是**单条文本**接口，需要把 15 条字幕拆成 N 个并发请求（QPS 限制内），再按序号重组。并发度 = `min(QPS, 5)`，串行等待保证不超限。

---

## 三、配置设计

```yaml
# config.yaml 新增 translation 段（替换现有 llm_ 平铺配置的翻译部分）
translation:
  primary: deepseek            # 主翻译服务
  fallbacks: [baidu, tencent]  # 降级顺序（可选，空=不降级）
  batch_size: 15               # LLM 类批大小
  max_workers: 1               # LLM 类并发
  retries: 3                   # 主服务重试次数
  context_size: 2              # 上下文句数（LLM 类）
  fail_threshold: 0.2          # 组失败率阈值（超过则视频失败）

  deepseek:
    api_key: "${DEEPSEEK_API_KEY}"   # 兼容现有环境变量
    base_url: "https://api.deepseek.com"
    model: "deepseek-v4-flash"

  baidu:
    app_id: ""
    app_key: ""
    qps: 1                      # 百度免费版 QPS=1

  tencent:                      # 复用 TTS 的腾讯云凭证（可选独立）
    secret_id: "${TENCENT_SECRET_ID}"
    secret_key: "${TENCENT_SECRET_KEY}"
    region: "ap-guangzhou"
    qps: 5

  volcano:
    api_key: ""
    endpoint: "https://translate.volcengineapi.com"
    region: "cn-north-1"
    qps: 5

  google:
    api_key: ""
    qps: 5

  microsoft:
    api_key: ""
    endpoint: "https://api.cognitive.microsofttranslator.com"
    region: "eastasia"
    qps: 10

  ollama:
    base_url: "http://localhost:11434"
    model: "qwen2.5:14b"        # 本机已装
    qps: 1
```

---

## 四、实施步骤

### Phase 1：Provider 抽象 + 适配器（0.5 天）

1. **定义 `Provider` 接口**（`internal/translator/provider.go`）
   ```go
   type Provider interface {
       TranslateBatch(ctx context.Context, texts []string, src, dst string) ([]string, error)
       Name() string
   }
   ```

2. **抽取现有 DeepSeek 逻辑**为 `DeepSeekProvider`（改名为 LLMProvider，保留 JSON 批处理）

3. **实现 4 个专用 API Provider**（对标 translation-tool 实现，修正其占位符）：
   - `baidu.go`：MD5 签名（appid+text+salt+key）+ GET 请求
   - `tencent.go`：腾讯云 TMT v3 签名（复用 config 已有 tencent_cloud 凭证结构）
   - `volcano.go`：火山引擎 TranslateText API
   - `microsoft.go`：Azure Translator v3（Ocp-Apim-Subscription-Key）
   - `google.go`：Cloud Translation v2（可选，P0 不做）

4. **并发批处理适配器**（`batch_adapter.go`）：
   ```go
   // 专用 API 是单条接口，包一层批量并发
   type BatchAdapter struct {
       single func(ctx, text, src, dst) (string, error)
       qps    int
   }
   // 并发度 min(qps, 5)，结果按原序号重组，失败条目标记并重试
   ```

5. **路由器**（`router.go`）：
   ```go
   type Router struct {
       primary   Provider
       fallbacks []Provider
       retries   int
   }
   func (r *Router) TranslateBatch(...) ([]string, error) {
       // primary 重试 → fallback 遍历 → 返回
   }
   ```

### Phase 2：配置 + 接线（0.5 天）

1. `internal/config/config.go`：新增 `TranslationConfig`（含各 Provider 子配置）
2. `config.yaml`：添加 `translation` 段示例（含注释）
3. `translator.SRTContext`：按配置构建 Router，替换硬编码 DeepSeek
4. 兼容：`translation.primary` 为空时退化为现有 `llm_api_key` 配置（不破坏现有用户）

### Phase 3：降级日志 + 统计（0.5 天）

1. 每次降级记录：`[翻译降级] 组 3/15 使用 baidu (deepseek 限流)`
2. 视频级统计：`降级次数 / 总组数`，写入 Result.Errors
3. `ytb debug` 增加翻译 Provider 健康状态展示
4. 新增 `ytb translate --test` 命令：测试所有配置的 Provider 连通性

### Phase 4：验证（0.5 天）

1. 单测：每个 Provider 的签名/请求/解析（mock HTTP）
2. 集成测试：真实调用（用有免费额度的 baidu/tencent）
3. 故障注入：模拟 DeepSeek 失败 → 验证自动降级到 baidu
4. 端到端：跑一条视频全流程验证

---

## 五、优先级评估

| Provider | 优先级 | 理由 |
|----------|:------:|------|
| **baidu** | P0 | 免费、稳定、接入简单（MD5 签名）|
| **tencent** | P0 | 已有腾讯云凭证（TTS 在用），复用成本低 |
| **ollama** | P1 | 本机已装 qwen2.5:14b，零成本本地兜底（质量一般但可用）|
| **volcano** | P1 | 免费额度，接入中等 |
| **microsoft** | P1 | 免费 2M 字符/月，接入中等 |
| **google** | P2 | 需要科学上网 + 付费 key，优先级最低 |

---

## 六、预期收益

| 指标 | 现在 | 接入后 |
|------|------|--------|
| 翻译可用性 | 单点 DeepSeek（高峰 30% 失败）| 主备切换（>99%）|
| 高峰吞吐 | 降并发到 1，速度慢 | 主服务 + 降级分流 |
| 失败恢复 | 整视频跳过 | 降级继续，失败率<20% 不中断 |
| 成本 | DeepSeek 按量 | baidu/tencent 免费额度优先 |

---

## 七、风险与注意事项

1. **专用 API 翻译质量**：术语一致性不如 LLM 上下文翻译 → 建议 fallback 场景接受"可用但略差"
2. **QPS 限制**：专用 API 有严格 QPS，批量适配器必须限流（token bucket）
3. **语言检测**：现有 `shouldTranslate` 用 LLM 判定语言 → 专用 API 走不通时保留 LLM 判定或简化
4. **凭证安全**：各 Provider key 存 config.yaml（现有模式），[REDACTED] 不上传
5. **translation-tool 参考修正**：其 baidu 签名是占位符（`generateSign` 未实现）、URL 未 URL 编码，接入时需实现正确版本

---

## 八、参考资源

- translation-tool 源码：`/tmp/translation-tool/`
  - `internal/services/`：baidu/google/microsoft/tencent/volcano/ollama 适配器
  - `internal/interfaces/translator.go`：统一接口
  - `internal/services/services.go`：Manager 注册表模式
- 现有实现：`internal/translator/translator.go`（LLM 批处理 + 严格校验）
- 现有配置：`internal/config/config.go`（llm_* 平铺 + tencent_cloud TTS 凭证）
