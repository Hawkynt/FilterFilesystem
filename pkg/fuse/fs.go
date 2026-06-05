//go:build linux || darwin

// Package fuse provides the FUSE filesystem implementation for FilterFS.
// It integrates with the go-fuse library to provide a filtered view of directories.
package fuse

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"go.uber.org/zap"

	"github.com/Hawkynt/FilterFilesystem/pkg/filter"
	"github.com/Hawkynt/FilterFilesystem/pkg/pattern"
)

// FilterFS holds the shared state of the filtered filesystem: the source
// directory, configuration, pattern matcher and logger. The actual go-fuse
// node tree consists of FilterNode values referencing this shared state;
// the mount root is a FilterNode with an empty relative path.
type FilterFS struct {
	root    string           // Absolute path to the source directory
	config  *filter.Config   // Configuration settings
	matcher *pattern.Matcher // Pattern matcher for blacklisting
	logger  *zap.Logger      // Structured logger
}

// NewFilterFS creates the filtered filesystem and returns its root node,
// ready to be passed to fs.Mount.
//
// The source path in the config is converted to an absolute path for consistency.
func NewFilterFS(config *filter.Config, logger *zap.Logger) (*FilterNode, error) {
	absRoot, err := filepath.Abs(config.SourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	root := &FilterFS{
		root:    absRoot,
		config:  config,
		matcher: pattern.NewMatcher(config.Blacklist),
		logger:  logger,
	}

	return &FilterNode{path: "", root: root}, nil
}

// fullPath converts a relative filesystem path to an absolute path in the source directory.
func (r *FilterFS) fullPath(path string) string {
	return filepath.Join(r.root, path)
}

// shouldHide determines if a path should be hidden based on the blacklist patterns.
func (r *FilterFS) shouldHide(path string) bool {
	return r.matcher.IsBlacklisted(path)
}

// FilterNode represents a file or directory node in the filtered filesystem.
// It implements the go-fuse node interface and handles all filesystem operations
// while applying the filtering rules defined in the parent FilterFS.
type FilterNode struct {
	fs.Inode
	path string    // Relative path from filesystem root
	root *FilterFS // Reference to the root filesystem
}

// fullPath returns the absolute system path for this node.
func (n *FilterNode) fullPath() string {
	return n.root.fullPath(n.path)
}

// OnAdd logs the mount operation once the root node is attached to the tree.
func (n *FilterNode) OnAdd(ctx context.Context) {
	if n.path != "" {
		return
	}
	n.root.logger.Info("FilterFS mounted",
		zap.String("source", n.root.root), zap.String("mount", n.root.config.MountPath))
}

// Getattr returns file attributes for this node.
// This implements the FUSE Getattr operation for files and directories.
func (n *FilterNode) Getattr(ctx context.Context, f fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	st := &syscall.Stat_t{}
	err := syscall.Stat(n.fullPath(), st)
	if err != nil {
		return fs.ToErrno(err)
	}
	out.FromStat(st)
	return fs.OK
}

// Lookup finds a child node by name within this directory node.
// It applies filtering rules and returns ENOENT for blacklisted items.
func (n *FilterNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	path := filepath.Join(n.path, name)

	if n.root.shouldHide(path) {
		return nil, syscall.ENOENT
	}

	fullPath := n.root.fullPath(path)
	st := &syscall.Stat_t{}
	err := syscall.Stat(fullPath, st)
	if err != nil {
		return nil, fs.ToErrno(err)
	}

	out.FromStat(st)

	child := &FilterNode{
		path: path,
		root: n.root,
	}

	return n.NewInode(ctx, child, fs.StableAttr{Mode: uint32(st.Mode)}), fs.OK //nolint:unconvert // uint16 on darwin
}

// Readdir returns the contents of this directory with blacklisted items filtered out.
// Only visible files and directories are included in the result.
func (n *FilterNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	entries, err := os.ReadDir(n.fullPath())
	if err != nil {
		return nil, fs.ToErrno(err)
	}

	var filtered []fuse.DirEntry
	for _, e := range entries {
		path := filepath.Join(n.path, e.Name())
		if n.root.shouldHide(path) {
			continue
		}

		mode := uint32(0)
		if e.IsDir() {
			mode = syscall.S_IFDIR
		} else {
			mode = syscall.S_IFREG
		}

		filtered = append(filtered, fuse.DirEntry{
			Name: e.Name(),
			Mode: mode,
		})
	}

	return fs.NewListDirStream(filtered), fs.OK
}

// Setattr changes file attributes: size (truncate), mode, ownership and times.
// Without it the kernel cannot truncate files, so opening an existing file
// with O_TRUNC (e.g. os.WriteFile) would fail with ENOTSUP.
func (n *FilterNode) Setattr(
	ctx context.Context, f fs.FileHandle, in *fuse.SetAttrIn, out *fuse.AttrOut,
) syscall.Errno {
	if n.root.config.ReadOnly {
		return syscall.EROFS
	}

	p := n.fullPath()

	if errno := applySize(p, in); errno != fs.OK {
		return errno
	}
	if errno := applyModeAndOwner(p, in); errno != fs.OK {
		return errno
	}
	if errno := applyTimes(p, in); errno != fs.OK {
		return errno
	}

	st := &syscall.Stat_t{}
	if err := syscall.Stat(p, st); err != nil {
		return fs.ToErrno(err)
	}
	out.FromStat(st)

	return fs.OK
}

// applySize truncates the file when a size change is requested.
func applySize(p string, in *fuse.SetAttrIn) syscall.Errno {
	sz, ok := in.GetSize()
	if !ok {
		return fs.OK
	}
	if sz > math.MaxInt64 {
		return syscall.EINVAL
	}
	if err := os.Truncate(p, int64(sz)); err != nil {
		return fs.ToErrno(err)
	}
	return fs.OK
}

// applyModeAndOwner applies chmod/chown requests.
func applyModeAndOwner(p string, in *fuse.SetAttrIn) syscall.Errno {
	if mode, ok := in.GetMode(); ok {
		if err := os.Chmod(p, os.FileMode(mode)); err != nil {
			return fs.ToErrno(err)
		}
	}

	uid, uok := in.GetUID()
	gid, gok := in.GetGID()
	if uok || gok {
		// -1 leaves the respective id unchanged; ids follow the kernel's 32-bit ABI
		suid, sgid := -1, -1
		if uok {
			suid = int(uid)
		}
		if gok {
			sgid = int(gid)
		}
		if err := os.Chown(p, suid, sgid); err != nil {
			return fs.ToErrno(err)
		}
	}

	return fs.OK
}

// applyTimes applies atime/mtime changes.
func applyTimes(p string, in *fuse.SetAttrIn) syscall.Errno {
	atime, aok := in.GetATime()
	mtime, mok := in.GetMTime()
	if !aok && !mok {
		return fs.OK
	}

	// a zero time.Time leaves the corresponding timestamp unchanged
	var at, mt time.Time
	if aok {
		at = atime
	}
	if mok {
		mt = mtime
	}
	if err := os.Chtimes(p, at, mt); err != nil {
		return fs.ToErrno(err)
	}
	return fs.OK
}

// Open opens a file for reading or writing.
// It respects the read-only configuration and returns appropriate errors.
func (n *FilterNode) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	if n.root.config.ReadOnly && (flags&(syscall.O_WRONLY|syscall.O_RDWR)) != 0 {
		return nil, 0, syscall.EROFS
	}

	fh, err := os.OpenFile(n.fullPath(), int(flags), 0)
	if err != nil {
		return nil, 0, fs.ToErrno(err)
	}

	return &FilterFile{file: fh}, 0, fs.OK
}

