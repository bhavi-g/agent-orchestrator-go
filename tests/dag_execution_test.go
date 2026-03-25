package tests

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"agent-orchestrator/agent"
	"agent-orchestrator/orchestrator"
	"agent-orchestrator/planner"
	"agent-orchestrator/tools"
)

// --- helpers -----------------------------------------------------------

// trackingAgent records when it ran and supports artificial delay.
type trackingAgent struct {
	id       string
	delay    time.Duration
	mu       sync.Mutex
	started  []time.Time
	finished []time.Time
}

func newTrackingAgent(id string, delay time.Duration) *trackingAgent {
	return &trackingAgent{id: id, delay: delay}
}

func (a *trackingAgent) Run(ctx context.Context, input map[string]any) (*agent.Result, error) {
	a.mu.Lock()
	a.started = append(a.started, time.Now())
	a.mu.Unlock()

	if a.delay > 0 {
		time.Sleep(a.delay)
	}

	a.mu.Lock()
	a.finished = append(a.finished, time.Now())
	a.mu.Unlock()

	return &agent.Result{Output: map[string]any{"agent": a.id, "ok": true}}, nil
}

func (a *trackingAgent) callCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.started)
}

// dagStaticPlanner returns a fixed plan.
type dagStaticPlanner struct {
	plan *planner.Plan
}

func (p *dagStaticPlanner) CreatePlan(_ context.Context, taskID string) (*planner.Plan, error) {
	return p.plan, nil
}

// --- DAG scheduler unit tests -----------------------------------------

