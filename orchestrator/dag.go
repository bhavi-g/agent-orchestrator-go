package orchestrator

import (
	"fmt"

	"agent-orchestrator/planner"
)

// dagNode represents a step in the DAG with its dependency information.
type dagNode struct {
	index    int              // index into the plan.Steps slice
	step     planner.PlanStep // the step itself
	inDegree int              // number of unresolved dependencies
}

// dagScheduler manages topological ordering and parallel grouping of plan steps.
type dagScheduler struct {
	nodes    []dagNode
	children map[int][]int // parent index -> child indices
}

// newDAGScheduler builds a DAG from a plan.
// Steps without StepIDs or DependsOn are treated as sequential
// (each depends on the previous step).
func newDAGScheduler(plan *planner.Plan) (*dagScheduler, error) {
	steps := plan.Steps
	if len(steps) == 0 {
		return &dagScheduler{}, nil
	}

	// Check if any step uses DAG fields; if not, fall back to sequential.
	hasDAG := false
	for _, s := range steps {
		if s.StepID != "" || len(s.DependsOn) > 0 {
			hasDAG = true
			break
		}
	}

	nodes := make([]dagNode, len(steps))
	children := make(map[int][]int)

	if !hasDAG {
		// Sequential fallback: step i depends on step i-1.
		for i, s := range steps {
			inDeg := 0
			if i > 0 {
				inDeg = 1
				children[i-1] = append(children[i-1], i)
			}
			nodes[i] = dagNode{index: i, step: s, inDegree: inDeg}
		}
		return &dagScheduler{nodes: nodes, children: children}, nil
	}

	// Build stepID -> index map for DAG mode.
	idToIndex := make(map[string]int, len(steps))
	for i, s := range steps {
		if s.StepID == "" {
			return nil, fmt.Errorf("step %d: StepID required when DAG mode is used", i)
		}
		if _, dup := idToIndex[s.StepID]; dup {
			return nil, fmt.Errorf("step %d: duplicate StepID %q", i, s.StepID)
		}
		idToIndex[s.StepID] = i
	}

	for i, s := range steps {
		inDeg := 0
		for _, dep := range s.DependsOn {
			parentIdx, ok := idToIndex[dep]
			if !ok {
				return nil, fmt.Errorf("step %d (%s): depends on unknown StepID %q", i, s.StepID, dep)
			}
			children[parentIdx] = append(children[parentIdx], i)
			inDeg++
		}
		nodes[i] = dagNode{index: i, step: s, inDegree: inDeg}
	}

	// Cycle detection via Kahn's algorithm (count reachable nodes).
	reachable := 0
	queue := make([]int, 0, len(nodes))
	tempDeg := make([]int, len(nodes))
	for i := range nodes {
		tempDeg[i] = nodes[i].inDegree
		if tempDeg[i] == 0 {
			queue = append(queue, i)
		}
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		reachable++
		for _, child := range children[cur] {
			tempDeg[child]--
			if tempDeg[child] == 0 {
				queue = append(queue, child)
			}
		}
	}
	if reachable != len(nodes) {
		return nil, fmt.Errorf("plan contains a dependency cycle")
	}

	return &dagScheduler{nodes: nodes, children: children}, nil
}

// ReadySteps returns the indices of all steps whose dependencies are satisfied.
// It should be called with the current in-degree state.
func (d *dagScheduler) ReadySteps() []int {
	var ready []int
	for i := range d.nodes {
		if d.nodes[i].inDegree == 0 {
			ready = append(ready, i)
		}
	}
	return ready
}

// MarkDone marks a step as completed and decrements the in-degree of its children.
func (d *dagScheduler) MarkDone(idx int) {
	// Use -1 to signal "already processed"
	d.nodes[idx].inDegree = -1
	for _, child := range d.children[idx] {
		d.nodes[child].inDegree--
	}
}

// Len returns total number of steps.
func (d *dagScheduler) Len() int {
	return len(d.nodes)
}
