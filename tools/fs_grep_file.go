package tools

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"
)

// GrepFileTool searches a file for lines matching a substring or keyword.
// Useful for scanning log files for errors/warnings.
type GrepFileTool struct {
	rootDir string
}

func NewGrepFileTool(rootDir string) *GrepFileTool {
	return &GrepFileTool{rootDir: rootDir}
}

func (t *GrepFileTool) Name() string { return "fs.grep_file" }

func (t *GrepFileTool) Spec() Spec {
	return Spec{
		Name:        "fs.grep_file",
		Description: "Search a file for lines containing a keyword (case-insensitive). Returns matching lines with line numbers.",
		Input: map[string]Field{
			"path":    {Type: "string", Description: "Relative path to the file", Required: true},
			"keyword": {Type: "string", Description: "Substring to search for (case-insensitive)", Required: true},
		},
		Output: map[string]Field{
			"matches":     {Type: "array", Description: "Matching lines with line numbers"},
			"match_count": {Type: "number", Description: "Number of matches"},
		},
	}
}

const maxGrepMatches = 200

func (t *GrepFileTool) Execute(ctx context.Context, call Call) (Result, error) {
	relPath, ok := call.Args["path"].(string)
	if !ok || relPath == "" {
		return Result{}, InvalidArgsf("'path' is required and must be a string")
	}
	keyword, ok := call.Args["keyword"].(string)
	if !ok || keyword == "" {
		return Result{}, InvalidArgsf("'keyword' is required and must be a string")
	}

	absPath, err := t.safePath(relPath)
	if err != nil {
		return Result{}, err
	}

	f, err := os.Open(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return Result{}, ToolFailedf("file not found: %s", relPath)
		}
		return Result{}, ToolFailedf("open error: %v", err)
	}
	defer f.Close()

	lowerKW := strings.ToLower(keyword)
	var matches []map[string]any
	scanner := bufio.NewScanner(f)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if strings.Contains(strings.ToLower(line), lowerKW) {
			matches = append(matches, map[string]any{
				"line_number": lineNum,
				"text":        line,
			})
			if len(matches) >= maxGrepMatches {
				break
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return Result{}, ToolFailedf("scan error: %v", err)
	}

	return Result{
		ToolName: t.Name(),
		Data: map[string]any{
			"matches":     matches,
			"match_count": len(matches),
			"path":        relPath,
			"keyword":     keyword,
		},
	}, nil
}

func (t *GrepFileTool) safePath(rel string) (string, error) {
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
	absRoot, _ := filepath.Abs(t.rootDir)
	absResolved, _ := filepath.Abs(abs)
	if !strings.HasPrefix(absResolved, absRoot+string(filepath.Separator)) && absResolved != absRoot {
		return "", InvalidArgsf("path escapes root directory")
	}
	return absResolved, nil
}

var _ Tool = (*GrepFileTool)(nil)
