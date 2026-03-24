package repair

import (
	"agent-orchestrator/failure"
	"context"
	"fmt"
	"time"
)

// Strategy determines how to create a repair plan for a failure
type Strategy interface {
	// CreateRepairPlan analyzes a failure and produces a repair plan
	CreateRepairPlan(ctx context.Context, f *failure.FailureEvent) (*RepairPlan, error)
}

// Engine coordinates failure repair
type Engine struct {
	strategy    Strategy
	config      RetryConfig
	corrector   *InputCorrector
}

// NewEngine creates a new repair engine with the given strategy
func NewEngine(strategy Strategy, maxAttempts int) *Engine {
	config := DefaultRetryConfig()
	if maxAttempts > 0 {
		config.MaxAttempts = maxAttempts
	}
	return &Engine{
		strategy:  strategy,
		config:    config,
		corrector: NewInputCorrector(),
	}
}

// NewEngineWithConfig creates a new repair engine with custom retry config
func NewEngineWithConfig(strategy Strategy, config RetryConfig) *Engine {
	return &Engine{
		strategy:  strategy,
		config:    config,
		corrector: NewInputCorrector(),
	}
}

// Repair analyzes a failure and produces a repair plan
func (e *Engine) Repair(ctx context.Context, f *failure.FailureEvent) (*RepairPlan, error) {
	// Check if we've exceeded max attempts
	if !e.config.ShouldRetry(f.Attempt) {
		plan := NewRepairPlan(f)
		plan.AddAction(RepairAction{
			Type:      Abort,
			StepIndex: f.StepIndex,
		})
		plan.WithReasoning(fmt.Sprintf("exceeded max attempts (%d)", e.config.MaxAttempts))
		return plan, nil
	}

	// Check if failure is retryable
	if !f.IsRetryable() {
		plan := NewRepairPlan(f)
		plan.AddAction(RepairAction{
			Type:      Abort,
			StepIndex: f.StepIndex,
		})
		plan.WithReasoning("failure type is not retryable")
		return plan, nil
	}

	// Use strategy to create repair plan
	return e.strategy.CreateRepairPlan(ctx, f)
}

// GetRetryDelay returns the delay before the next retry
func (e *Engine) GetRetryDelay(attempt int) time.Duration {
	return e.config.CalculateDelay(attempt)
}

// SimpleRetryStrategy is a basic strategy that just retries failed steps
type SimpleRetryStrategy struct{}

// NewSimpleRetryStrategy creates a basic retry strategy
func NewSimpleRetryStrategy() *SimpleRetryStrategy {
	return &SimpleRetryStrategy{}
}

// CreateRepairPlan creates a simple retry repair plan
func (s *SimpleRetryStrategy) CreateRepairPlan(ctx context.Context, f *failure.FailureEvent) (*RepairPlan, error) {
	plan := NewRepairPlan(f)

	switch f.Type {
	case failure.ValidationFailure:
		// For validation failures, we might want to replan
		plan.ShouldReplan = true
		plan.AddAction(RepairAction{
			Type:      Replan,
			StepIndex: f.StepIndex,
		})
		plan.WithReasoning("validation failed - requesting replan")

	case failure.ToolFailure, failure.AgentFailure:
		// For tool/agent failures, simple retry
		plan.AddAction(RepairAction{
			Type:      RetryStep,
			StepIndex: f.StepIndex,
		})
		plan.WithReasoning(fmt.Sprintf("%s - retrying step", f.Type))

	default:
		// Unknown failures should abort
		plan.AddAction(RepairAction{
			Type:      Abort,
			StepIndex: f.StepIndex,
		})
		plan.WithReasoning("unknown failure type")
	}

	return plan, nil
}

// IntelligentStrategy is a placeholder for future LLM-based repair strategy
type IntelligentStrategy struct {
	// Future: add LLM client, context analyzer, etc.
}

// NewIntelligentStrategy creates an intelligent repair strategy
func NewIntelligentStrategy() *IntelligentStrategy {
	return &IntelligentStrategy{}
}

// CreateRepairPlan analyzes failure deeply and creates sophisticated repair plans
func (s *IntelligentStrategy) CreateRepairPlan(ctx context.Context, f *failure.FailureEvent) (*RepairPlan, error) {
	// TODO: Implement LLM-based repair planning
	// For now, fall back to simple retry
	simple := NewSimpleRetryStrategy()
	return simple.CreateRepairPlan(ctx, f)
}
