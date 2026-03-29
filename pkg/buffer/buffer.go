package buffer

import (
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/dongsheng123132/fastcp/pkg/scanner"
)

// FileData holds a file's content in memory
type FileData struct {
	Info scanner.FileInfo
	Data []byte // nil for large files (streamed instead)
}

// Pool manages reading source files into memory buffers.
// Small files are fully cached; large files are streamed on demand.
type Pool struct {
	scanResult *scanner.ScanResult
	cache      map[string]*FileData // relPath -> data
	mu         sync.RWMutex
	totalRead  int64
}

func NewPool(sr *scanner.ScanResult) *Pool {
	return &Pool{
		scanResult: sr,
		cache:      make(map[string]*FileData, len(sr.Files)),
	}
}

// PreloadSmallFiles reads all small files into memory.
// This is the key optimization: read source once, write to many targets.
func (p *Pool) PreloadSmallFiles(progressFn func(bytesRead int64)) error {
	smallFiles := p.scanResult.SmallFiles()

	for i := range smallFiles {
		fi := &smallFiles[i]
		data, err := os.ReadFile(fi.AbsPath)
		if err != nil {
			return fmt.Errorf("read %s: %w", fi.RelPath, err)
		}

		p.mu.Lock()
		p.cache[fi.RelPath] = &FileData{
			Info: *fi,
			Data: data,
		}
		p.totalRead += int64(len(data))
		p.mu.Unlock()

		if progressFn != nil {
			progressFn(int64(len(data)))
		}
	}

	return nil
}

// PreloadAll reads ALL files into memory (use when total size fits in RAM).
func (p *Pool) PreloadAll(progressFn func(bytesRead int64)) error {
	for i := range p.scanResult.Files {
		fi := &p.scanResult.Files[i]

		data, err := os.ReadFile(fi.AbsPath)
		if err != nil {
			return fmt.Errorf("read %s: %w", fi.RelPath, err)
		}

		p.mu.Lock()
		p.cache[fi.RelPath] = &FileData{
			Info: *fi,
			Data: data,
		}
		p.totalRead += int64(len(data))
		p.mu.Unlock()

		if progressFn != nil {
			progressFn(int64(len(data)))
		}
	}

	return nil
}

// Get returns cached file data. Returns nil if not cached (large file).
func (p *Pool) Get(relPath string) *FileData {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cache[relPath]
}

// IsCached returns true if the file is in memory cache.
func (p *Pool) IsCached(relPath string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	_, ok := p.cache[relPath]
	return ok
}

// OpenStream opens a file for streaming (large files not in cache).
func (p *Pool) OpenStream(fi *scanner.FileInfo) (io.ReadCloser, error) {
	return os.Open(fi.AbsPath)
}

// TotalCached returns total bytes cached in memory.
func (p *Pool) TotalCached() int64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.totalRead
}

// Clear releases all cached data.
func (p *Pool) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cache = make(map[string]*FileData)
	p.totalRead = 0
}
