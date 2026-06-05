//go:build windows

// Package winfs provides the Windows implementation of FilterFS on top of
// WinFsp (via cgofuse). It reuses the same platform-neutral core as the
// FUSE backend: pkg/filter for configuration and pkg/pattern for blacklist
// matching, so both backends enforce identical filtering semantics.
package winfs

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/winfsp/cgofuse/fuse"
	"go.uber.org/zap"
	"golang.org/x/sys/windows"

	"github.com/Hawkynt/FilterFilesystem/pkg/filter"
	"github.com/Hawkynt/FilterFilesystem/pkg/pattern"
)

// FilterFS implements cgofuse's FileSystemInterface as a filtered passthrough
// to the source directory.
type FilterFS struct {
	fuse.FileSystemBase
	root    string           // Absolute path to the source directory
	config  *filter.Config   // Configuration settings
	matcher *pattern.Matcher // Pattern matcher for blacklisting
	logger  *zap.Logger      // Structured logger

	mu      sync.Mutex
	handles map[uint64]*os.File
	nextFh  uint64
}

// New creates the filtered filesystem ready to be passed to a cgofuse host.
func New(config *filter.Config, logger *zap.Logger) (*FilterFS, error) {
	absRoot, err := filepath.Abs(config.SourcePath)
	if err != nil {
		return nil, err
	}

	return &FilterFS{
		root:    absRoot,
		config:  config,
		matcher: pattern.NewMatcher(config.Blacklist),
		logger:  logger,
		handles: map[uint64]*os.File{},
	}, nil
}

// relPath converts the slash-separated FUSE path to a path relative to the
// source root ("" for the root itself).
func relPath(fusePath string) string {
	return strings.TrimPrefix(fusePath, "/")
}

// fullPath returns the absolute path in the source directory for a FUSE path.
func (f *FilterFS) fullPath(fusePath string) string {
	return filepath.Join(f.root, filepath.FromSlash(relPath(fusePath)))
}

// shouldHide determines if a path should be hidden based on the blacklist patterns.
func (f *FilterFS) shouldHide(fusePath string) bool {
	rel := relPath(fusePath)
	return rel != "" && f.matcher.IsBlacklisted(rel)
}

// hasHiddenChildren checks if the directory at the FUSE path contains blacklisted entries.
func (f *FilterFS) hasHiddenChildren(fusePath string) bool {
	entries, err := os.ReadDir(f.fullPath(fusePath))
	if err != nil {
		return false
	}

	entryNames := make([]string, len(entries))
	for i, e := range entries {
		entryNames[i] = e.Name()
	}

	return f.matcher.HasBlacklistedChildren(relPath(fusePath), entryNames)
}

// errno maps a Go filesystem error to a negative FUSE errno.
func errno(err error) int {
	var winErr syscall.Errno
	switch {
	case err == nil:
		return 0
	case errors.Is(err, fs.ErrNotExist):
		return -fuse.ENOENT
	case errors.Is(err, fs.ErrExist):
		return -fuse.EEXIST
	case errors.Is(err, fs.ErrPermission):
		return -fuse.EACCES
	case errors.As(err, &winErr) && winErr == windows.ERROR_PRIVILEGE_NOT_HELD:
		return -fuse.EPERM // e.g. symlink creation without Developer Mode
	default:
		return -fuse.EIO
	}
}

// statFromInfo fills a fuse.Stat_t from an os.FileInfo.
func statFromInfo(info os.FileInfo, stat *fuse.Stat_t) {
	perm := uint32(info.Mode().Perm())
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		stat.Mode = fuse.S_IFLNK | perm
	case info.IsDir():
		stat.Mode = fuse.S_IFDIR | perm
	default:
		stat.Mode = fuse.S_IFREG | perm
	}
	stat.Nlink = 1
	stat.Size = info.Size()
	ts := fuse.NewTimespec(info.ModTime())
	stat.Mtim = ts
	stat.Ctim = ts
	stat.Atim = ts
}

// toOSFlags translates FUSE open flags into os.OpenFile flags.
func toOSFlags(flags int) int {
	var o int
	switch flags & fuse.O_ACCMODE {
	case fuse.O_WRONLY:
		o = os.O_WRONLY
	case fuse.O_RDWR:
		o = os.O_RDWR
	default:
		o = os.O_RDONLY
	}
	if flags&fuse.O_APPEND != 0 {
		o |= os.O_APPEND
	}
	if flags&fuse.O_CREAT != 0 {
		o |= os.O_CREATE
	}
	if flags&fuse.O_TRUNC != 0 {
		o |= os.O_TRUNC
	}
	if flags&fuse.O_EXCL != 0 {
		o |= os.O_EXCL
	}
	return o
}

// track registers an open file and returns its handle id.
func (f *FilterFS) track(file *os.File) uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextFh++
	fh := f.nextFh
	f.handles[fh] = file
	return fh
}

// handle resolves a handle id to its open file.
func (f *FilterFS) handle(fh uint64) *os.File {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.handles[fh]
}

