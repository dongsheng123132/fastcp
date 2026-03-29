package main

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/dongsheng123132/fastcp/pkg/buffer"
	"github.com/dongsheng123132/fastcp/pkg/copier"
	"github.com/dongsheng123132/fastcp/pkg/scanner"
	"github.com/dongsheng123132/fastcp/pkg/scheduler"
	"github.com/dongsheng123132/fastcp/pkg/verify"
	"github.com/spf13/cobra"
)

var (
	version = "dev"

	concurrency int
	bufferSize  string
	doVerify    bool
	incremental bool
	dryRun      bool
	verbose     bool
	preloadAll  bool
	jsonOutput  bool
	quiet       bool
)

// JSON output structures
type jsonResult struct {
	OK      bool           `json:"ok"`
	Source  string         `json:"source"`
	Stats  jsonStats      `json:"stats"`
	Targets []jsonTarget  `json:"targets"`
	Elapsed string        `json:"elapsed"`
}

type jsonStats struct {
	Files     int   `json:"files"`
	Dirs      int   `json:"dirs"`
	TotalBytes int64 `json:"total_bytes"`
}

type jsonTarget struct {
	Path        string `json:"path"`
	OK          bool   `json:"ok"`
	Error       string `json:"error,omitempty"`
	FilesCopied int64  `json:"files_copied"`
	BytesCopied int64  `json:"bytes_copied"`
	Skipped     int64  `json:"skipped,omitempty"`
	Speed       string `json:"speed"`
	Duration    string `json:"duration"`
	Verified    *bool  `json:"verified,omitempty"`
}

func main() {
	rootCmd := &cobra.Command{
		Use:   "fastcp <source> <target1> [target2] ...",
		Short: "multi-target parallel file copy",
		Long:  "Read source into memory once, write to multiple targets in parallel.",
		Args:  cobra.MinimumNArgs(2),
		RunE:  runCopy,
		SilenceUsage: true,
	}

	rootCmd.Flags().IntVarP(&concurrency, "concurrency", "c", 3, "simultaneous target writes")
	rootCmd.Flags().StringVarP(&bufferSize, "buffer-size", "b", "4M", "write buffer size")
	rootCmd.Flags().BoolVar(&doVerify, "verify", false, "xxhash verify after copy")
	rootCmd.Flags().BoolVar(&incremental, "incremental", false, "skip unchanged files")
	rootCmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview only")
	rootCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.Flags().BoolVar(&preloadAll, "preload-all", false, "preload all files into memory")
	rootCmd.Flags().BoolVar(&jsonOutput, "json", false, "JSON output (for AI/scripts)")
	rootCmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "minimal output")

	rootCmd.Version = version

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func log(format string, a ...interface{}) {
	if !jsonOutput && !quiet {
		fmt.Fprintf(os.Stderr, format+"\n", a...)
	}
}

func logv(format string, a ...interface{}) {
	if verbose && !jsonOutput && !quiet {
		fmt.Fprintf(os.Stderr, format+"\n", a...)
	}
}

