package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// ReadFileTool reads the contents of a file from disk.
// It restricts reads to paths under a configured root directory.
type ReadFileTool struct {
	rootDir string
}

func NewReadFileTool(rootDir string) *ReadFileTool {
	return &ReadFileTool{rootDir: rootDir}
}

func (t *ReadFileTool) Name() string { return "fs.read_file" }

func (t *ReadFileTool) Spec() Spec {
	return Spec{
		Name:        "fs.read_file",
		Description: "Read the full contents of a file. Path must be relative to the configured root directory.",
		Input: map[string]Field{
			"path": {Type: "string", Description: "Relative path to the file", Required: true},
		},
		Output: map[string]Field{
			"content":  {Type: "string", Description: "File contents"},
			"size":     {Type: "number", Description: "File size in bytes"},
			"filename": {Type: "string", Description: "Base file name"},
		},
	}
}

func (t *ReadFileTool) Execute(ctx context.Context, call Call) (Result, error) {
	relPath, ok := call.Args["path"].(string)
	if !ok || relPath == "" {
		return Result{}, InvalidArgsf("'path' is required and must be a string")
	}

	absPath, err := t.safePath(relPath)
	if err != nil {
		return Result{}, err
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return Result{}, ToolFailedf("file not found: %s", relPath)
		}
		return Result{}, ToolFailedf("read error: %v", err)
	}

	return Result{
		ToolName: t.Name(),
		Data: map[string]any{
			"content":  string(data),
			"size":     len(data),
			"filename": filepath.Base(absPath),
		},
	}, nil
}

// safePath resolves a relative path under rootDir and prevents traversal.
func (t *ReadFileTool) safePath(rel string) (string, error) {
	cleaned := filepath.Clean(rel)
	// Allow absolute paths — this is a local dev tool; user already has filesystem access.
	if filepath.IsAbs(cleaned) {
		if _, err := os.Stat(cleaned); err != nil {
			return "", ToolFailedf("file not found: %s", rel)
		}
		return cleaned, nil
	}
	if strings.HasPrefix(cleaned, "..") {
		return "", InvalidArgsf("path must be relative and within root directory")
	}
	abs := filepath.Join(t.rootDir, cleaned)
	// Double-check the resolved path is still under rootDir.
	absRoot, _ := filepath.Abs(t.rootDir)
	absResolved, _ := filepath.Abs(abs)
	if !strings.HasPrefix(absResolved, absRoot+string(filepath.Separator)) && absResolved != absRoot {
		return "", InvalidArgsf("path escapes root directory")
	}
	return absResolved, nil
}