// Create creates a new file in this directory.
// It prevents creation of files that would match blacklist patterns.
func (n *FilterNode) Create(
	ctx context.Context, name string, flags, mode uint32, out *fuse.EntryOut,
) (*fs.Inode, fs.FileHandle, uint32, syscall.Errno) {
	if n.root.config.ReadOnly {
		return nil, nil, 0, syscall.EROFS
	}

	if n.root.matcher.WouldCreateBlacklisted(n.path, name) {
		return nil, nil, 0, syscall.EPERM
	}

	path := filepath.Join(n.fullPath(), name)
	fh, err := os.OpenFile(path, int(flags)|os.O_CREATE, os.FileMode(mode))
	if err != nil {
		return nil, nil, 0, fs.ToErrno(err)
	}

	st := &syscall.Stat_t{}
	if err := syscall.Stat(path, st); err != nil {
		fh.Close()
		return nil, nil, 0, fs.ToErrno(err)
	}

	out.FromStat(st)

	child := &FilterNode{
		path: filepath.Join(n.path, name),
		root: n.root,
	}

	return n.NewInode(ctx, child, fs.StableAttr{Mode: uint32(st.Mode)}), &FilterFile{file: fh}, 0, fs.OK //nolint:unconvert,lll // uint16 on darwin
}

// Mkdir creates a new directory in this directory.
// It prevents creation of directories that would match blacklist patterns.
func (n *FilterNode) Mkdir(
	ctx context.Context, name string, mode uint32, out *fuse.EntryOut,
) (*fs.Inode, syscall.Errno) {
	if n.root.config.ReadOnly {
		return nil, syscall.EROFS
	}

	if n.root.matcher.WouldCreateBlacklisted(n.path, name) {
		return nil, syscall.EPERM
	}

	path := filepath.Join(n.fullPath(), name)
	if err := os.Mkdir(path, os.FileMode(mode)); err != nil {
		return nil, fs.ToErrno(err)
	}

	st := &syscall.Stat_t{}
	if err := syscall.Stat(path, st); err != nil {
		return nil, fs.ToErrno(err)
	}

	out.FromStat(st)

	child := &FilterNode{
		path: filepath.Join(n.path, name),
		root: n.root,
	}

	return n.NewInode(ctx, child, fs.StableAttr{Mode: uint32(st.Mode)}), fs.OK //nolint:unconvert // uint16 on darwin
}

