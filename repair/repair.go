package repair

import "agent-orchestrator/failure"

// ActionType defines the type of repair action to take
type ActionType string

const (
	// RetryStep retries the same step with same input
	RetryStep ActionType = "retry_step"
	// ModifyInput retries step with modified input
	ModifyInput ActionType = "modify_input"
	// InsertStep inserts a new step before/after current
	InsertStep ActionType = "insert_step"
	// ReplaceStep replaces the current step with a different one
	ReplaceStep ActionType = "replace_step"
	// SkipStep skips the current step and continues
	SkipStep ActionType = "skip_step"
	// Replan requests a full replan from the planner
	Replan ActionType = "replan"
	// Abort terminates the execution (non-recoverable)
	Abort ActionType = "abort"
)

// RepairAction represents a single action to take to repair a failure
type RepairAction struct {
	// Type of repair action
	Type ActionType
	// StepIndex that this action applies to (0-based)
	StepIndex int
	// AgentID to use (for InsertStep, ReplaceStep)
	AgentID string
	// ModifiedInput for ModifyInput action
	ModifiedInput map[string]any
	// Metadata for additional context
	Metadata map[string]any
}

// RepairPlan contains the list of actions to repair a failure
type RepairPlan struct {
	// FailureEvent that triggered this repair
	Failure *failure.FailureEvent
	// Actions to execute in order
	Actions []RepairAction
	// ShouldReplan indicates if full replanning is needed
	ShouldReplan bool
	// Reasoning explains why this repair plan was chosen
	Reasoning string
}

// NewRepairPlan creates a new repair plan for a failure
func NewRepairPlan(f *failure.FailureEvent) *RepairPlan {
	return &RepairPlan{
		Failure: f,
		Actions: []RepairAction{},
	}
}

// AddAction adds a repair action to the plan
func (rp *RepairPlan) AddAction(action RepairAction) {
	rp.Actions = append(rp.Actions, action)
}

// WithReasoning sets the reasoning for the repair plan
func (rp *RepairPlan) WithReasoning(reasoning string) *RepairPlan {
	rp.Reasoning = reasoning
	return rp
}

// IsEmpty returns true if the repair plan has no actions
func (rp *RepairPlan) IsEmpty() bool {
	return len(rp.Actions) == 0
}

// HasReplan returns true if any action is a Replan
func (rp *RepairPlan) HasReplan() bool {
	for _, action := range rp.Actions {
		if action.Type == Replan {
			return true
		}
	}
	return rp.ShouldReplan
}

// HasAbort returns true if any action is an Abort
func (rp *RepairPlan) HasAbort() bool {
	for _, action := range rp.Actions {
		if action.Type == Abort {
			return true
		}
	}
	return false
}
