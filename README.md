# FastCP - 跨平台多目标快速复制 CLI 工具

[English](#english) | [中文](#中文)

---

## 中文

FastCP 一次性将源文件读入内存，然后并行写入多个目标目录。专为 AI 工具（OpenClaw、Claude Code）设计，解决「同一份文件同时复制到多个U盘」的场景。

### 核心特性

- **读一次，写多份** — 源数据缓存到内存，不重复读盘
- **并行多目标** — 同时写 7+ 个U盘，并发数可控
- **小文件优化** — 大缓冲区批量写入，减少 syscall 开销
- **跨平台** — Windows / macOS / Linux 单二进制，无运行时依赖
- **增量复制** — 跳过未变更文件（兼容 FAT32 时间戳精度）
- **哈希校验** — xxhash 完整性校验
- **Hub 感知** — 可调并发数，避免 USB Hub 带宽争抢

### 安装

从 [Releases](https://github.com/dongsheng123132/fastcp/releases) 下载对应平台的二进制文件，或源码编译：

```bash
go install github.com/dongsheng123132/fastcp@latest
```

### 使用方法

```bash
# 复制到多个U盘
fastcp.exe D:\源文件夹 E:\ F:\ G:\ H:\ I:\ J:\ K:\

# 带校验
fastcp.exe --verify D:\源文件夹 E:\ F:\ G:\

# 增量复制（跳过未变更文件）
fastcp.exe --incremental D:\源文件夹 E:\

# 预览模式（不实际复制）
fastcp.exe --dry-run D:\源文件夹 E:\ F:\

# 调整并发数（默认3，Hub慢时建议2）
fastcp.exe -c 2 D:\源文件夹 E:\ F:\ G:\
```

### AI 工具集成

FastCP 已发布为 Claude Code Skill 和 ClawHub Skill，AI 工具可以直接调用：

```bash
# ClawHub 安装
clawhub install fastcp

# Claude Code 中直接使用 /fastcp 调用
```

### 参数说明

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-c, --concurrency` | 3 | 同时写入的目标数 |
| `-b, --buffer-size` | 4M | 写缓冲区大小 |
| `--verify` | false | 复制后 xxhash 校验 |
| `--incremental` | false | 跳过大小/时间相同的文件 |
| `--dry-run` | false | 预览，不实际复制 |
| `-v, --verbose` | false | 详细输出 |
| `--preload-all` | false | 强制全量预加载（<=4GB 时自动启用） |

### 编译

```bash
make build        # 当前平台
make build-all    # 交叉编译 win/mac/linux
```

---

## English

FastCP reads source files into memory once, then writes to multiple target directories in parallel. Designed for AI tools (OpenClaw, Claude Code) to batch-copy files to multiple USB drives on the same hub.

### Features

- **Read once, write many** — Source data cached in memory, no repeated disk reads
- **Parallel multi-target** — Copy to 7+ USB drives simultaneously with configurable concurrency
- **Small file optimization** — Large write buffers reduce syscall overhead
- **Cross-platform** — Windows, macOS, Linux single binary, no runtime dependencies
- **Incremental copy** — Skip unchanged files (FAT32 timestamp compatible)
- **Hash verification** — xxhash integrity check after copy
- **Hub-aware** — Configurable concurrency to avoid USB hub bandwidth saturation

### Install

Download from [Releases](https://github.com/dongsheng123132/fastcp/releases), or build from source:

```bash
go install github.com/dongsheng123132/fastcp@latest
```

### Usage

```bash
# Copy to multiple USB drives
fastcp /path/to/source /media/usb1 /media/usb2 /media/usb3

# Windows
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

### AI Tool Integration

FastCP is published as a Claude Code Skill and ClawHub Skill for direct AI tool invocation:

```bash
# Install via ClawHub
clawhub install fastcp
```

### Options

| Flag | Default | Description |
|------|---------|-------------|
| `-c, --concurrency` | 3 | Number of targets to write simultaneously |
| `-b, --buffer-size` | 4M | Write buffer size (e.g. 1M, 4M, 8M) |
| `--verify` | false | Verify copies with xxhash after completion |
| `--incremental` | false | Skip files with same size/modification time |
| `--dry-run` | false | Preview only, do not copy |
| `-v, --verbose` | false | Verbose output |
| `--preload-all` | false | Preload all files into memory (auto if <=4GB) |

### Build

```bash
make build        # Current platform
make build-all    # Cross-compile win/mac/linux
```

## License

Apache 2.0
