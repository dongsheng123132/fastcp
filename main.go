package main

import (
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
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "fastcp <source> <target1> [target2] [target3] ...",
		Short: "FastCP - cross-platform multi-target fast copy tool",
		Long: `FastCP reads source files into memory once, then writes to multiple targets in parallel.
Optimized for copying to multiple USB drives on the same hub.`,
		Args: cobra.MinimumNArgs(2),
		RunE: runCopy,
	}

	rootCmd.Flags().IntVarP(&concurrency, "concurrency", "c", 3, "number of targets to write simultaneously")
	rootCmd.Flags().StringVarP(&bufferSize, "buffer-size", "b", "4M", "write buffer size (e.g. 1M, 4M, 8M)")
	rootCmd.Flags().BoolVar(&doVerify, "verify", false, "verify copies with xxhash after completion")
	rootCmd.Flags().BoolVar(&incremental, "incremental", false, "skip files that already exist with same size/time")
	rootCmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview only, do not copy")
	rootCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.Flags().BoolVar(&preloadAll, "preload-all", false, "preload ALL files into memory (default: only small files <256KB)")

	rootCmd.Version = version

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runCopy(cmd *cobra.Command, args []string) error {
	srcDir := args[0]
	targets := args[1:]

	bufSize, err := parseSize(bufferSize)
	if err != nil {
		return fmt.Errorf("invalid buffer-size: %w", err)
	}

	// Step 1: Scan source directory
	fmt.Printf("Scanning source: %s\n", srcDir)
	scanStart := time.Now()
	sr, err := scanner.Scan(srcDir)
	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}
	fmt.Printf("Found %d files (%s) in %d directories [%v]\n",
		sr.TotalFiles,
		scheduler.FormatSize(sr.TotalSize),
		len(sr.Dirs),
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
		fmt.Printf("  Small files (<256KB): %d (%s)\n", len(smallFiles), scheduler.FormatSize(smallTotal))
		fmt.Printf("  Large files (>=256KB): %d (%s)\n", len(largeFiles), scheduler.FormatSize(largeTotal))
	}

	if dryRun {
		fmt.Println("\n[DRY RUN] Would copy to:")
		for _, t := range targets {
			fmt.Printf("  -> %s\n", t)
		}
		fmt.Printf("Concurrency: %d, Buffer: %s\n", concurrency, bufferSize)
		return nil
	}

	// Step 2: Preload files into memory
	pool := buffer.NewPool(sr)
	fmt.Print("Loading source files into memory...")
	loadStart := time.Now()

	if preloadAll || sr.TotalSize <= 4*1024*1024*1024 { // auto preload if <= 4GB
		err = pool.PreloadAll(nil)
	} else {
		err = pool.PreloadSmallFiles(nil)
	}
	if err != nil {
		return fmt.Errorf("preload failed: %w", err)
	}
	loadDuration := time.Since(loadStart)
	fmt.Printf(" %s cached [%v]\n",
		scheduler.FormatSize(pool.TotalCached()),
		loadDuration.Round(time.Millisecond),
	)

	// Force GC before copy to free scanner temporaries
	runtime.GC()

	// Step 3: Copy to all targets
	fmt.Printf("\nCopying to %d targets (concurrency=%d):\n", len(targets), concurrency)
	for i, t := range targets {
		fmt.Printf("  [%d] %s\n", i+1, t)
	}
	fmt.Println()

	cfg := copier.Config{
		BufferSize:  bufSize,
		Incremental: incremental,
		DryRun:      false,
	}

	sched := scheduler.New(sr, pool, targets, concurrency, cfg)
	copyStart := time.Now()

	results := sched.Run(func(results []scheduler.TargetResult) {
		// Progress display
		printProgress(results, sr.TotalSize, sr.TotalFiles, copyStart)
	})

	copyDuration := time.Since(copyStart)
	fmt.Print("\r\033[K") // clear progress line

	// Step 4: Print summary
	fmt.Println("\n--- Copy Summary ---")
	allOK := true
	for _, r := range results {
		status := "OK"
		if r.Error != nil {
			status = fmt.Sprintf("FAILED: %v", r.Error)
			allOK = false
		}

		filesCopied := r.Progress.FilesCopied.Load()
		bytesCopied := r.Progress.BytesCopied.Load()
		speed := float64(0)
		if r.Duration.Seconds() > 0 {
			speed = float64(bytesCopied) / r.Duration.Seconds()
		}

		fmt.Printf("  %s: %d files, %s, %s, %s\n",
			r.TargetDir,
			filesCopied,
			scheduler.FormatSize(bytesCopied),
			r.Duration.Round(time.Millisecond),
			scheduler.FormatSpeed(speed),
		)
		if r.Error != nil {
			fmt.Printf("    Status: %s\n", status)
		}
		if incremental {
			skipped := r.Progress.FilesSkipped.Load()
			if skipped > 0 {
				fmt.Printf("    Skipped: %d files (unchanged)\n", skipped)
			}
		}
	}

	fmt.Printf("\nTotal time: %v\n", copyDuration.Round(time.Millisecond))

	// Step 5: Verify if requested
	if doVerify && allOK {
		fmt.Println("\n--- Verification ---")
		verifyAllOK := true
		for _, r := range results {
			if r.Error != nil {
				continue
			}
			fmt.Printf("Verifying %s...", r.TargetDir)
			vp := &verify.VerifyProgress{}
			vr, err := verify.Verify(sr, r.TargetDir, vp)
			if err != nil {
				fmt.Printf(" ERROR: %v\n", err)
				verifyAllOK = false
				continue
			}
			if len(vr.Mismatched) == 0 && len(vr.Errors) == 0 {
				fmt.Printf(" OK (%d/%d files match)\n", vr.MatchedFiles, vr.TotalFiles)
			} else {
				verifyAllOK = false
				fmt.Printf(" MISMATCH!\n")
				for _, m := range vr.Mismatched {
					fmt.Printf("    DIFF: %s (src=%x, tgt=%x)\n", m.RelPath, m.SourceHash, m.TargetHash)
				}
				for _, e := range vr.Errors {
					fmt.Printf("    ERR: %s: %s\n", e.RelPath, e.Error)
				}
			}
		}
		if verifyAllOK {
			fmt.Println("All targets verified successfully!")
		}
	}

	if !allOK {
		return fmt.Errorf("some targets failed")
	}
	return nil
}

