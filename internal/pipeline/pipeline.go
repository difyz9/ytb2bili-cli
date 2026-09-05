// Package pipeline implements the shared YouTube to Bilibili application workflow.
package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/zolagz/ytb2bili-go/internal/config"
	"github.com/zolagz/ytb2bili-go/internal/llm"
	"github.com/zolagz/ytb2bili-go/internal/metadata"
	"github.com/zolagz/ytb2bili-go/internal/search"
	"github.com/zolagz/ytb2bili-go/internal/storage"
	"github.com/zolagz/ytb2bili-go/internal/workflow"
)

type Request struct {
	URL, SourceLang, TargetLang, Source string
	CookiesPath                         string
	Tid                                 int
	DryRun, SkipTranslate, PlanOnly     bool
	Chain                               []string
	Planner, Goal                       string
	TaskID                              string
	AudioDir                            string
	DisableAudioSpeedAdjust             bool
	AudioMissingMode                    string
}

type Result struct {
	TaskID, VideoID, DownloadDir, VideoPath, SubtitlePath, SyncedVideoPath, BVID string
	Plan                                                                         []string
	Metadata                                                                     *metadata.VideoMeta
	Duration                                                                     time.Duration
}

// ArtifactID is the stable directory/file identity for media artifacts.
// TaskID remains the execution identity and must not leak into media paths.
func (r *Result) ArtifactID() string {
	if r.VideoID != "" {
		return r.VideoID
	}
	return r.TaskID
}

type Event struct {
	Step            string
	Position, Total int
	Status          string
	Err             error
}

type Reporter func(Event)

type Processor struct {
	Config   *config.Config
	Planner  workflow.Planner
	Reporter Reporter
}

func (p *Processor) Process(ctx context.Context, req Request) (*Result, error) {
	if p.Config == nil {
		return nil, fmt.Errorf("pipeline: config is required")
	}
	if req.URL == "" {
		return nil, fmt.Errorf("pipeline: URL is required")
	}
	if req.SourceLang == "" {
		req.SourceLang = "en"
	}
	if req.TargetLang == "" {
		req.TargetLang = p.Config.EffectiveTranslationTargetLang()
	}
	if req.Tid == 0 {
		req.Tid = p.Config.BiliTid
	}
	if req.Source == "" {
		req.Source = "manual"
	}
	planner := p.Planner
	if planner == nil {
		switch strings.ToLower(strings.TrimSpace(req.Planner)) {
		case "agent":
			if len(req.Chain) == 0 && strings.TrimSpace(req.Goal) == "" {
				return nil, fmt.Errorf("使用 agent 规划器时必须提供 goal")
			}
			planner = workflow.AgentPlanner{Provider: LLMDecisionProvider{Config: p.Config}}
		case "", "adaptive":
			planner = workflow.AdaptivePlanner{}
		default:
			return nil, fmt.Errorf("未知规划器 %q，可用值: adaptive, agent", req.Planner)
		}
	}

	started := time.Now()
	videoID := ExtractYouTubeID(req.URL)
	// 裸 videoId（如直接传 11 位 ID）归一化为完整 watch URL
	req.URL = normalizeURL(req.URL, videoID)
	history := storage.NewHistoryStore(filepath.Join(p.Config.DataDir, "history"))
	if videoID != "" && history.IsSubmitted(videoID) {
		submitted := history.GetSubmitted(videoID)
		return nil, fmt.Errorf("该视频已提交过: https://www.bilibili.com/video/%s", submitted.BVID)
	}

	tasks := storage.NewTaskStore(filepath.Join(p.Config.DataDir, "tasks"))
	task := tasks.Prepare(req.TaskID, req.URL)
	result := &Result{TaskID: task.ID, VideoID: videoID}
	state := &PipelineState{Request: req, Result: result}
	registry, err := buildRegistry(state, stepDeps{config: p.Config, tasks: tasks, history: history})
	if err != nil {
		return nil, err
	}

	result.Plan, err = planner.Plan(ctx, workflow.Intent{Requested: req.Chain, DryRun: req.DryRun, SkipTranslate: req.SkipTranslate, Goal: req.Goal}, registry)
	if err != nil {
		return nil, fmt.Errorf("任务链规划失败: %w（可用步骤: %s）", err, strings.Join(workflow.Available(registry), ", "))
	}
	if req.PlanOnly {
		return result, nil
	}
	if err := tasks.Persist(task, result.Plan); err != nil {
		return nil, fmt.Errorf("创建任务记录失败: %w", err)
	}

	observer := &taskObserver{tasks: tasks, taskID: task.ID, report: p.Reporter}
	executor := workflow.Executor{Registry: registry, Observer: observer}
	// 步骤级超时（来自 daemon 配置）：超时自动 kill 重试，防止长任务无限卡死
	if p.Config != nil && p.Config.Daemon != nil && len(p.Config.Daemon.StepTimeoutSec) > 0 {
		executor.StepTimeout = make(map[string]time.Duration, len(p.Config.Daemon.StepTimeoutSec))
		for step, sec := range p.Config.Daemon.StepTimeoutSec {
			if sec > 0 {
				executor.StepTimeout[step] = time.Duration(sec) * time.Second
			}
		}
	}
	if err = executor.Run(ctx, result.Plan, workflow.NewState()); err != nil {
		return result, err
	}
	tasks.SetCompleted(task.ID)
	result.Duration = time.Since(started)
	return result, nil
}

