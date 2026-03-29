package verify

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/cespare/xxhash/v2"
	"github.com/dongsheng123132/fastcp/pkg/scanner"
)

// Result holds verification results.
type Result struct {
	TotalFiles   int
	MatchedFiles int
	Mismatched   []MismatchInfo
	Errors       []ErrorInfo
}

type MismatchInfo struct {
	RelPath    string
	SourceHash uint64
	TargetHash uint64
}

type ErrorInfo struct {
	RelPath string
	Error   string
}

// VerifyProgress tracks verification progress.
type VerifyProgress struct {
	FilesChecked atomic.Int64
	TotalFiles   int
}

// Verify compares files between source and a target directory using xxhash.
func Verify(sr *scanner.ScanResult, targetDir string, progress *VerifyProgress) (*Result, error) {
	result := &Result{TotalFiles: len(sr.Files)}
	progress.TotalFiles = len(sr.Files)

	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8) // limit concurrent file reads

	for i := range sr.Files {
		fi := &sr.Files[i]
		wg.Add(1)
		go func(fi *scanner.FileInfo) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			srcHash, err := hashFile(fi.AbsPath)
			if err != nil {
				mu.Lock()
				result.Errors = append(result.Errors, ErrorInfo{
					RelPath: fi.RelPath,
					Error:   fmt.Sprintf("source: %v", err),
				})
				mu.Unlock()
				progress.FilesChecked.Add(1)
				return
			}

			targetPath := filepath.Join(targetDir, filepath.FromSlash(fi.RelPath))
			tgtHash, err := hashFile(targetPath)
			if err != nil {
				mu.Lock()
				result.Errors = append(result.Errors, ErrorInfo{
					RelPath: fi.RelPath,
					Error:   fmt.Sprintf("target: %v", err),
				})
				mu.Unlock()
				progress.FilesChecked.Add(1)
				return
			}

			mu.Lock()
			if srcHash == tgtHash {
				result.MatchedFiles++
			} else {
				result.Mismatched = append(result.Mismatched, MismatchInfo{
					RelPath:    fi.RelPath,
					SourceHash: srcHash,
					TargetHash: tgtHash,
				})
			}
			mu.Unlock()

			progress.FilesChecked.Add(1)
		}(fi)
	}

	wg.Wait()
	return result, nil
}

// VerifyFromMemory compares cached data with target files (faster, no re-read of source).
func VerifyFromMemory(data []byte, targetPath string) (bool, error) {
	srcHash := xxhash.Sum64(data)

	tgtData, err := os.ReadFile(targetPath)
	if err != nil {
		return false, err
	}

	return srcHash == xxhash.Sum64(tgtData), nil
}

func hashFile(path string) (uint64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	h := xxhash.New()
	buf := make([]byte, 1024*1024) // 1MB buffer
	if _, err := io.CopyBuffer(h, f, buf); err != nil {
		return 0, err
	}

	return h.Sum64(), nil
}

// QuickVerify does a fast size-only check (no hash).
func QuickVerify(sr *scanner.ScanResult, targetDir string) (matched, mismatched, missing int) {
	for _, fi := range sr.Files {
		targetPath := filepath.Join(targetDir, filepath.FromSlash(fi.RelPath))
		info, err := os.Stat(targetPath)
		if err != nil {
			missing++
			continue
		}
		if info.Size() == fi.Size {
			matched++
		} else {
			mismatched++
		}
	}
	return
}
