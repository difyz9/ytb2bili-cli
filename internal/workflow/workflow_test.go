package workflow

import (
	"context"
	"reflect"
	"testing"
)

func testRegistry(t *testing.T) *Registry {
	t.Helper()
	noop := func(context.Context, *State) error { return nil }
	r, err := NewRegistry(
		Step{Name: "download", Run: noop},
		Step{Name: "transcribe", Requires: []string{"download"}, Run: noop},
		Step{Name: "translate", Requires: []string{"transcribe"}, Run: noop},
		Step{Name: "metadata", Requires: []string{"download"}, Run: noop},
		Step{Name: "upload", Requires: []string{"download", "metadata"}, Run: noop},
	)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestAdaptivePlannerDefault(t *testing.T) {
	plan, err := (AdaptivePlanner{}).Plan(context.Background(), Intent{}, testRegistry(t))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"download", "transcribe", "translate", "metadata", "upload"}
	if !reflect.DeepEqual(plan, want) {
		t.Fatalf("plan = %v, want %v", plan, want)
	}
}

func TestAdaptivePlannerUserChainExpandsDependencies(t *testing.T) {
	plan, err := (AdaptivePlanner{}).Plan(context.Background(), Intent{Requested: []string{"translate"}}, testRegistry(t))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"download", "transcribe", "translate"}
	if !reflect.DeepEqual(plan, want) {
		t.Fatalf("plan = %v, want %v", plan, want)
	}
}

func TestAdaptivePlannerDryRunAndSkipTranslate(t *testing.T) {
	plan, err := (AdaptivePlanner{}).Plan(context.Background(), Intent{DryRun: true, SkipTranslate: true}, testRegistry(t))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"download", "transcribe", "metadata"}
	if !reflect.DeepEqual(plan, want) {
		t.Fatalf("plan = %v, want %v", plan, want)
	}
}

func TestAdaptivePlannerSafetyFlagsOverrideCustomChain(t *testing.T) {
	plan, err := (AdaptivePlanner{}).Plan(context.Background(), Intent{
		Requested: []string{"translate", "upload"}, DryRun: true, SkipTranslate: true,
	}, testRegistry(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 0 {
		t.Fatalf("plan = %v, want empty", plan)
	}
}

func TestExecutorStopsOnFailure(t *testing.T) {
	var ran []string
	r, _ := NewRegistry(
		Step{Name: "one", Run: func(context.Context, *State) error { ran = append(ran, "one"); return context.Canceled }},
		Step{Name: "two", Run: func(context.Context, *State) error { ran = append(ran, "two"); return nil }},
	)
	err := (Executor{Registry: r}).Run(context.Background(), []string{"one", "two"}, NewState())
	if err == nil || !reflect.DeepEqual(ran, []string{"one"}) {
		t.Fatalf("err=%v ran=%v", err, ran)
	}
}

type fakeDecider struct{ steps []string }

func (f fakeDecider) Decide(context.Context, AgentRequest) ([]string, error) { return f.steps, nil }

func TestAgentPlannerValidatesAndExpandsProposal(t *testing.T) {
	planner := AgentPlanner{Provider: fakeDecider{steps: []string{"translate"}}}
	plan, err := planner.Plan(context.Background(), Intent{Goal: "生成中文字幕"}, testRegistry(t))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"download", "transcribe", "translate"}
	if !reflect.DeepEqual(plan, want) {
		t.Fatalf("plan=%v want=%v", plan, want)
	}
}

func TestAgentPlannerCannotBypassDryRun(t *testing.T) {
	planner := AgentPlanner{Provider: fakeDecider{steps: []string{"upload"}}}
	plan, err := planner.Plan(context.Background(), Intent{Goal: "投稿", DryRun: true}, testRegistry(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 0 {
		t.Fatalf("plan=%v want empty", plan)
	}
}

func TestDisabledDependencyIsRejected(t *testing.T) {
	noop := func(context.Context, *State) error { return nil }
	registry, err := NewRegistry(
		Step{Name: "translate", Run: noop},
		Step{Name: "audio-sync", Requires: []string{"translate"}, Run: noop},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = (AdaptivePlanner{}).Plan(context.Background(), Intent{Requested: []string{"audio-sync"}, SkipTranslate: true}, registry)
	if err == nil {
		t.Fatal("expected disabled dependency error")
	}
}
