---
name: fastcp
version: 1.0.0
description: Copy files from a source directory to multiple target directories in parallel. Optimized for batch-copying to multiple USB drives on the same hub. Reads source once into memory, writes to all targets concurrently.
user_invocable: true
---

# FastCP - Multi-target Parallel Copy

Use the `fastcp` CLI tool to copy a source directory to one or more target directories in parallel.

## When to use
- Copying files to multiple USB drives simultaneously
- Batch deploying the same files to multiple destinations
- Any scenario requiring one-to-many file copy with verification

## Binary location
The fastcp binary is at: `C:/Users/ZhuanZ/Desktop/newclaude/fastcp/fastcp.exe`

If not built yet, build it:
```bash
cd "C:/Users/ZhuanZ/Desktop/newclaude/fastcp" && go build -ldflags "-s -w" -o fastcp.exe .
```

## Usage

```bash
# Basic: copy to multiple targets
fastcp.exe <source_dir> <target1> <target2> [target3] ...

# With verification (xxhash)
fastcp.exe --verify <source> <target1> <target2>

# Incremental (skip unchanged files)
fastcp.exe --incremental <source> <target1>

# Control parallelism (default 3, reduce for slow hubs)
fastcp.exe -c 2 <source> <target1> <target2> <target3>

# Preview without copying
fastcp.exe --dry-run <source> <target1> <target2>
```

## Key flags
- `-c, --concurrency N`: simultaneous target writes (default 3)
- `-b, --buffer-size`: write buffer (default 4M)
- `--verify`: xxhash integrity check after copy
- `--incremental`: skip files with matching size/mtime
- `--dry-run`: preview only
- `--preload-all`: force all files into memory

## Typical workflow for USB batch copy
1. Identify mounted USB drives (e.g. E:\, F:\, G:\)
2. Run: `fastcp.exe --verify D:\source E:\ F:\ G:\`
3. Check output for verification results