// drop closes and forgets an open handle.
func (f *FilterFS) drop(fh uint64) int {
	f.mu.Lock()
	file := f.handles[fh]
	delete(f.handles, fh)
	f.mu.Unlock()
	if file == nil {
		return -fuse.EBADF
	}
	return errno(file.Close())
}

// Statfs reports the capacity of the volume backing the source directory so
// tools like Explorer show meaningful totals instead of zeros.
func (f *FilterFS) Statfs(path string, stat *fuse.Statfs_t) int {
	root, err := windows.UTF16PtrFromString(f.root)
	if err != nil {
		return -fuse.EIO
	}

	var freeAvail, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(root, &freeAvail, &total, &totalFree); err != nil {
		return errno(err)
	}

	// Report in fixed 4 KiB blocks - WinFsp only cares about the products
	// Frsize*Blocks etc., not the underlying cluster size.
	const blockSize = 4096
	stat.Bsize = blockSize
	stat.Frsize = blockSize
	stat.Blocks = total / blockSize
	stat.Bfree = totalFree / blockSize
	stat.Bavail = freeAvail / blockSize
	stat.Namemax = 255 // NTFS path-component limit

	return 0
}

// Getattr returns file attributes; blacklisted paths report ENOENT.
// Lstat semantics: symbolic links report themselves, not their target.
func (f *FilterFS) Getattr(path string, stat *fuse.Stat_t, fh uint64) int {
	if f.shouldHide(path) {
		return -fuse.ENOENT
	}

	info, err := os.Lstat(f.fullPath(path))
	if err != nil {
		return errno(err)
	}

	statFromInfo(info, stat)
	return 0
}

// Readlink resolves a symbolic link in the source tree.
func (f *FilterFS) Readlink(path string) (errc int, target string) {
	if f.shouldHide(path) {
		return -fuse.ENOENT, ""
	}

	target, err := os.Readlink(f.fullPath(path))
	if err != nil {
		return errno(err), ""
	}
	return 0, filepath.ToSlash(target)
}

// Symlink creates a symbolic link. On Windows this needs either Developer
// Mode or the SeCreateSymbolicLinkPrivilege; without it EPERM is reported.
func (f *FilterFS) Symlink(target, newpath string) int {
	if f.config.ReadOnly {
		return -fuse.EROFS
	}
	if f.shouldHide(newpath) {
		return -fuse.EPERM
	}
	return errno(os.Symlink(filepath.FromSlash(target), f.fullPath(newpath)))
}

// Link creates a hard link on the source volume (NTFS supports them; whether
// the request ever arrives depends on the WinFsp version's FUSE layer).
func (f *FilterFS) Link(oldpath, newpath string) int {
	if f.config.ReadOnly {
		return -fuse.EROFS
	}
	if f.shouldHide(oldpath) {
		return -fuse.ENOENT
	}
	if f.shouldHide(newpath) {
		return -fuse.EPERM
	}
	return errno(os.Link(f.fullPath(oldpath), f.fullPath(newpath)))
}

// Opendir validates that the directory is visible and accessible.
func (f *FilterFS) Opendir(path string) (errc int, fh uint64) {
	if f.shouldHide(path) {
		return -fuse.ENOENT, ^uint64(0)
	}

	info, err := os.Stat(f.fullPath(path))
	if err != nil {
		return errno(err), ^uint64(0)
	}
	if !info.IsDir() {
		return -fuse.ENOTDIR, ^uint64(0)
	}

	return 0, 0
}

// Releasedir is a no-op; Opendir allocates no per-directory state.
func (f *FilterFS) Releasedir(path string, fh uint64) int {
	return 0
}

// Readdir lists a directory with blacklisted entries filtered out.
func (f *FilterFS) Readdir(path string,
	fill func(name string, stat *fuse.Stat_t, ofst int64) bool,
	ofst int64, fh uint64,
) int {
	if f.shouldHide(path) {
		return -fuse.ENOENT
	}

	entries, err := os.ReadDir(f.fullPath(path))
	if err != nil {
		return errno(err)
	}

	fill(".", nil, 0)
	fill("..", nil, 0)
	rel := relPath(path)
	for _, e := range entries {
		childRel := filepath.Join(rel, e.Name())
		if f.matcher.IsBlacklisted(childRel) {
			continue
		}
		if !fill(e.Name(), nil, 0) {
			break
		}
	}

	return 0
}

// Open opens a file honoring visibility and the read-only configuration.
func (f *FilterFS) Open(path string, flags int) (errc int, fh uint64) {
	if f.shouldHide(path) {
		return -fuse.ENOENT, ^uint64(0)
	}

	if f.config.ReadOnly && flags&(fuse.O_WRONLY|fuse.O_RDWR) != 0 {
		return -fuse.EROFS, ^uint64(0)
	}

	file, err := os.OpenFile(f.fullPath(path), toOSFlags(flags), 0)
	if err != nil {
		return errno(err), ^uint64(0)
	}

	return 0, f.track(file)
}