// LLMDecisionProvider lets the configured model propose step names. The
// proposal is still validated and dependency-expanded by AgentPlanner.
type LLMDecisionProvider struct{ Config *config.Config }

func (p LLMDecisionProvider) Decide(ctx context.Context, req workflow.AgentRequest) ([]string, error) {
	if p.Config == nil || p.Config.LLMAPIKey == "" {
		return nil, fmt.Errorf("Agent 规划需要配置 LLM API Key")
	}
	catalog, _ := json.Marshal(req.Available)
	prompt := fmt.Sprintf(`你是视频处理任务规划器。根据用户目标，从可用步骤中选择必要步骤。
只能使用给出的步骤名称，不要发明工具。最小化步骤数量。只输出 JSON：{"steps":["step"]}。
用户目标：%s
可用步骤：%s`, req.Goal, catalog)
	response, err := (&llm.OpenAIClient{APIKey: p.Config.LLMAPIKey, BaseURL: p.Config.LLMBaseURL, Model: p.Config.LLMModel}).Complete(ctx, prompt)
	if err != nil {
		return nil, err
	}
	start, end := strings.Index(response, "{"), strings.LastIndex(response, "}")
	if start < 0 || end < start {
		return nil, fmt.Errorf("Agent 返回的计划不是 JSON")
	}
	var decision struct {
		Steps []string `json:"steps"`
	}
	if err := json.Unmarshal([]byte(response[start:end+1]), &decision); err != nil {
		return nil, fmt.Errorf("解析 Agent 计划失败: %w", err)
	}
	if len(decision.Steps) == 0 {
		return nil, fmt.Errorf("Agent 返回了空任务链")
	}
	return decision.Steps, nil
}

type taskObserver struct {
	tasks  *storage.TaskStore
	taskID string
	report Reporter
}

func (o *taskObserver) StepStarted(name string, position, total int) {
	o.tasks.UpdateStep(o.taskID, name, "running")
	if o.report != nil {
		o.report(Event{Step: name, Position: position, Total: total, Status: "running"})
	}
}
func (o *taskObserver) StepFinished(name string, err error) {
	status := "completed"
	if err != nil {
		status = "failed"
		o.tasks.UpdateStep(o.taskID, name, status, err.Error())
	} else {
		o.tasks.UpdateStep(o.taskID, name, status)
	}
	if o.report != nil {
		o.report(Event{Step: name, Status: status, Err: err})
	}
}

func ExtractYouTubeID(url string) string {
	return search.ExtractVideoID(url)
}

// normalizeURL 将裸 videoId（如直接传 11 位 ID）归一化为完整 watch URL；其余原样返回。
func normalizeURL(url, videoID string) string {
	if videoID != "" && url == videoID {
		return "https://www.youtube.com/watch?v=" + videoID
	}
	return url
}
