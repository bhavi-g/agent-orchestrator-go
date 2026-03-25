package llm

import (
	"encoding/json"
	"fmt"
	"strings"
)

// SchemaFieldType defines what type a field must be.
type SchemaFieldType string

const (
	FieldString SchemaFieldType = "string"
	FieldNumber SchemaFieldType = "number"
	FieldBool   SchemaFieldType = "bool"
	FieldObject SchemaFieldType = "object"
	FieldArray  SchemaFieldType = "array"
)

// SchemaField describes a single field in an expected JSON structure.
type SchemaField struct {
	Name     string
	Type     SchemaFieldType
	Required bool
	// AllowedValues restricts string fields to a fixed set (empty = any).
	AllowedValues []string
}

// Schema describes the expected shape of a JSON value.
type Schema struct {
	// Name identifies the schema (for error messages).
	Name string
	// IsArray means the top-level value must be a JSON array.
	IsArray bool
	// Fields describes required/optional fields on each object.
	Fields []SchemaField
}

// ValidationError records a single schema violation.
type ValidationError struct {
	Path    string
	Message string
}

func (e ValidationError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("%s: %s", e.Path, e.Message)
	}
	return e.Message
}

// ValidateJSON checks raw JSON bytes against the schema.
// It returns all violations found (empty slice = valid).
func (s *Schema) ValidateJSON(data []byte) []ValidationError {
	data = []byte(strings.TrimSpace(string(data)))

	if s.IsArray {
		return s.validateArray(data)
	}
	return s.validateObject(data, "")
}

func (s *Schema) validateArray(data []byte) []ValidationError {
	var arr []json.RawMessage
	if err := json.Unmarshal(data, &arr); err != nil {
		return []ValidationError{{Message: fmt.Sprintf("expected JSON array: %v", err)}}
	}
	if len(arr) == 0 {
		return []ValidationError{{Message: "array must contain at least one element"}}
	}

	var errs []ValidationError
	for i, item := range arr {
		path := fmt.Sprintf("[%d]", i)
		errs = append(errs, s.validateObject(item, path)...)
	}
	return errs
}

func (s *Schema) validateObject(data json.RawMessage, pathPrefix string) []ValidationError {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return []ValidationError{{Path: pathPrefix, Message: fmt.Sprintf("expected JSON object: %v", err)}}
	}

	var errs []ValidationError
	for _, f := range s.Fields {
		path := pathPrefix
		if path != "" {
			path += "."
		}
		path += f.Name

		raw, exists := obj[f.Name]
		if !exists {
			if f.Required {
				errs = append(errs, ValidationError{Path: path, Message: "required field missing"})
			}
			continue
		}

		errs = append(errs, checkType(raw, f, path)...)
	}
	return errs
}

func checkType(raw json.RawMessage, f SchemaField, path string) []ValidationError {
	var errs []ValidationError

	switch f.Type {
	case FieldString:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			errs = append(errs, ValidationError{Path: path, Message: "expected string"})
			return errs
		}
		if s == "" && f.Required {
			errs = append(errs, ValidationError{Path: path, Message: "required field is empty"})
		}
		if len(f.AllowedValues) > 0 {
			found := false
			for _, av := range f.AllowedValues {
				if s == av {
					found = true
					break
				}
			}
			if !found {
				errs = append(errs, ValidationError{Path: path, Message: fmt.Sprintf("value %q not in allowed set %v", s, f.AllowedValues)})
			}
		}

	case FieldNumber:
		var n float64
		if err := json.Unmarshal(raw, &n); err != nil {
			errs = append(errs, ValidationError{Path: path, Message: "expected number"})
		}

	case FieldBool:
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			errs = append(errs, ValidationError{Path: path, Message: "expected bool"})
		}

	case FieldObject:
		var m map[string]json.RawMessage
		if err := json.Unmarshal(raw, &m); err != nil {
			errs = append(errs, ValidationError{Path: path, Message: "expected object"})
		}

	case FieldArray:
		var a []json.RawMessage
		if err := json.Unmarshal(raw, &a); err != nil {
			errs = append(errs, ValidationError{Path: path, Message: "expected array"})
		}
	}

	return errs
}

// PlanStepSchema is the standard schema for LLM-generated plan step arrays.
var PlanStepSchema = &Schema{
	Name:    "PlanStep",
	IsArray: true,
	Fields: []SchemaField{
		{Name: "agent_id", Type: FieldString, Required: true},
		{Name: "input", Type: FieldObject, Required: false},
		{Name: "metadata", Type: FieldObject, Required: false},
		{Name: "step_id", Type: FieldString, Required: false},
		{Name: "depends_on", Type: FieldArray, Required: false},
	},
}
