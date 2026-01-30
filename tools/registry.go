package tools

import (
	"fmt"
	"sort"
	"sync"
)

// Registry stores tools by name. It is safe for concurrent reads.
// (We’re not implementing parallel execution yet, but thread-safety is cheap here.)
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds a tool. Duplicate names are rejected deterministically.
func (r *Registry) Register(t Tool) error {
	if t == nil {
		return fmt.Errorf("%w: nil tool", ErrInvalidArgs)
	}
	name := t.Name()
	if name == "" {
		return fmt.Errorf("%w: tool name is empty", ErrInvalidArgs)
	}
	// enforce Spec name match early
	if spec := t.Spec(); spec.Name != "" && spec.Name != name {
		return fmt.Errorf("%w: spec.name (%s) != tool.name (%s)", ErrInvalidArgs, spec.Name, name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("%w: duplicate tool name: %s", ErrInvalidArgs, name)
	}
	r.tools[name] = t
	return nil
}

func (r *Registry) Get(name string) (Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	t, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrToolNotFound, name)
	}
	return t, nil
}

// List returns tool names in stable order (useful for tests and debugging).
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]string, 0, len(r.tools))
	for k := range r.tools {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
