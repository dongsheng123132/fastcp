package copier

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/dongsheng123132/fastcp/pkg/scanner"
)

// Writer handles writing files to a single target directory.
type Writer struct {
	targetDir  string
	bufferSize int
}

func NewWriter(targetDir string, bufferSize int) *Writer {
	return &Writer{
		targetDir:  targetDir,
		bufferSize: bufferSize,
	}
}

// EnsureDirs creates all necessary directories in the target.
func (w *Writer) EnsureDirs(dirs []scanner.FileInfo) error {
	for _, d := range dirs {
		targetPath := filepath.Join(w.targetDir, filepath.FromSlash(d.RelPath))
		if err := os.MkdirAll(targetPath, d.Mode|0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", d.RelPath, err)
		}
	}
	return nil
}

// WriteFromMemory writes a file from in-memory data.
func (w *Writer) WriteFromMemory(relPath string, data []byte, mode os.FileMode, modTime int64) error {
	targetPath := filepath.Join(w.targetDir, filepath.FromSlash(relPath))

	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir for %s: %w", relPath, err)
	}

	f, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("create %s: %w", relPath, err)
	}

	_, err = f.Write(data)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(targetPath)
		return fmt.Errorf("write %s: %w", relPath, err)
	}

	// Restore modification time
	t := time.Unix(0, modTime)
	os.Chtimes(targetPath, t, t)

	return nil
}

// WriteFromStream copies a file from a reader (for large files).
func (w *Writer) WriteFromStream(relPath string, r io.Reader, mode os.FileMode, modTime int64) (int64, error) {
	targetPath := filepath.Join(w.targetDir, filepath.FromSlash(relPath))

	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return 0, fmt.Errorf("mkdir for %s: %w", relPath, err)
	}

	f, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return 0, fmt.Errorf("create %s: %w", relPath, err)
	}

	bw := bufio.NewWriterSize(f, w.bufferSize)
	n, err := io.Copy(bw, r)
	if err == nil {
		err = bw.Flush()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(targetPath)
		return n, fmt.Errorf("write %s: %w", relPath, err)
	}

	t := time.Unix(0, modTime)
	os.Chtimes(targetPath, t, t)

	return n, nil
}
