package scheduler

import (
	"fmt"
	"sync"
	"time"

	"github.com/dongsheng123132/fastcp/pkg/buffer"
	"github.com/dongsheng123132/fastcp/pkg/copier"
	"github.com/dongsheng123132/fastcp/pkg/scanner"
)

// TargetResult holds the result for one target directory.
type TargetResult struct {
	TargetDir string
	Error     error
	Duration  time.Duration
	Progress  *copier.Progress
	mu        sync.Mutex
}

// Scheduler manages parallel copy operations to multiple targets.
type Scheduler struct {
	scanResult  *scanner.ScanResult
	pool        *buffer.Pool
	targets     []string
	concurrency int
	config      copier.Config
}

func New(sr *scanner.ScanResult, pool *buffer.Pool, targets []string, concurrency int, cfg copier.Config) *Scheduler {
	if concurrency <= 0 {
		concurrency = 3
	}
	if concurrency > len(targets) {
		concurrency = len(targets)
	}
	return &Scheduler{
		scanResult:  sr,
		pool:        pool,
		targets:     targets,
		concurrency: concurrency,
		config:      cfg,
	}
}

// Run executes parallel copy to all targets, calling progressFn periodically.
// progressFn receives the slice of all TargetResult (including in-progress ones).
// Finished returns true if the target has completed (successfully or with error).
func (r *TargetResult) Finished() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.Duration > 0
}

// GetError returns the error (thread-safe).
func (r *TargetResult) GetError() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.Error
}

// GetDuration returns the duration (thread-safe).
func (r *TargetResult) GetDuration() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.Duration
}

func (s *Scheduler) Run(progressFn func(results []TargetResult)) []TargetResult {
	results := make([]TargetResult, len(s.targets))
	for i, t := range s.targets {
		results[i] = TargetResult{
			TargetDir: t,
			Progress:  &copier.Progress{},
		}
	}

	sem := make(chan struct{}, s.concurrency)
	var wg sync.WaitGroup

	// Start progress reporter
	done := make(chan struct{})
	progressDone := make(chan struct{})
	if progressFn != nil {
		go func() {
			defer close(progressDone)
			ticker := time.NewTicker(200 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					progressFn(results)
				case <-done:
					progressFn(results) // final update
					return
				}
			}
		}()
	} else {
		close(progressDone)
	}

	for i := range s.targets {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sem <- struct{}{}        // acquire
			defer func() { <-sem }() // release

			start := time.Now()
			err := copier.CopyToTarget(
				s.scanResult,
				s.pool,
				s.targets[idx],
				s.config,
				results[idx].Progress,
			)
			results[idx].mu.Lock()
			results[idx].Duration = time.Since(start)
			results[idx].Error = err
			results[idx].mu.Unlock()
		}(i)
	}

	wg.Wait()
	close(done)
	<-progressDone // wait for progress goroutine to finish

	return results
}

// FormatSize formats bytes into human-readable string.
func FormatSize(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// FormatSpeed formats bytes/second into human-readable string.
func FormatSpeed(bytesPerSec float64) string {
	return FormatSize(int64(bytesPerSec)) + "/s"
}