func printProgress(results []scheduler.TargetResult, totalSize int64, totalFiles int, start time.Time) {
	var totalCopied int64
	var totalDone int64
	for _, r := range results {
		totalCopied += r.Progress.BytesCopied.Load()
		totalDone += r.Progress.FilesCopied.Load()
	}

	elapsed := time.Since(start).Seconds()
	avgSpeed := float64(0)
	if elapsed > 0 {
		avgSpeed = float64(totalCopied) / elapsed
	}

	// Overall progress across all targets
	totalExpected := totalSize * int64(len(results))
	pct := float64(0)
	if totalExpected > 0 {
		pct = float64(totalCopied) / float64(totalExpected) * 100
	}

	fmt.Printf("\r\033[K  [%.1f%%] %s / %s | %s | targets: ",
		pct,
		scheduler.FormatSize(totalCopied),
		scheduler.FormatSize(totalExpected),
		scheduler.FormatSpeed(avgSpeed),
	)

	for i, r := range results {
		if i > 0 {
			fmt.Print(" ")
		}
		fc := r.Progress.FilesCopied.Load()
		if r.GetError() != nil {
			fmt.Print("X")
		} else if fc >= int64(totalFiles) {
			fmt.Print("done")
		} else {
			fmt.Printf("%d/%d", fc, totalFiles)
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
