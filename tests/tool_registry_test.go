package tests

import (
	"context"
	"errors"
	"testing"

	"agent-orchestrator/tools"
)

type dummyTool struct{ name string }

func (d dummyTool) Name() string { return d.name }

func (d dummyTool) Spec() tools.Spec {
	return tools.Spec{
		Name:        d.name,
		Description: "dummy",
		Input: map[string]tools.Field{
			"msg": {Type: "string", Required: true},
		},
		Output: map[string]tools.Field{
			"echo": {Type: "string", Required: true},
		},
	}
}

func (d dummyTool) Execute(ctx context.Context, call tools.Call) (tools.Result, error) {
	msg, ok := call.Args["msg"].(string)
	if !ok {
		return tools.Result{}, tools.InvalidArgsf("msg must be string")
	}
	return tools.Result{
		ToolName: d.name,
		Data:     map[string]any{"echo": msg},
	}, nil
}

func TestToolRegistry_RegisterGetList(t *testing.T) {
	r := tools.NewRegistry()

	if err := r.Register(dummyTool{name: "echo"}); err != nil {
		t.Fatalf("register: %v", err)
	}

	names := r.List()
	if len(names) != 1 || names[0] != "echo" {
		t.Fatalf("unexpected list: %#v", names)
	}

	got, err := r.Get("echo")
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	res, err := got.Execute(context.Background(), tools.Call{
		ToolName: "echo",
		Args:     map[string]any{"msg": "hi"},
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.Data["echo"] != "hi" {
		t.Fatalf("unexpected result: %#v", res)
	}
}

func TestToolRegistry_DuplicateRejected(t *testing.T) {
	r := tools.NewRegistry()
	if err := r.Register(dummyTool{name: "echo"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	err := r.Register(dummyTool{name: "echo"})
	if err == nil {
		t.Fatalf("expected duplicate error")
	}
	if !errors.Is(err, tools.ErrInvalidArgs) {
		t.Fatalf("expected ErrInvalidArgs, got: %v", err)
	}
}

func TestToolRegistry_NotFound(t *testing.T) {
	r := tools.NewRegistry()
	_, err := r.Get("missing")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, tools.ErrToolNotFound) {
		t.Fatalf("expected ErrToolNotFound, got: %v", err)
	}
}