func runCopy(cmd *cobra.Command, args []string) error {
	srcDir := args[0]
	targets := args[1:]

	bufSize, err := parseSize(bufferSize)
	if err != nil {
		return fmt.Errorf("invalid buffer-size: %w", err)
	}

	// scan
	log("scan %s", srcDir)
	scanStart := time.Now()
	sr, err := scanner.Scan(srcDir)
	if err != nil {
		return fmt.Errorf("scan: %w", err)
	}
	log("  %d files, %d dirs, %s [%v]",
		sr.TotalFiles, len(sr.Dirs),
		scheduler.FormatSize(sr.TotalSize),
		time.Since(scanStart).Round(time.Millisecond),
	)

	if verbose {
		smallFiles := sr.SmallFiles()
		largeFiles := sr.LargeFiles()
		var smallTotal, largeTotal int64
		for _, f := range smallFiles {
			smallTotal += f.Size
		}
		for _, f := range largeFiles {
			largeTotal += f.Size
		}
		logv("  small(<256K): %d (%s)", len(smallFiles), scheduler.FormatSize(smallTotal))
		logv("  large(>=256K): %d (%s)", len(largeFiles), scheduler.FormatSize(largeTotal))
	}

	if dryRun {
		log("dry-run: would copy to %d targets", len(targets))
		for _, t := range targets {
			log("  -> %s", t)
		}
		return nil
	}

	// preload
	pool := buffer.NewPool(sr)
	loadStart := time.Now()
	if preloadAll || sr.TotalSize <= 4*1024*1024*1024 {
		err = pool.PreloadAll(nil)
	} else {
		err = pool.PreloadSmallFiles(nil)
	}
	if err != nil {
		return fmt.Errorf("preload: %w", err)
	}
	log("cache %s [%v]", scheduler.FormatSize(pool.TotalCached()),
		time.Since(loadStart).Round(time.Millisecond))

	runtime.GC()

	// copy
	log("copy -> %d targets (concurrency=%d)", len(targets), concurrency)
	for _, t := range targets {
		logv("  %s", t)
	}

	cfg := copier.Config{
		BufferSize:  bufSize,
		Incremental: incremental,
	}

	sched := scheduler.New(sr, pool, targets, concurrency, cfg)
	copyStart := time.Now()

	var progressFn func([]scheduler.TargetResult)
	if !jsonOutput && !quiet {
		progressFn = func(results []scheduler.TargetResult) {
			printProgress(results, sr.TotalSize, sr.TotalFiles, copyStart)
		}
	}

	results := sched.Run(progressFn)
	copyDuration := time.Since(copyStart)

	// clear progress line
	if !jsonOutput && !quiet {
		fmt.Fprintf(os.Stderr, "\r\033[K")
	}

	// verify
	type verifyResult struct {
		ok *bool
	}
	verifyResults := make([]verifyResult, len(results))

	if doVerify {
		log("verify")
		for i, r := range results {
			if r.Error != nil {
				continue
			}
			vp := &verify.VerifyProgress{}
			vr, err := verify.Verify(sr, r.TargetDir, vp)
			if err != nil {
				log("  %s: ERR %v", r.TargetDir, err)
				b := false
				verifyResults[i].ok = &b
				continue
			}
			ok := len(vr.Mismatched) == 0 && len(vr.Errors) == 0
			verifyResults[i].ok = &ok
			if ok {
				log("  %s: OK %d/%d", r.TargetDir, vr.MatchedFiles, vr.TotalFiles)
			} else {
				log("  %s: FAIL", r.TargetDir)
				for _, m := range vr.Mismatched {
					log("    DIFF %s", m.RelPath)
				}
				for _, e := range vr.Errors {
					log("    ERR %s: %s", e.RelPath, e.Error)
				}
			}
		}
	}

	// output
	allOK := true
	if jsonOutput {
		jr := jsonResult{
			OK:     true,
			Source: srcDir,
			Stats: jsonStats{
				Files:      sr.TotalFiles,
				Dirs:       len(sr.Dirs),
				TotalBytes: sr.TotalSize,
			},
			Elapsed: copyDuration.Round(time.Millisecond).String(),
		}
		for i, r := range results {
			bytesCopied := r.Progress.BytesCopied.Load()
			speed := float64(0)
			if r.Duration.Seconds() > 0 {
				speed = float64(bytesCopied) / r.Duration.Seconds()
			}
			jt := jsonTarget{
				Path:        r.TargetDir,
				OK:          r.Error == nil,
				FilesCopied: r.Progress.FilesCopied.Load(),
				BytesCopied: bytesCopied,
				Skipped:     r.Progress.FilesSkipped.Load(),
				Speed:       scheduler.FormatSpeed(speed),
				Duration:    r.Duration.Round(time.Millisecond).String(),
				Verified:    verifyResults[i].ok,
			}
			if r.Error != nil {
				jt.Error = r.Error.Error()
				jr.OK = false
				allOK = false
			}
			jr.Targets = append(jr.Targets, jt)
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(jr)
	} else {
		// Unix-style summary to stdout
		for _, r := range results {
			bytesCopied := r.Progress.BytesCopied.Load()
			speed := float64(0)
			if r.Duration.Seconds() > 0 {
				speed = float64(bytesCopied) / r.Duration.Seconds()
			}

			status := "OK"
			if r.Error != nil {
				status = "FAIL"
				allOK = false
			}

			line := fmt.Sprintf("%s\t%s\t%d files\t%s\t%s\t%s",
				status,
				r.TargetDir,
				r.Progress.FilesCopied.Load(),
				scheduler.FormatSize(bytesCopied),
				r.Duration.Round(time.Millisecond),
				scheduler.FormatSpeed(speed),
			)

			if incremental {
				skipped := r.Progress.FilesSkipped.Load()
				if skipped > 0 {
					line += fmt.Sprintf("\t(%d skipped)", skipped)
				}
			}

			if verifyResults[len(results)-1].ok != nil {
				// find this target's verify result
				for j, vr := range verifyResults {
					if results[j].TargetDir == r.TargetDir && vr.ok != nil {
						if *vr.ok {
							line += "\tverified"
						} else {
							line += "\tverify-fail"
						}
					}
				}
			}

			fmt.Println(line)

			if r.Error != nil {
				fmt.Fprintf(os.Stderr, "fastcp: %s: %v\n", r.TargetDir, r.Error)
			}
		}

		log("total: %v", copyDuration.Round(time.Millisecond))
	}

	if !allOK {
		return fmt.Errorf("some targets failed")
	}
	return nil
}

func printProgress(results []scheduler.TargetResult, totalSize int64, totalFiles int, start time.Time) {
	var totalCopied int64
	for _, r := range results {
		totalCopied += r.Progress.BytesCopied.Load()
	}

	elapsed := time.Since(start).Seconds()
	speed := float64(0)
	if elapsed > 0 {
		speed = float64(totalCopied) / elapsed
	}

	totalExpected := totalSize * int64(len(results))
	pct := float64(0)
	if totalExpected > 0 {
		pct = float64(totalCopied) / float64(totalExpected) * 100
	}

	fmt.Fprintf(os.Stderr, "\r\033[K  %.0f%% %s/%s %s ",
		pct,
		scheduler.FormatSize(totalCopied),
		scheduler.FormatSize(totalExpected),
		scheduler.FormatSpeed(speed),
	)

	for i, r := range results {
		if i > 0 {
			fmt.Fprint(os.Stderr, " ")
		}
		fc := r.Progress.FilesCopied.Load()
		if r.GetError() != nil {
			fmt.Fprint(os.Stderr, "X")
		} else if fc >= int64(totalFiles) {
			fmt.Fprint(os.Stderr, "*")
		} else {
			fmt.Fprintf(os.Stderr, "%d/%d", fc, totalFiles)
		}
	}
}

func parseSize(s string) (int, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	multiplier := 1
	if strings.HasSuffix(s, "K") {
		multiplier = 1024
		s = s[:len(s)-1]
	} else if strings.HasSuffix(s, "M") {
		multiplier = 1024 * 1024
		s = s[:len(s)-1]
	} else if strings.HasSuffix(s, "G") {
		multiplier = 1024 * 1024 * 1024
		s = s[:len(s)-1]
	}

	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	if err != nil {
		return 0, fmt.Errorf("cannot parse %q as size", s)
	}
	if n <= 0 {
		return 0, fmt.Errorf("size must be positive, got %d", n)
	}
	return n * multiplier, nil
}
