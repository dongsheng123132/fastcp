# FastCP - Cross-platform Multi-target Fast Copy

FastCP reads source files into memory once, then writes to multiple target directories in parallel. Optimized for copying to multiple USB drives on the same USB hub.

## Features

- **Read once, write many** - Source data cached in memory, no repeated disk reads
- **Parallel multi-target** - Copy to 7+ USB drives simultaneously with configurable concurrency
- **Small file optimization** - Batched I/O with large write buffers reduces syscall overhead
- **Cross-platform** - Windows, macOS, Linux (single binary, no runtime dependencies)
- **Incremental copy** - Skip unchanged files
- **Hash verification** - xxhash-based integrity check after copy
- **Hub-aware** - Configurable concurrency to avoid USB hub bandwidth saturation

## Install

Download from [Releases](https://github.com/dongsheng123132/fastcp/releases), or build from source:

```bash
go install github.com/dongsheng123132/fastcp@latest
```

## Usage

```bash
# Copy to multiple targets
fastcp /path/to/source /media/usb1 /media/usb2 /media/usb3

# Windows - copy to multiple USB drives
fastcp.exe D:\source E:\ F:\ G:\ H:\ I:\ J:\ K:\

# With verification
fastcp --verify /path/to/source /media/usb1 /media/usb2

# Incremental copy (skip unchanged files)
fastcp --incremental /path/to/source /media/usb1

# Preview without copying
fastcp --dry-run /path/to/source /media/usb1 /media/usb2

# Adjust concurrency for USB hub (default: 3)
fastcp -c 2 /path/to/source /media/usb1 /media/usb2 /media/usb3
```

## Options

| Flag | Default | Description |
|------|---------|-------------|
| `-c, --concurrency` | 3 | Number of targets to write simultaneously |
| `-b, --buffer-size` | 4M | Write buffer size (e.g. 1M, 4M, 8M) |
| `--verify` | false | Verify copies with xxhash after completion |
| `--incremental` | false | Skip files with same size/modification time |
| `--dry-run` | false | Preview only, do not copy |
| `-v, --verbose` | false | Verbose output |
| `--preload-all` | false | Preload all files into memory (auto if <=4GB) |

## Build

```bash
# Build for current platform
make build

# Build for all platforms
make build-all

# Output in dist/
```

## License

Apache 2.0
