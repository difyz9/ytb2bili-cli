// Package workflow provides a small, deterministic task-chain planner and executor.
// Planners decide what to run; the executor validates and runs only registered steps.
package workflow

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type StepFunc func(context.Context, *State) error

type Step struct {
	Name        string
	Description string
	Requires    []string
	Run         StepFunc
}

type State struct {
	Values map[string]any
}

func NewState() *State { return &State{Values: make(map[string]any)} }

type Registry struct{ steps map[string]Step }

func NewRegistry(steps ...Step) (*Registry, error) {
	r := &Registry{steps: make(map[string]Step, len(steps))}
	for _, step := range steps {
		step.Name = normalize(step.Name)
		if step.Name == "" || step.Run == nil {
			return nil, fmt.Errorf("workflow: step name and runner are required")
		}
		if _, exists := r.steps[step.Name]; exists {
			return nil, fmt.Errorf("workflow: duplicate step %q", step.Name)
		}
		for i := range step.Requires {
			step.Requires[i] = normalize(step.Requires[i])
		}
		r.steps[step.Name] = step
	}
	for _, step := range r.steps {
		for _, dep := range step.Requires {
			if _, ok := r.steps[dep]; !ok {
				return nil, fmt.Errorf("workflow: step %q requires unknown step %q", step.Name, dep)
			}
		}
	}
	return r, nil
}

func (r *Registry) Step(name string) (Step, bool) {
	step, ok := r.steps[normalize(name)]
	return step, ok
}

type Intent struct {
	Requested     []string
	DryRun        bool
	SkipTranslate bool
	Goal          string
}

// Planner is deliberately replaceable: an LLM agent may implement it, while the
// registry and executor remain the safety boundary.
type Planner interface {
	Plan(context.Context, Intent, *Registry) ([]string, error)
}

// AdaptivePlanner creates the normal chain from intent, or expands a user chain
// with its declared dependencies.
type AdaptivePlanner struct{}

func (AdaptivePlanner) Plan(_ context.Context, intent Intent, registry *Registry) ([]string, error) {
	targets := intent.Requested
	if len(targets) == 0 {
		targets = []string{"download", "transcribe"}
		if !intent.SkipTranslate {
			targets = append(targets, "translate")
		}
		targets = append(targets, "metadata")
		if !intent.DryRun {
			targets = append(targets, "upload")
		}
	} else {
		// Safety flags are hard constraints even when a custom/agent plan asks for
		// the corresponding side-effecting steps.
		filtered := make([]string, 0, len(targets))
		for _, target := range targets {
			name := normalize(target)
			if (intent.DryRun && name == "upload") || (intent.SkipTranslate && name == "translate") {
				continue
			}
			filtered = append(filtered, target)
		}
		targets = filtered
	}

	var plan []string
	visiting, added := map[string]bool{}, map[string]bool{}
	var add func(string) error
	add = func(raw string) error {
		name := normalize(raw)
		if intent.SkipTranslate && name == "translate" {
			return fmt.Errorf("workflow: step %q requires translation, but translation is disabled", raw)
		}
		if intent.DryRun && name == "upload" {
			return fmt.Errorf("workflow: step %q is disabled by dry-run", raw)
		}
		step, ok := registry.Step(name)
		if !ok {
			return fmt.Errorf("workflow: unknown step %q", raw)
		}
		if visiting[name] {
			return fmt.Errorf("workflow: dependency cycle at %q", name)
		}
		if added[name] {
			return nil
		}
		visiting[name] = true
		for _, dep := range step.Requires {
			if err := add(dep); err != nil {
				return err
			}
		}
		visiting[name] = false
		added[name] = true
		plan = append(plan, name)
		return nil
	}
	for _, target := range targets {
		if err := add(target); err != nil {
			return nil, err
		}
	}
	return plan, nil
}

type StepInfo struct {
	Name, Description string
	Requires          []string
}

func Catalog(r *Registry) []StepInfo {
	infos := make([]StepInfo, 0, len(r.steps))
	for _, name := range Available(r) {
		step, _ := r.Step(name)
		infos = append(infos, StepInfo{Name: step.Name, Description: step.Description, Requires: append([]string(nil), step.Requires...)})
	}
	return infos
}

type AgentRequest struct {
	Goal      string
	Available []StepInfo
}

// DecisionProvider may be backed by an LLM, rules engine, or remote agent.
// It only proposes registered step names and never executes them directly.
type DecisionProvider interface {
	Decide(context.Context, AgentRequest) ([]string, error)
}

type AgentPlanner struct{ Provider DecisionProvider }

func (p AgentPlanner) Plan(ctx context.Context, intent Intent, registry *Registry) ([]string, error) {
	if len(intent.Requested) > 0 {
		return (AdaptivePlanner{}).Plan(ctx, intent, registry)
	}
	if p.Provider == nil {
		return nil, fmt.Errorf("workflow: agent decision provider is required")
	}
	proposed, err := p.Provider.Decide(ctx, AgentRequest{Goal: intent.Goal, Available: Catalog(registry)})
	if err != nil {
		return nil, fmt.Errorf("workflow: agent planning failed: %w", err)
	}
	intent.Requested = proposed
	return (AdaptivePlanner{}).Plan(ctx, intent, registry)
}

type Observer interface {
	StepStarted(name string, position, total int)
	StepFinished(name string, err error)
}

type Executor struct {
	Registry *Registry
	Observer Observer
}

func (e Executor) Run(ctx context.Context, plan []string, state *State) error {
	if e.Registry == nil {
		return fmt.Errorf("workflow: registry is required")
	}
	if state == nil {
		state = NewState()
	}
	for i, name := range plan {
		step, ok := e.Registry.Step(name)
		if !ok {
			return fmt.Errorf("workflow: unknown step %q", name)
		}
		if e.Observer != nil {
			e.Observer.StepStarted(step.Name, i+1, len(plan))
		}
		err := step.Run(ctx, state)
		if e.Observer != nil {
			e.Observer.StepFinished(step.Name, err)
		}
		if err != nil {
			return fmt.Errorf("workflow step %s: %w", step.Name, err)
		}
	}
	return nil
}

func ParseChain(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '>' || r == ' ' })
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if name := normalize(part); name != "" {
			result = append(result, name)
		}
	}
	return result
}

func Available(r *Registry) []string {
	names := make([]string, 0, len(r.steps))
	for name := range r.steps {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func normalize(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
