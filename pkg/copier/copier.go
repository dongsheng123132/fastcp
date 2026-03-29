package copier

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/dongsheng123132/fastcp/pkg/buffer"
	"github.com/dongsheng123132/fastcp/pkg/scanner"
)

// Config holds copier configuration.
type Config struct {
	BufferSize  int
	Incremental bool
	DryRun      bool
}

// Progress tracks copy progress for a single target.
type Progress struct {
	BytesCopied  atomic.Int64
	FilesCopied  atomic.Int64
	FilesSkipped atomic.Int64
}

// CopyToTarget copies all files to a single target directory.
func CopyToTarget(
	sr *scanner.ScanResult,
	pool *buffer.Pool,
	targetDir string,
	cfg Config,
	progress *Progress,
) error {
	w := NewWriter(targetDir, cfg.BufferSize)

	// Create directories first
	if !cfg.DryRun {
		if err := w.EnsureDirs(sr.Dirs); err != nil {
			return fmt.Errorf("create dirs: %w", err)
		}
	}

	// Copy all files
	for i := range sr.Files {
		fi := &sr.Files[i]

		if cfg.Incremental {
			if shouldSkip(fi, targetDir) {
				progress.FilesSkipped.Add(1)
				continue
			}
		}

		if cfg.DryRun {
			progress.FilesCopied.Add(1)
			progress.BytesCopied.Add(fi.Size)
			continue
		}

		// Try memory-cached copy first
		cached := pool.Get(fi.RelPath)
		if cached != nil {
			err := w.WriteFromMemory(fi.RelPath, cached.Data, fi.Mode, fi.ModTime)
			if err != nil {
				return fmt.Errorf("write %s to %s: %w", fi.RelPath, targetDir, err)
			}
			progress.BytesCopied.Add(int64(len(cached.Data)))
		} else {
			// Stream from disk (large file not in cache)
			r, err := pool.OpenStream(fi)
			if err != nil {
				return fmt.Errorf("open %s: %w", fi.RelPath, err)
			}
			n, err := w.WriteFromStream(fi.RelPath, r, fi.Mode, fi.ModTime)
			if cerr := r.Close(); cerr != nil && err == nil {
				err = cerr
			}
			if err != nil {
				return fmt.Errorf("stream %s to %s: %w", fi.RelPath, targetDir, err)
			}
			progress.BytesCopied.Add(n)
		}

		progress.FilesCopied.Add(1)
	}

	return nil
}

// shouldSkip checks if a file should be skipped in incremental mode.
func shouldSkip(fi *scanner.FileInfo, targetDir string) bool {
	targetPath := filepath.Join(targetDir, filepath.FromSlash(fi.RelPath))
	targetInfo, err := os.Stat(targetPath)
	if err != nil {
		return false // file doesn't exist, need to copy
	}

	// Skip if same size and target is not older.
	// Compare at 2-second granularity for FAT32/exFAT compatibility.
	return targetInfo.Size() == fi.Size &&
		targetInfo.ModTime().Unix() >= (fi.ModTime/1e9)-2
}
