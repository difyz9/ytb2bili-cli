package translator

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/zolagz/ytb2bili-go/internal/config"
)

// Provider 翻译服务提供者接口（Phase 1 多源接入）。
// 实现类：DeepSeekProvider（LLM 批处理）、BaiduProvider、TencentProvider（专用 API）。
type Provider interface {
	// TranslateBatch 翻译一批文本，返回与输入等长、按原序的译文。
	TranslateBatch(ctx context.Context, texts []string, sourceLang, targetLang string) ([]string, error)
	// Name 返回提供者名称（deepseek/baidu/tencent/...）。
	Name() string
}

// ProviderStats 单个提供者的使用统计（诊断用）。
type ProviderStats struct {
	Name        string
	Used        int  // 成功使用次数
	Failures    int  // 失败次数
	LastUsedAt  time.Time
}

// TestResult 单个 Provider 连通性测试结果。
type TestResult struct {
	Name    string
	OK      bool
	Latency time.Duration
	Error   string
	Note    string
}

// TestProviders 测试所有已配置 Provider 的连通性（翻译一行文本）。
// 返回按配置顺序排列的结果。cfg 为 nil 或未配置 translation 时只测 deepseek。
func TestProviders(cfg *config.Config) []TestResult {
	var results []TestResult

	// 构建 router（含所有已注册 provider）
	router := buildRouter(cfg.Translation, cfg.LLMAPIKey, cfg.LLMBaseURL, cfg.LLMModel, cfg.TencentCloud)
	if router == nil {
		// 未配置 translation 段 → 只测默认 LLM
		ds := NewDeepSeekProvider(DeepSeekConfig{
			APIKey: cfg.LLMAPIKey, BaseURL: cfg.LLMBaseURL, Model: cfg.LLMModel,
		})
		return []TestResult{testOneProvider(ds)}
	}

	// 收集 router 内所有 provider（primary + fallbacks）
	seen := map[string]bool{}
	if router.primary != nil && !seen[router.primary.Name()] {
		seen[router.primary.Name()] = true
		results = append(results, testOneProvider(router.primary))
	}
	for _, fb := range router.fallbacks {
		if !seen[fb.Name()] {
			seen[fb.Name()] = true
			results = append(results, testOneProvider(fb))
		}
	}
	return results
}

func testOneProvider(p Provider) TestResult {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	start := time.Now()
	out, err := p.TranslateBatch(ctx, []string{"Hello world, this is a translation test."}, "en", "zh-Hans")
	latency := time.Since(start)
	res := TestResult{Name: p.Name(), Latency: latency}
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.OK = true
	if len(out) > 0 && out[0] != "" {
		res.Note = "样例: " + truncateStr(out[0], 60)
	}
	return res
}

// Router 主备翻译路由器：primary 重试 → fallbacks 依次降级。
type Router struct {
	primary   Provider
	fallbacks []Provider
	retries   int
	stats     map[string]*ProviderStats
	mu        sync.Mutex
}

// NewRouter 创建路由器。retries 为 primary 的重试次数（默认 2）。
func NewRouter(primary Provider, fallbacks []Provider, retries int) *Router {
	if retries < 0 {
		retries = 2
	}
	r := &Router{
		primary:   primary,
		fallbacks: fallbacks,
		retries:   retries,
		stats:     make(map[string]*ProviderStats),
	}
	r.stats[primary.Name()] = &ProviderStats{Name: primary.Name()}
	for _, f := range fallbacks {
		r.stats[f.Name()] = &ProviderStats{Name: f.Name()}
	}
	return r
}

