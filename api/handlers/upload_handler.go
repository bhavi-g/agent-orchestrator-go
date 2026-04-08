package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// UploadHandler handles log file uploads.
type UploadHandler struct {
	uploadDir string
}

// NewUploadHandler creates a handler that stores uploads under uploadDir.
func NewUploadHandler(uploadDir string) *UploadHandler {
	return &UploadHandler{uploadDir: uploadDir}
}

// UploadResponse is returned after a successful upload.
type UploadResponse struct {
	Directory string `json:"directory"`
	Files     int    `json:"files"`
}

// Upload handles POST /upload — accepts multipart/form-data with log files.
func (h *UploadHandler) Upload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// 10 MB max total
	const maxSize = 10 << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxSize)

	if err := r.ParseMultipartForm(maxSize); err != nil {
		writeError(w, http.StatusBadRequest, "files too large (10 MB max)")
		return
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		writeError(w, http.StatusBadRequest, "no files uploaded")
		return
	}

	// Create a unique directory for this upload
	dirName, err := randomDirName()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create upload directory")
		return
	}
	dest := filepath.Join(h.uploadDir, dirName)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create upload directory")
		return
	}

	saved := 0
	for _, fh := range files {
		if err := saveUploadedFile(fh, dest); err != nil {
			continue
		}
		saved++
	}

	if saved == 0 {
		_ = os.RemoveAll(dest)
		writeError(w, http.StatusBadRequest, "no valid log files uploaded")
		return
	}

	writeJSON(w, http.StatusOK, UploadResponse{
		Directory: dest,
		Files:     saved,
	})
}

// DemoDir handles GET /demo-dir — returns the path to the extracted demo logs.
func (h *UploadHandler) DemoDir(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	// The demo directory is set by main at startup
	writeJSON(w, http.StatusOK, map[string]string{"directory": h.uploadDir + "/_demo"})
}

func saveUploadedFile(fh *multipart.FileHeader, dest string) error {
	// Only allow safe filenames
	name := filepath.Base(fh.Filename)
	if name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("invalid filename")
	}
	// Only allow text/log files
	ext := strings.ToLower(filepath.Ext(name))
	allowed := map[string]bool{".log": true, ".txt": true, ".out": true, ".err": true, ".json": true, ".csv": true, "": true}
	if !allowed[ext] {
		return fmt.Errorf("file type not allowed: %s", ext)
	}

	src, err := fh.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	outPath := filepath.Join(dest, name)
	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Limit individual file to 5MB
	if _, err := io.Copy(out, io.LimitReader(src, 5<<20)); err != nil {
		return err
	}
	return nil
}

func randomDirName() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "upload-" + hex.EncodeToString(b), nil
}

// GetDemoDir returns the demo directory path. Used by the handler.
func GetDemoDir(baseDir string) string {
	return filepath.Join(baseDir, "_demo")
}