// Create creates a new file unless it would match a blacklist pattern.
func (f *FilterFS) Create(path string, flags int, mode uint32) (errc int, fh uint64) {
	if f.config.ReadOnly {
		return -fuse.EROFS, ^uint64(0)
	}

	if f.shouldHide(path) {
		return -fuse.EPERM, ^uint64(0)
	}

	file, err := os.OpenFile(f.fullPath(path), toOSFlags(flags)|os.O_CREATE, os.FileMode(mode))
	if err != nil {
		return errno(err), ^uint64(0)
	}

	return 0, f.track(file)
}

// Truncate resizes a file, honoring read-only mode.
func (f *FilterFS) Truncate(path string, size int64, fh uint64) int {
	if f.config.ReadOnly {
		return -fuse.EROFS
	}
	if f.shouldHide(path) {
		return -fuse.ENOENT
	}

	if file := f.handle(fh); file != nil {
		return errno(file.Truncate(size))
	}
	return errno(os.Truncate(f.fullPath(path), size))
}

// Read reads from an open handle at the given offset.
func (f *FilterFS) Read(path string, buff []byte, ofst int64, fh uint64) int {
	file := f.handle(fh)
	if file == nil {
		return -fuse.EBADF
	}

	n, err := file.ReadAt(buff, ofst)
	if err != nil && err != io.EOF {
		return errno(err)
	}
	return n
}

// Write writes to an open handle at the given offset.
func (f *FilterFS) Write(path string, buff []byte, ofst int64, fh uint64) int {
	if f.config.ReadOnly {
		return -fuse.EROFS
	}

	file := f.handle(fh)
	if file == nil {
		return -fuse.EBADF
	}

	n, err := file.WriteAt(buff, ofst)
	if err != nil {
		return errno(err)
	}
	return n
}

// Flush syncs an open handle's data to storage.
func (f *FilterFS) Flush(path string, fh uint64) int {
	file := f.handle(fh)
	if file == nil {
		return -fuse.EBADF
	}
	return errno(file.Sync())
}

// Fsync synchronizes file data to storage.
func (f *FilterFS) Fsync(path string, datasync bool, fh uint64) int {
	return f.Flush(path, fh)
}

// Release closes an open handle.
func (f *FilterFS) Release(path string, fh uint64) int {
	return f.drop(fh)
}

// Mkdir creates a directory unless it would match a blacklist pattern.
func (f *FilterFS) Mkdir(path string, mode uint32) int {
	if f.config.ReadOnly {
		return -fuse.EROFS
	}
	if f.shouldHide(path) {
		return -fuse.EPERM
	}
	return errno(os.Mkdir(f.fullPath(path), os.FileMode(mode)))
}

// Unlink removes a file; hidden files report ENOENT.
func (f *FilterFS) Unlink(path string) int {
	if f.config.ReadOnly {
		return -fuse.EROFS
	}
	if f.shouldHide(path) {
		return -fuse.ENOENT
	}
	return errno(os.Remove(f.fullPath(path)))
}

// Rmdir removes a directory, respecting the allow_delete_with_hidden setting.
func (f *FilterFS) Rmdir(path string) int {
	if f.config.ReadOnly {
		return -fuse.EROFS
	}
	if f.shouldHide(path) {
		return -fuse.ENOENT
	}
	if !f.config.AllowDelete && f.hasHiddenChildren(path) {
		return -fuse.EPERM
	}
	return errno(os.Remove(f.fullPath(path)))
}

// Rename moves a file or directory, enforcing the same policies as the FUSE backend.
func (f *FilterFS) Rename(oldpath, newpath string) int {
	if f.config.ReadOnly {
		return -fuse.EROFS
	}
	if f.shouldHide(oldpath) {
		return -fuse.ENOENT
	}
	if f.shouldHide(newpath) {
		return -fuse.EPERM
	}

	info, err := os.Stat(f.fullPath(oldpath))
	if err == nil && info.IsDir() && !f.config.AllowRename && f.hasHiddenChildren(oldpath) {
		return -fuse.EPERM
	}

	return errno(os.Rename(f.fullPath(oldpath), f.fullPath(newpath)))
}

// Chmod changes file permissions (best effort on Windows).
func (f *FilterFS) Chmod(path string, mode uint32) int {
	if f.config.ReadOnly {
		return -fuse.EROFS
	}
	if f.shouldHide(path) {
		return -fuse.ENOENT
	}
	return errno(os.Chmod(f.fullPath(path), os.FileMode(mode)))
}

// Utimens updates access and modification times.
func (f *FilterFS) Utimens(path string, tmsp []fuse.Timespec) int {
	if f.config.ReadOnly {
		return -fuse.EROFS
	}
	if f.shouldHide(path) {
		return -fuse.ENOENT
	}
	// the FUSE contract passes exactly [atime, mtime]
	const timespecCount = 2
	if len(tmsp) < timespecCount {
		return -fuse.EINVAL
	}
	return errno(os.Chtimes(f.fullPath(path), tmsp[0].Time(), tmsp[1].Time()))
}
