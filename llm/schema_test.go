package llm

import (
	"testing"
)

func TestSchema_ValidArray(t *testing.T) {
	data := `[{"agent_id": "agent.read", "input": {"path": "/var/log"}}, {"agent_id": "agent.summarize"}]`

	errs := PlanStepSchema.ValidateJSON([]byte(data))
	if len(errs) > 0 {
		t.Errorf("expected no errors, got: %v", errs)
	}
}

func TestSchema_MissingRequiredField(t *testing.T) {
	data := `[{"input": {"path": "/var/log"}}]`

	errs := PlanStepSchema.ValidateJSON([]byte(data))
	if len(errs) == 0 {
		t.Fatal("expected validation errors for missing agent_id")
	}
	found := false
	for _, e := range errs {
		if e.Path == "[0].agent_id" && e.Message == "required field missing" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected missing agent_id error, got: %v", errs)
	}
}

func TestSchema_EmptyAgentID(t *testing.T) {
	data := `[{"agent_id": ""}]`

	errs := PlanStepSchema.ValidateJSON([]byte(data))
	if len(errs) == 0 {
		t.Fatal("expected error for empty agent_id")
	}
}

func TestSchema_EmptyArray(t *testing.T) {
	data := `[]`

	errs := PlanStepSchema.ValidateJSON([]byte(data))
	if len(errs) == 0 {
		t.Fatal("expected error for empty array")
	}
}

func TestSchema_NotJSON(t *testing.T) {
	data := `this is not json`

	errs := PlanStepSchema.ValidateJSON([]byte(data))
	if len(errs) == 0 {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestSchema_WrongType(t *testing.T) {
	data := `[{"agent_id": 123}]`

	errs := PlanStepSchema.ValidateJSON([]byte(data))
	if len(errs) == 0 {
		t.Fatal("expected error for wrong type")
	}
}

func TestSchema_NonArrayTopLevel(t *testing.T) {
	schema := &Schema{
		Name:    "test",
		IsArray: false,
		Fields: []SchemaField{
			{Name: "name", Type: FieldString, Required: true},
			{Name: "count", Type: FieldNumber, Required: false},
		},
	}

	data := `{"name": "hello", "count": 42}`
	errs := schema.ValidateJSON([]byte(data))
	if len(errs) > 0 {
		t.Errorf("expected no errors, got: %v", errs)
	}
}

func TestSchema_AllowedValues(t *testing.T) {
	schema := &Schema{
		Name:    "test",
		IsArray: false,
		Fields: []SchemaField{
			{Name: "status", Type: FieldString, Required: true, AllowedValues: []string{"ok", "fail"}},
		},
	}

	// Valid
	errs := schema.ValidateJSON([]byte(`{"status": "ok"}`))
	if len(errs) > 0 {
		t.Errorf("expected no errors for valid value, got: %v", errs)
	}

	// Invalid
	errs = schema.ValidateJSON([]byte(`{"status": "maybe"}`))
	if len(errs) == 0 {
		t.Fatal("expected error for disallowed value")
	}
}

func TestSchema_BoolField(t *testing.T) {
	schema := &Schema{
		Name: "test",
		Fields: []SchemaField{
			{Name: "active", Type: FieldBool, Required: true},
		},
	}
	errs := schema.ValidateJSON([]byte(`{"active": true}`))
	if len(errs) > 0 {
		t.Errorf("expected no errors, got: %v", errs)
	}

	errs = schema.ValidateJSON([]byte(`{"active": "yes"}`))
	if len(errs) == 0 {
		t.Fatal("expected error for wrong bool type")
	}
}

func TestSchema_ArrayField(t *testing.T) {
	schema := &Schema{
		Name: "test",
		Fields: []SchemaField{
			{Name: "items", Type: FieldArray, Required: true},
		},
	}
	errs := schema.ValidateJSON([]byte(`{"items": [1,2,3]}`))
	if len(errs) > 0 {
		t.Errorf("expected no errors, got: %v", errs)
	}

	errs = schema.ValidateJSON([]byte(`{"items": "not-array"}`))
	if len(errs) == 0 {
		t.Fatal("expected error for wrong array type")
	}
}