// TranslateBatch 主备路由翻译：
//  1. primary 重试 retries 次
//  2. 仍失败 → 依次尝试 fallbacks（各 1 次）
//  3. 全部失败 → 返回最后一个错误
func (r *Router) TranslateBatch(ctx context.Context, texts []string, sourceLang, targetLang string) ([]string, error) {
	// primary 重试
	var lastErr error
	for attempt := 0; attempt <= r.retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}
		out, err := r.invoke(r.primary, ctx, texts, sourceLang, targetLang)
		if err == nil {
			return out, nil
		}
		lastErr = err
		log.Printf("  ⚠ [%s] 第 %d/%d 次失败: %v", r.primary.Name(), attempt+1, r.retries+1, err)
	}

	// fallbacks 降级
	for _, fb := range r.fallbacks {
		log.Printf("  🔄 降级到 %s ...", fb.Name())
		out, err := r.invoke(fb, ctx, texts, sourceLang, targetLang)
		if err == nil {
			log.Printf("  ✅ %s 降级成功", fb.Name())
			return out, nil
		}
		lastErr = err
		log.Printf("  ⚠ [%s] 降级失败: %v", fb.Name(), err)
	}

	return nil, fmt.Errorf("全部翻译服务失败 (primary=%s): %w", r.primary.Name(), lastErr)
}

func (r *Router) invoke(p Provider, ctx context.Context, texts []string, src, dst string) ([]string, error) {
	out, err := p.TranslateBatch(ctx, texts, src, dst)
	r.mu.Lock()
	s := r.stats[p.Name()]
	if s == nil {
		s = &ProviderStats{Name: p.Name()}
		r.stats[p.Name()] = s
	}
	s.LastUsedAt = time.Now()
	if err != nil {
		s.Failures++
	} else {
		s.Used++
	}
	r.mu.Unlock()
	return out, err
}

// Stats 返回所有提供者的使用统计快照。
func (r *Router) Stats() map[string]*ProviderStats {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]*ProviderStats, len(r.stats))
	for k, v := range r.stats {
		cp := *v
		out[k] = &cp
	}
	return out
}

// ─── BatchAdapter：专用 API（单条接口）→ 批量并发适配器 ──────────

// SingleTranslator 单条文本翻译函数签名（专用 API 适配器实现）。
type SingleTranslator func(ctx context.Context, text, sourceLang, targetLang string) (string, error)

// BatchAdapter 把单条翻译函数包装为批量 Provider，按原序号重组结果。
// 并发度受 qps 限制（token bucket 简化：并发数 = qps，超限排队）。
type BatchAdapter struct {
	name   string
	single SingleTranslator
	qps    int
}

// NewBatchAdapter 创建批量适配器。
func NewBatchAdapter(name string, single SingleTranslator, qps int) *BatchAdapter {
	if qps <= 0 {
		qps = 5
	}
	return &BatchAdapter{name: name, single: single, qps: qps}
}

func (b *BatchAdapter) Name() string { return b.name }

// TranslateBatch 并发翻译一批文本，保持原序。
func (b *BatchAdapter) TranslateBatch(ctx context.Context, texts []string, sourceLang, targetLang string) ([]string, error) {
	if len(texts) == 0 {
		return []string{}, nil
	}
	if len(texts) == 1 {
		out, err := b.single(ctx, texts[0], sourceLang, targetLang)
		if err != nil {
			return nil, err
		}
		return []string{out}, nil
	}

	results := make([]string, len(texts))
	errs := make([]error, len(texts))
	sem := make(chan struct{}, b.qps)
	var wg sync.WaitGroup
	for i, text := range texts {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, txt string) {
			defer wg.Done()
			defer func() { <-sem }()
			out, err := b.single(ctx, txt, sourceLang, targetLang)
			results[idx] = out
			errs[idx] = err
		}(i, text)
	}
	wg.Wait()

	// 汇总错误
	var failed int
	var firstErr error
	for i, err := range errs {
		if err != nil {
			failed++
			if firstErr == nil {
				firstErr = fmt.Errorf("第 %d 条: %w", i+1, err)
			}
		}
	}
	if failed > 0 {
		return nil, fmt.Errorf("%d/%d 条翻译失败，首个错误: %w", failed, len(texts), firstErr)
	}
	return results, nil
}
