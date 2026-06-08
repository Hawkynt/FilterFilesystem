# FilterFS

[![License](https://img.shields.io/github/license/Hawkynt/FilterFilesystem)](https://github.com/Hawkynt/FilterFilesystem/blob/main/LICENSE)
[![Language](https://img.shields.io/github/languages/top/Hawkynt/FilterFilesystem?color=8957D5)](https://github.com/Hawkynt/FilterFilesystem)

[![CI](https://github.com/Hawkynt/FilterFilesystem/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/Hawkynt/FilterFilesystem/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/Hawkynt/FilterFilesystem)](https://goreportcard.com/report/github.com/Hawkynt/FilterFilesystem)
![Last Commit](https://img.shields.io/github/last-commit/Hawkynt/FilterFilesystem?branch=main)
![Activity](https://img.shields.io/github/commit-activity/m/Hawkynt/FilterFilesystem)

[![Stars](https://img.shields.io/github/stars/Hawkynt/FilterFilesystem?color=FFD700)](https://github.com/Hawkynt/FilterFilesystem/stargazers)
[![Forks](https://img.shields.io/github/forks/Hawkynt/FilterFilesystem?color=008080)](https://github.com/Hawkynt/FilterFilesystem/network/members)
[![Issues](https://img.shields.io/github/issues/Hawkynt/FilterFilesystem)](https://github.com/Hawkynt/FilterFilesystem/issues)
![Code Size](https://img.shields.io/github/languages/code-size/Hawkynt/FilterFilesystem?color=4CAF50)
![Repo Size](https://img.shields.io/github/repo-size/Hawkynt/FilterFilesystem?color=FF9800)

[![Release](https://img.shields.io/github/v/release/Hawkynt/FilterFilesystem)](https://github.com/Hawkynt/FilterFilesystem/releases/latest)
[![Nightly](https://img.shields.io/github/v/release/Hawkynt/FilterFilesystem?include_prereleases&sort=date&label=nightly&color=FF9800)](https://github.com/Hawkynt/FilterFilesystem/releases)
[![Downloads](https://img.shields.io/github/downloads/Hawkynt/FilterFilesystem/total)](https://github.com/Hawkynt/FilterFilesystem/releases)

> A high-performance filter filesystem in Go (FUSE on Linux, WinFsp on Windows) that mounts any directory with fine-grained pattern-based visibility filtering — read-only or read-write — so tools and users only ever see the files they're supposed to.

## ✨ Features

- **Pattern-based Filtering**: Advanced glob and wildcard pattern matching
- **Flexible Mount Modes**: Support for both read-only and read-write mounting
- **Smart Operations**: Intelligent handling of operations on filtered content
- **High Performance**: Optimized for low latency and high throughput
- **Cross-platform**: Linux and macOS via FUSE, Windows via [WinFsp](https://winfsp.dev)
- **Comprehensive Logging**: Detailed logging with configurable levels
- **Docker Support**: Ready-to-use Docker containers
- **Production Ready**: Extensive testing and error handling

## 📦 Installation

### From Source

```bash
# Clone the repository
git clone https://github.com/Hawkynt/FilterFilesystem.git
cd FilterFilesystem

# Build and install
make install
```

### Using Go Install

```bash
go install github.com/Hawkynt/FilterFilesystem/cmd/filterfs@latest
```

### Docker

```bash
docker pull ghcr.io/hawkynt/filterfilesystem:latest
```

### Binary Releases

Download pre-built binaries from the [releases page](https://github.com/Hawkynt/FilterFilesystem/releases).
Stable releases are dated `vyyyyMMdd` tags; automatically published `nightly-yyyyMMdd`
prereleases track every green CI run on `main`.

### Windows

FilterFS runs on Windows through [WinFsp](https://winfsp.dev) - install it first
(`winget install WinFsp.WinFsp`), then mount onto an unused drive letter or a
non-existing directory:

```powershell
filterfs.exe mount -s C:\data -m X: -b "**/*.log"
```

The mount point must not already exist (WinFsp creates it). Unmount by stopping
the filterfs process (Ctrl+C); the `unmount` command is Unix-only.

## 🚀 Quick Start

### Basic Usage

```bash
# Mount a directory with basic filtering
filterfs mount -s /path/to/source -m /path/to/mount -b "**/*.log" -b "**/*.tmp"

# Mount in read-only mode
filterfs mount -s /path/to/source -m /path/to/mount --readonly -b "**/.git"

# Use a configuration file
filterfs mount --config filterfs.yaml
```

### Configuration File

Create a `filterfs.yaml` configuration file:

```yaml
source_path: /home/user/documents
mount_path: /home/user/filtered
read_only: false
blacklist:
  - "**/*.log"          # Hide all log files
  - "**/*.tmp"          # Hide all temporary files
  - "**/.git"           # Hide git directories
  - "**/node_modules"   # Hide node_modules directories
  - "**/*.cache"        # Hide cache files
  - "**/temp"           # Hide temp directories
allow_delete_with_hidden: false
allow_rename_with_hidden: false
```

## Pattern Matching

FilterFS supports sophisticated pattern matching for blacklisting files and directories:

### Pattern Types

| Pattern | Description | Example |
|---------|-------------|---------|
| `*/filename` | Matches files in the first sublevel only | `*/config.txt` |
| `**/filename` | Matches files at any level | `**/secret.key` |
| `/**/*.ext` | Matches all files with extension from root | `/**/*.log` |
| `**/*.ext` | Matches all files with extension anywhere | `**/*.tmp` |
| `**/dirname` | Matches directories at any level | `**/.git` |

### Pattern Examples

```yaml
blacklist:
  # Hide specific files
  - "**/Thumbs.db"        # Windows thumbnail cache
  - "**/.DS_Store"        # macOS metadata
  
  # Hide by extension
  - "**/*.log"            # All log files
  - "**/*.tmp"            # All temporary files
  - "**/*.bak"            # All backup files
  
  # Hide directories
  - "**/.git"             # Git repositories
  - "**/node_modules"     # Node.js dependencies
  - "**/__pycache__"      # Python cache
  - "**/target"           # Rust/Java build output
  
  # Hide first-level only
  - "*/secrets"           # Hide 'secrets' in immediate subdirectories only
  
  # Complex patterns
  - "**/.*"               # Hide all hidden files (starting with .)
  - "/**/cache.*"         # Hide cache files with any extension
```

## Command Line Interface

### Mount Command

```bash
filterfs mount [flags]

Flags:
  -s, --source string      Source directory to filter
  -m, --mount string       Mount point for filtered filesystem
  -c, --config string      Configuration file path
  -r, --readonly           Mount as read-only
  -b, --blacklist strings  Blacklist patterns
      --allow-other        Allow other users to access the mount (requires 'user_allow_other' in /etc/fuse.conf)
      --log-level string   Log level (debug, info, warn, error) (default "info")
  -h, --help              Help for mount
```

### Examples

```bash
# Basic filtering
filterfs mount -s ~/documents -m ~/filtered -b "**/*.log" -b "**/.git"

# Read-only mount with multiple patterns
filterfs mount -s /data -m /filtered --readonly \
  -b "**/*.tmp" -b "**/cache" -b "**/*.log"

# Using configuration file
filterfs mount --config ./filterfs.yaml

# Debug mode
filterfs mount -s ~/test -m ~/filtered --log-level debug -b "**/*.hidden"
```

## Configuration

### Configuration File Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `source_path` | string | required | Path to source directory |
| `mount_path` | string | required | Path to mount point |
| `read_only` | bool | false | Mount in read-only mode |
| `blacklist` | []string | [] | List of blacklist patterns |
| `allow_delete_with_hidden` | bool | false | Allow deleting directories containing hidden files |
| `allow_rename_with_hidden` | bool | false | Allow renaming directories containing hidden files |

### Environment Variables

- `FILTERFS_LOG_LEVEL`: Override log level
- `FILTERFS_CONFIG`: Default configuration file path

## Docker Usage

### Basic Docker Run

```bash
# Create directories
mkdir -p ~/source ~/filtered

# Run FilterFS in Docker
docker run -d \
  --name filterfs \
  --device /dev/fuse \
  --cap-add SYS_ADMIN \
  --security-opt apparmor:unconfined \
  -v ~/source:/mnt/source:ro \
  -v ~/filtered:/mnt/filtered:rshared \
  ghcr.io/hawkynt/filterfilesystem:latest \
  mount -s /mnt/source -m /mnt/filtered -b "**/*.log"
```

### Docker Compose

```yaml
version: '3.8'
services:
  filterfs:
    image: ghcr.io/hawkynt/filterfilesystem:latest
    devices:
      - /dev/fuse
    cap_add:
      - SYS_ADMIN
    security_opt:
      - apparmor:unconfined
    volumes:
      - ./source:/mnt/source:ro
      - ./filtered:/mnt/filtered:rshared
      - ./config.yaml:/etc/filterfs/config.yaml
    command: mount --config /etc/filterfs/config.yaml
```

## 🛠️ Development

### Prerequisites

- Go 1.20 or later
- FUSE development libraries
- Make

#### Installing FUSE Libraries

**Ubuntu/Debian:**
```bash
sudo apt-get install fuse libfuse-dev
```

**macOS:**
```bash
brew install macfuse
```

**RHEL/CentOS:**
```bash
sudo yum install fuse fuse-devel
```

### Building from Source

```bash
# Clone repository
git clone https://github.com/Hawkynt/FilterFilesystem.git
cd FilterFilesystem

# Install dependencies
make deps

# Run tests
make test

# Build binary
make build

# Run all checks
make check
```

### Development Commands

```bash
make help              # Show all available commands
make build             # Build the binary
make test              # Run tests
make test-coverage     # Run tests with coverage
make lint              # Run linters
make fmt               # Format code
make check             # Run all checks
make dev               # Run in development mode
make example-config    # Create example config
make build-all         # Build for all platforms
```

### Running Tests

```bash
# Run all tests
make test

# Run tests with coverage
make test-coverage

# Run benchmarks
make bench

# Run specific test
go test -v ./pkg/pattern -run TestMatcher

# Run integration tests (requires FUSE)
sudo make test
```

## Performance

FilterFS is optimized for performance with:

- **Zero-copy operations** where possible
- **Efficient pattern matching** using optimized algorithms
- **Minimal memory allocations** in hot paths
- **Concurrent file operations** support
- **Smart caching** of filesystem metadata

### Benchmarks

```bash
# Run performance benchmarks
make bench

# Example results on modern hardware:
BenchmarkPatternMatching-8     1000000    1.2 µs/op    0 allocs/op
BenchmarkFileRead-8            500000     2.4 µs/op    1 allocs/op
BenchmarkDirectoryList-8       100000     15.6 µs/op   3 allocs/op
```

## Security Considerations

- FilterFS operates with the same permissions as the mounting user
- Hidden files are completely invisible to applications accessing the mount
- Read-only mounts prevent any modifications to the source filesystem
- Pattern matching is performed securely without shell expansion
- No sensitive information is logged by default

## Troubleshooting

### Common Issues

**Permission Denied:**
```bash
# Make sure you have permission to use FUSE
sudo usermod -a -G fuse $USER
# Log out and back in
```

**Mount Point Busy:**
```bash
# Unmount existing filesystem
fusermount -u /path/to/mount
# Or force unmount
sudo umount -f /path/to/mount
```

**FUSE Not Available:**
```bash
# Load FUSE module
sudo modprobe fuse

# Install FUSE (Ubuntu/Debian)
sudo apt-get install fuse
```

### Debug Mode

Enable debug logging for troubleshooting:

```bash
filterfs mount -s ~/source -m ~/mount --log-level debug -b "**/*.log"
```

### Log Analysis

FilterFS provides structured logging:

```json
{
  "level": "info",
  "ts": "2024-01-15T10:30:00Z",
  "msg": "FilterFS mounted",
  "source": "/home/user/documents",
  "mount": "/home/user/filtered",
  "readonly": false,
  "blacklist_patterns": 3
}
```

## Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Run `make check`
6. Submit a pull request

## 📜 License

Licensed under LGPL-3.0-or-later — see [LICENSE](LICENSE).

## Acknowledgments

- Built with [go-fuse](https://github.com/hanwen/go-fuse) for FUSE integration
- Uses [cobra](https://github.com/spf13/cobra) for CLI interface
- Logging powered by [zap](https://github.com/uber-go/zap)

## 🆘 Getting Help

- **Issues**: [GitHub Issues](https://github.com/Hawkynt/FilterFilesystem/issues)
- **Discussions**: [GitHub Discussions](https://github.com/Hawkynt/FilterFilesystem/discussions)

## ❤️ Support

If this project saves you time or money, consider supporting its development:

[![GitHub Sponsors](https://img.shields.io/badge/GitHub-Sponsor-EA4AAA?logo=githubsponsors)](https://github.com/sponsors/Hawkynt)
[![PayPal](https://img.shields.io/badge/PayPal-Donate-00457C?logo=paypal)](https://www.paypal.me/hawkynt)