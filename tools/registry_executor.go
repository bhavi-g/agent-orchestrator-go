package tools

import "context"

type RegistryExecutor struct {
	reg *Registry
}

func NewRegistryExecutor(reg *Registry) *RegistryExecutor {
	return &RegistryExecutor{reg: reg}
}

func (e *RegistryExecutor) Execute(ctx context.Context, call Call) (Result, error) {
	tool, err := e.reg.Get(call.ToolName)
	if err != nil {
		return Result{}, err
	}
	return tool.Execute(ctx, call)
}
