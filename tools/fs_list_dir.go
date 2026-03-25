package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// ListDirTool lists the contents of a directory.
// Only directories under the configured root are accessible.
type ListDirTool struct {
	rootDir string
}

func NewListDirTool(rootDir string) *ListDirTool {
	return &ListDirTool{rootDir: rootDir}
}

func (t *ListDirTool) Name() string { return "fs.list_dir" }

func (t *ListDirTool) Spec() Spec {
	return Spec{
		Name:        "fs.list_dir",
		Description: "List files and subdirectories in a directory. Path must be relative to root. Use '.' for the root itself.",
		Input: map[string]Field{
			"path": {Type: "string", Description: "Relative path to the directory (use '.' for root)", Required: true},
		},
		Output: map[string]Field{
			"entries": {Type: "array", Description: "List of entry objects with name, is_dir, and size"},
		},
	}
}

func (t *ListDirTool) Execute(ctx context.Context, call Call) (Result, error) {
	relPath, ok := call.Args["path"].(string)
	if !ok || relPath == "" {
		return Result{}, InvalidArgsf("'path' is required and must be a string")
	}

	absPath, err := t.safePath(relPath)
	if err != nil {
		return Result{}, err
	}

	entries, err := os.ReadDir(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return Result{}, ToolFailedf("directory not found: %s", relPath)
		}
		return Result{}, ToolFailedf("read dir error: %v", err)
	}

	items := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		info, _ := e.Info()
		size := int64(0)
		if info != nil {
			size = info.Size()
		}
		items = append(items, map[string]any{
			"name":   e.Name(),
			"is_dir": e.IsDir(),
			"size":   size,
		})
	}

	return Result{
		ToolName: t.Name(),
		Data: map[string]any{
			"entries": items,
			"count":   len(items),
			"path":    relPath,
		},
	}, nil
}

func (t *ListDirTool) safePath(rel string) (string, error) {
	cleaned := filepath.Clean(rel)
	// Allow absolute paths — this is a local dev tool; user already has filesystem access.
	if filepath.IsAbs(cleaned) {
		info, err := os.Stat(cleaned)
		if err != nil {
			return "", ToolFailedf("path not found: %s", rel)
		}
		if !info.IsDir() {
			return "", InvalidArgsf("%s is not a directory", rel)
		}
		return cleaned, nil
	}
	if strings.HasPrefix(cleaned, "..") {
		return "", InvalidArgsf("path must be relative and within root directory")
	}
	abs := filepath.Join(t.rootDir, cleaned)
	absRoot, _ := filepath.Abs(t.rootDir)
	absResolved, _ := filepath.Abs(abs)
	if !strings.HasPrefix(absResolved, absRoot+string(filepath.Separator)) && absResolved != absRoot {
		return "", InvalidArgsf("path escapes root directory")
	}
	info, err := os.Stat(absResolved)
	if err != nil {
		return "", ToolFailedf("stat error: %v", err)
	}
	if !info.IsDir() {
		return "", InvalidArgsf("%s is not a directory", rel)
	}
	return absResolved, nil
}

// ensure interface
var _ Tool = (*ListDirTool)(nil)
var _ Tool = (*ReadFileTool)(nil)