// Unlink removes a file from this directory.
// It prevents deletion of blacklisted files (which shouldn't be visible anyway).
func (n *FilterNode) Unlink(ctx context.Context, name string) syscall.Errno {
	if n.root.config.ReadOnly {
		return syscall.EROFS
	}

	path := filepath.Join(n.path, name)
	if n.root.shouldHide(path) {
		return syscall.ENOENT
	}

	fullPath := filepath.Join(n.fullPath(), name)
	if err := os.Remove(fullPath); err != nil {
		return fs.ToErrno(err)
	}

	return fs.OK
}

// Rmdir removes a directory from this directory.
// It respects the allow_delete_with_hidden configuration setting.
func (n *FilterNode) Rmdir(ctx context.Context, name string) syscall.Errno {
	if n.root.config.ReadOnly {
		return syscall.EROFS
	}

	dirPath := filepath.Join(n.path, name)
	if n.root.shouldHide(dirPath) {
		return syscall.ENOENT
	}

	// Check if directory contains hidden files
	if !n.root.config.AllowDelete && n.hasHiddenChildren(name) {
		return syscall.EPERM
	}

	fullPath := filepath.Join(n.fullPath(), name)
	if err := os.Remove(fullPath); err != nil {
		return fs.ToErrno(err)
	}

	return fs.OK
}

// hasHiddenChildren checks if a directory contains any blacklisted files or directories.
// This is used to enforce policies about operations on directories with hidden content.
func (n *FilterNode) hasHiddenChildren(name string) bool {
	dirPath := filepath.Join(n.fullPath(), name)
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return false
	}

	// Get entry names for pattern matcher
	entryNames := make([]string, len(entries))
	for i, e := range entries {
		entryNames[i] = e.Name()
	}

	// Use the pattern matcher to check for blacklisted children
	relativeDirPath := filepath.Join(n.path, name)
	return n.root.matcher.HasBlacklistedChildren(relativeDirPath, entryNames)
}

// Rename moves/renames a file or directory.
// It prevents renaming to blacklisted names and respects the allow_rename_with_hidden setting.
func (n *FilterNode) Rename(
	ctx context.Context, name string, newParent fs.InodeEmbedder, newName string, flags uint32,
) syscall.Errno {
	if n.root.config.ReadOnly {
		return syscall.EROFS
	}

	newParentNode, ok := newParent.(*FilterNode)
	if !ok {
		return syscall.EINVAL
	}

	oldPath := filepath.Join(n.path, name)

	// Check if source is hidden
	if n.root.shouldHide(oldPath) {
		return syscall.ENOENT
	}

	// Check if destination would be blacklisted
	if n.root.matcher.WouldCreateBlacklisted(newParentNode.path, newName) {
		return syscall.EPERM
	}

	// Check if moving directory with hidden children
	info, err := os.Stat(n.fullPath())
	if err == nil && info.IsDir() && !n.root.config.AllowRename && n.hasHiddenChildren(name) {
		return syscall.EPERM
	}

	oldFullPath := filepath.Join(n.fullPath(), name)
	newFullPath := filepath.Join(newParentNode.fullPath(), newName)

	if err := os.Rename(oldFullPath, newFullPath); err != nil {
		return fs.ToErrno(err)
	}

	return fs.OK
}

// FilterFile represents an open file handle in the filtered filesystem.
// It wraps the underlying os.File and implements the go-fuse FileHandle interface.
type FilterFile struct {
	file *os.File
}

// Read reads data from the file at the specified offset.
func (f *FilterFile) Read(ctx context.Context, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	n, err := f.file.ReadAt(dest, off)
	if err != nil && err != io.EOF {
		return nil, fs.ToErrno(err)
	}
	return fuse.ReadResultData(dest[:n]), fs.OK
}

// Write writes data to the file at the specified offset.
func (f *FilterFile) Write(ctx context.Context, data []byte, off int64) (written uint32, errno syscall.Errno) {
	n, err := f.file.WriteAt(data, off)
	// n is bounded by len(data), which FUSE caps far below the uint32 limit
	return uint32(n), fs.ToErrno(err) //nolint:gosec // see bound note above
}

// Release closes the file handle.
func (f *FilterFile) Release(ctx context.Context) syscall.Errno {
	return fs.ToErrno(f.file.Close())
}

// Flush ensures all written data is synced to storage.
func (f *FilterFile) Flush(ctx context.Context) syscall.Errno {
	return fs.ToErrno(f.file.Sync())
}

// Fsync synchronizes file data and metadata to storage.
func (f *FilterFile) Fsync(ctx context.Context, flags uint32) syscall.Errno {
	return fs.ToErrno(f.file.Sync())
}