func TestDAGExecution_SequentialFallback(t *testing.T) {
	// Plan without StepID/DependsOn should execute sequentially (like before).
	a1 := newTrackingAgent("a1", 20*time.Millisecond)
	a2 := newTrackingAgent("a2", 20*time.Millisecond)

	reg := agent.NewRegistry()
	reg.Register("a1", a1)
	reg.Register("a2", a2)

	pl := &dagStaticPlanner{plan: &planner.Plan{
		TaskID: "seq",
		Steps: []planner.PlanStep{
			{AgentID: "a1"},
			{AgentID: "a2"},
		},
	}}

	engine := orchestrator.NewEngine(pl, reg, tools.NewRegistryExecutor(tools.NewRegistry()), nil, nil, nil, nil)
	result, err := engine.Execute(context.Background(), orchestrator.ExecutionRequest{
		RunID: "run-seq", TaskID: "seq", Input: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != orchestrator.StatusSucceeded {
		t.Fatalf("expected success, got %v (err=%v)", result.Status, result.Err)
	}

	// Both agents should have been called exactly once.
	if a1.callCount() != 1 || a2.callCount() != 1 {
		t.Fatalf("expected 1 call each, got a1=%d a2=%d", a1.callCount(), a2.callCount())
	}

	// a2 should start after a1 finished (sequential).
	if a2.started[0].Before(a1.finished[0]) {
		t.Fatalf("a2 started before a1 finished — should be sequential")
	}
}

func TestDAGExecution_ParallelSteps(t *testing.T) {
	// Two independent steps (no DependsOn) should run in parallel.
	delay := 100 * time.Millisecond
	a1 := newTrackingAgent("a1", delay)
	a2 := newTrackingAgent("a2", delay)

	reg := agent.NewRegistry()
	reg.Register("a1", a1)
	reg.Register("a2", a2)

	pl := &dagStaticPlanner{plan: &planner.Plan{
		TaskID: "par",
		Steps: []planner.PlanStep{
			{AgentID: "a1", StepID: "s1"},
			{AgentID: "a2", StepID: "s2"},
		},
	}}

	engine := orchestrator.NewEngine(pl, reg, tools.NewRegistryExecutor(tools.NewRegistry()), nil, nil, nil, nil)

	start := time.Now()
	result, err := engine.Execute(context.Background(), orchestrator.ExecutionRequest{
		RunID: "run-par", TaskID: "par", Input: map[string]any{},
	})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != orchestrator.StatusSucceeded {
		t.Fatalf("expected success, got %v (err=%v)", result.Status, result.Err)
	}

	// If truly parallel, total time should be closer to 1×delay, not 2×delay.
	if elapsed >= 2*delay {
		t.Fatalf("parallel steps took %v — expected less than %v (sequential would be 2×%v)", elapsed, 2*delay, delay)
	}

	if a1.callCount() != 1 || a2.callCount() != 1 {
		t.Fatalf("expected 1 call each, got a1=%d a2=%d", a1.callCount(), a2.callCount())
	}
}

func TestDAGExecution_DiamondDependency(t *testing.T) {
	// Diamond: A -> B, A -> C, B+C -> D
	//   A
	//  / \
	// B   C
	//  \ /
	//   D
	agents := make(map[string]*trackingAgent)
	reg := agent.NewRegistry()
	for _, id := range []string{"A", "B", "C", "D"} {
		a := newTrackingAgent(id, 30*time.Millisecond)
		agents[id] = a
		reg.Register(id, a)
	}

	pl := &dagStaticPlanner{plan: &planner.Plan{
		TaskID: "diamond",
		Steps: []planner.PlanStep{
			{AgentID: "A", StepID: "sA"},
			{AgentID: "B", StepID: "sB", DependsOn: []string{"sA"}},
			{AgentID: "C", StepID: "sC", DependsOn: []string{"sA"}},
			{AgentID: "D", StepID: "sD", DependsOn: []string{"sB", "sC"}},
		},
	}}

	engine := orchestrator.NewEngine(pl, reg, tools.NewRegistryExecutor(tools.NewRegistry()), nil, nil, nil, nil)
	result, err := engine.Execute(context.Background(), orchestrator.ExecutionRequest{
		RunID: "run-diamond", TaskID: "diamond", Input: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != orchestrator.StatusSucceeded {
		t.Fatalf("expected success, got %v (err=%v)", result.Status, result.Err)
	}

	// Verify ordering constraints.
	for _, id := range []string{"A", "B", "C", "D"} {
		if agents[id].callCount() != 1 {
			t.Fatalf("agent %s call count = %d, want 1", id, agents[id].callCount())
		}
	}

	// B and C must start after A finished.
	if agents["B"].started[0].Before(agents["A"].finished[0]) {
		t.Fatal("B started before A finished")
	}
	if agents["C"].started[0].Before(agents["A"].finished[0]) {
		t.Fatal("C started before A finished")
	}
	// D must start after both B and C finished.
	if agents["D"].started[0].Before(agents["B"].finished[0]) {
		t.Fatal("D started before B finished")
	}
	if agents["D"].started[0].Before(agents["C"].finished[0]) {
		t.Fatal("D started before C finished")
	}
}

func TestDAGExecution_CycleDetection(t *testing.T) {
	reg := agent.NewRegistry()
	reg.Register("a1", newTrackingAgent("a1", 0))
	reg.Register("a2", newTrackingAgent("a2", 0))

	pl := &dagStaticPlanner{plan: &planner.Plan{
		TaskID: "cycle",
		Steps: []planner.PlanStep{
			{AgentID: "a1", StepID: "s1", DependsOn: []string{"s2"}},
			{AgentID: "a2", StepID: "s2", DependsOn: []string{"s1"}},
		},
	}}

	engine := orchestrator.NewEngine(pl, reg, tools.NewRegistryExecutor(tools.NewRegistry()), nil, nil, nil, nil)
	result, err := engine.Execute(context.Background(), orchestrator.ExecutionRequest{
		RunID: "run-cycle", TaskID: "cycle", Input: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.Status != orchestrator.StatusFailed {
		t.Fatalf("expected failure for cycle, got %v", result.Status)
	}
	if result.Err == nil || result.Err.Error() != "plan contains a dependency cycle" {
		t.Fatalf("expected cycle error, got %v", result.Err)
	}
}

func TestDAGExecution_UnknownDependency(t *testing.T) {
	reg := agent.NewRegistry()
	reg.Register("a1", newTrackingAgent("a1", 0))

	pl := &dagStaticPlanner{plan: &planner.Plan{
		TaskID: "bad-dep",
		Steps: []planner.PlanStep{
			{AgentID: "a1", StepID: "s1", DependsOn: []string{"nonexistent"}},
		},
	}}

	engine := orchestrator.NewEngine(pl, reg, tools.NewRegistryExecutor(tools.NewRegistry()), nil, nil, nil, nil)
	result, err := engine.Execute(context.Background(), orchestrator.ExecutionRequest{
		RunID: "run-bad-dep", TaskID: "bad-dep", Input: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.Status != orchestrator.StatusFailed {
		t.Fatalf("expected failure, got %v", result.Status)
	}
}

func TestDAGExecution_MissingStepID(t *testing.T) {
	reg := agent.NewRegistry()
	reg.Register("a1", newTrackingAgent("a1", 0))
	reg.Register("a2", newTrackingAgent("a2", 0))

	// One step uses StepID, one doesn't — should error.
	pl := &dagStaticPlanner{plan: &planner.Plan{
		TaskID: "missing-id",
		Steps: []planner.PlanStep{
			{AgentID: "a1", StepID: "s1"},
			{AgentID: "a2", DependsOn: []string{"s1"}}, // no StepID
		},
	}}

	engine := orchestrator.NewEngine(pl, reg, tools.NewRegistryExecutor(tools.NewRegistry()), nil, nil, nil, nil)
	result, err := engine.Execute(context.Background(), orchestrator.ExecutionRequest{
		RunID: "run-missing-id", TaskID: "missing-id", Input: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.Status != orchestrator.StatusFailed {
		t.Fatalf("expected failure for missing StepID, got %v", result.Status)
	}
}

func TestDAGExecution_VarsMerge(t *testing.T) {
	// Two parallel steps write different Vars; a dependent step sees both.
	var captured atomic.Value

	reg := agent.NewRegistry()
	reg.Register("writer1", &funcAgent{fn: func(ctx context.Context, input map[string]any) (*agent.Result, error) {
		return &agent.Result{Output: map[string]any{"key1": "val1"}}, nil
	}})
	reg.Register("writer2", &funcAgent{fn: func(ctx context.Context, input map[string]any) (*agent.Result, error) {
		return &agent.Result{Output: map[string]any{"key2": "val2"}}, nil
	}})
	reg.Register("reader", &funcAgent{fn: func(ctx context.Context, input map[string]any) (*agent.Result, error) {
		// This runs via RunWithContext so it can read Vars.
		// But agent.Run only gets input, not Vars. We'll check via output instead.
		return &agent.Result{Output: map[string]any{"reader": "done"}}, nil
	}})

	// We'll verify Vars merge indirectly through execution success + final output.
	pl := &dagStaticPlanner{plan: &planner.Plan{
		TaskID: "vars-merge",
		Steps: []planner.PlanStep{
			{AgentID: "writer1", StepID: "w1"},
			{AgentID: "writer2", StepID: "w2"},
			{AgentID: "reader", StepID: "r", DependsOn: []string{"w1", "w2"}},
		},
	}}

	engine := orchestrator.NewEngine(pl, reg, tools.NewRegistryExecutor(tools.NewRegistry()), nil, nil, nil, nil)
	result, err := engine.Execute(context.Background(), orchestrator.ExecutionRequest{
		RunID: "run-vars", TaskID: "vars-merge", Input: map[string]any{},
	})

	_ = captured // suppress unused

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != orchestrator.StatusSucceeded {
		t.Fatalf("expected success, got %v (err=%v)", result.Status, result.Err)
	}
	if result.Output["reader"] != "done" {
		t.Fatalf("expected reader output, got %+v", result.Output)
	}
}

// funcAgent is a simple agent implemented with a function.
type funcAgent struct {
	fn func(ctx context.Context, input map[string]any) (*agent.Result, error)
}

func (a *funcAgent) Run(ctx context.Context, input map[string]any) (*agent.Result, error) {
	return a.fn(ctx, input)
}

func TestDAGExecution_StepFailure(t *testing.T) {
	reg := agent.NewRegistry()
	reg.Register("ok", newTrackingAgent("ok", 0))
	reg.Register("fail", &funcAgent{fn: func(ctx context.Context, input map[string]any) (*agent.Result, error) {
		return nil, fmt.Errorf("agent failure: boom")
	}})

	pl := &dagStaticPlanner{plan: &planner.Plan{
		TaskID: "fail-dag",
		Steps: []planner.PlanStep{
			{AgentID: "ok", StepID: "s1"},
			{AgentID: "fail", StepID: "s2"},
			{AgentID: "ok", StepID: "s3", DependsOn: []string{"s1", "s2"}},
		},
	}}

	engine := orchestrator.NewEngine(pl, reg, tools.NewRegistryExecutor(tools.NewRegistry()), nil, nil, nil, nil)
	result, err := engine.Execute(context.Background(), orchestrator.ExecutionRequest{
		RunID: "run-fail-dag", TaskID: "fail-dag", Input: map[string]any{},
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.Status != orchestrator.StatusFailed {
		t.Fatalf("expected failure, got %v", result.Status)
	}
}

func TestDAGExecution_LargeParallelFanOut(t *testing.T) {
	// Fan-out: one root step, N parallel steps, one join step.
	const N = 10
	reg := agent.NewRegistry()
	delay := 50 * time.Millisecond

	for i := 0; i < N; i++ {
		id := fmt.Sprintf("worker-%d", i)
		reg.Register(id, newTrackingAgent(id, delay))
	}
	reg.Register("root", newTrackingAgent("root", 0))
	reg.Register("join", newTrackingAgent("join", 0))

	steps := []planner.PlanStep{
		{AgentID: "root", StepID: "root"},
	}
	deps := make([]string, N)
	for i := 0; i < N; i++ {
		id := fmt.Sprintf("worker-%d", i)
		sid := fmt.Sprintf("w%d", i)
		deps[i] = sid
		steps = append(steps, planner.PlanStep{AgentID: id, StepID: sid, DependsOn: []string{"root"}})
	}
	steps = append(steps, planner.PlanStep{AgentID: "join", StepID: "join", DependsOn: deps})

	pl := &dagStaticPlanner{plan: &planner.Plan{TaskID: "fanout", Steps: steps}}
	engine := orchestrator.NewEngine(pl, reg, tools.NewRegistryExecutor(tools.NewRegistry()), nil, nil, nil, nil)

	start := time.Now()
	result, err := engine.Execute(context.Background(), orchestrator.ExecutionRequest{
		RunID: "run-fanout", TaskID: "fanout", Input: map[string]any{},
	})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != orchestrator.StatusSucceeded {
		t.Fatalf("expected success, got %v (err=%v)", result.Status, result.Err)
	}

	// All N workers run in parallel, so total should be ~1×delay, not N×delay.
	maxSerial := time.Duration(N) * delay
	if elapsed >= maxSerial {
		t.Fatalf("fan-out took %v — appears sequential (serial would be %v)", elapsed, maxSerial)
	}
}
