package agent

import "fmt"

type Registry struct {
	agents map[string]Agent
}

func NewRegistry() *Registry {
	return &Registry{
		agents: make(map[string]Agent),
	}
}

func (r *Registry) Register(id string, agent Agent) {
	r.agents[id] = agent
}

func (r *Registry) Get(id string) (Agent, error) {
	agent, ok := r.agents[id]
	if !ok {
		return nil, fmt.Errorf("agent not found: %s", id)
	}
	return agent, nil
}
