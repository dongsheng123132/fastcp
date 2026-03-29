package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

const SmallFileThreshold = 256 * 1024 // 256KB

type FileInfo struct {
	RelPath  string // relative path from source root
	AbsPath  string // absolute path
	Size     int64
	ModTime  int64
	IsDir    bool
	Mode     os.FileMode
}

type ScanResult struct {
	BaseDir    string
	Files      []FileInfo // all files, sorted: small files first
	Dirs       []FileInfo // all directories
	TotalSize  int64
	TotalFiles int
}

func Scan(srcDir string) (*ScanResult, error) {
	srcDir, err := filepath.Abs(srcDir)
	if err != nil {
		return nil, fmt.Errorf("resolve source path: %w", err)
	}

	info, err := os.Stat(srcDir)
	if err != nil {
		return nil, fmt.Errorf("stat source: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("source is not a directory: %s", srcDir)
	}

	result := &ScanResult{BaseDir: srcDir}

	err = filepath.Walk(srcDir, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		entry := FileInfo{
			RelPath: filepath.ToSlash(rel),
			AbsPath: path,
			Size:    fi.Size(),
			ModTime: fi.ModTime().UnixNano(),
			IsDir:   fi.IsDir(),
			Mode:    fi.Mode(),
		}

		if fi.IsDir() {
			result.Dirs = append(result.Dirs, entry)
		} else {
			result.Files = append(result.Files, entry)
			result.TotalSize += fi.Size()
			result.TotalFiles++
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk source: %w", err)
	}

	// Sort: small files first (better for batching), then large files
	sort.Slice(result.Files, func(i, j int) bool {
		return result.Files[i].Size < result.Files[j].Size
	})

	return result, nil
}

func (r *ScanResult) SmallFiles() []FileInfo {
	var out []FileInfo
	for _, f := range r.Files {
		if f.Size < SmallFileThreshold {
			out = append(out, f)
		}
	}
	return out
}

func (r *ScanResult) LargeFiles() []FileInfo {
	var out []FileInfo
	for _, f := range r.Files {
		if f.Size >= SmallFileThreshold {
			out = append(out, f)
		}
	}
	return out
}
