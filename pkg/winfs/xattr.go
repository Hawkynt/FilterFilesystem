//go:build windows

package winfs

import (
	"os"
	"strings"
	"unsafe"

	"github.com/winfsp/cgofuse/fuse"
	"golang.org/x/sys/windows"
)

// Extended attributes are mapped onto NTFS alternate data streams named
// "xattr.<name>" on the source file. The prefix keeps them apart from
// regular streams other tools may have created, and stream-aware copy tools
// (robocopy /B, xcopy /O) carry them along with the file.
const xattrStreamPrefix = "xattr."

// xattrWriteMode is the permission mode for newly created xattr streams.
const xattrWriteMode = 0o644

var (
	modkernel32          = windows.NewLazySystemDLL("kernel32.dll")
	procFindFirstStreamW = modkernel32.NewProc("FindFirstStreamW")
	procFindNextStreamW  = modkernel32.NewProc("FindNextStreamW")
)

// win32FindStreamData mirrors WIN32_FIND_STREAM_DATA.
type win32FindStreamData struct {
	size int64
	name [296]uint16 // MAX_PATH + 36
}

// xattrStreamPath returns the ADS path backing the given attribute.
func (f *FilterFS) xattrStreamPath(fusePath, name string) string {
	return f.fullPath(fusePath) + ":" + xattrStreamPrefix + name
}

// Setxattr stores an extended attribute, honoring the CREATE/REPLACE flags.
func (f *FilterFS) Setxattr(path, name string, value []byte, flags int) int {
	if f.config.ReadOnly {
		return -fuse.EROFS
	}
	if f.shouldHide(path) {
		return -fuse.ENOENT
	}

	streamPath := f.xattrStreamPath(path, name)
	_, err := os.Stat(streamPath)
	exists := err == nil
	if flags&fuse.XATTR_CREATE != 0 && exists {
		return -fuse.EEXIST
	}
	if flags&fuse.XATTR_REPLACE != 0 && !exists {
		return -fuse.ENOATTR
	}

	return errno(os.WriteFile(streamPath, value, xattrWriteMode))
}

// Getxattr reads an extended attribute.
func (f *FilterFS) Getxattr(path, name string) (errc int, value []byte) {
	if f.shouldHide(path) {
		return -fuse.ENOENT, nil
	}

	value, err := os.ReadFile(f.xattrStreamPath(path, name))
	if err != nil {
		if os.IsNotExist(err) {
			return -fuse.ENOATTR, nil
		}
		return errno(err), nil
	}
	return 0, value
}

// Removexattr deletes an extended attribute.
func (f *FilterFS) Removexattr(path, name string) int {
	if f.config.ReadOnly {
		return -fuse.EROFS
	}
	if f.shouldHide(path) {
		return -fuse.ENOENT
	}

	err := os.Remove(f.xattrStreamPath(path, name))
	if err != nil && os.IsNotExist(err) {
		return -fuse.ENOATTR
	}
	return errno(err)
}

// Listxattr enumerates the extended attributes of a file.
func (f *FilterFS) Listxattr(path string, fill func(name string) bool) int {
	if f.shouldHide(path) {
		return -fuse.ENOENT
	}

	names, errc := listStreams(f.fullPath(path))
	if errc != 0 {
		return errc
	}

	for _, n := range names {
		if !strings.HasPrefix(n, xattrStreamPrefix) {
			continue
		}
		if !fill(strings.TrimPrefix(n, xattrStreamPrefix)) {
			return -fuse.ERANGE
		}
	}
	return 0
}

// listStreams enumerates the alternate data stream names of a file via
// FindFirstStreamW/FindNextStreamW (not yet wrapped by x/sys/windows).
func listStreams(fullPath string) (names []string, errc int) {
	pathPtr, err := windows.UTF16PtrFromString(fullPath)
	if err != nil {
		return nil, -fuse.EIO
	}

	var data win32FindStreamData
	// findStreamInfoStandard = 0, dwFlags reserved = 0
	h, _, callErr := procFindFirstStreamW.Call(
		uintptr(unsafe.Pointer(pathPtr)), 0, uintptr(unsafe.Pointer(&data)), 0)
	if windows.Handle(h) == windows.InvalidHandle {
		switch callErr {
		case windows.ERROR_HANDLE_EOF:
			return nil, 0 // no streams at all
		case windows.ERROR_FILE_NOT_FOUND, windows.ERROR_PATH_NOT_FOUND:
			return nil, -fuse.ENOENT
		default:
			return nil, -fuse.EIO
		}
	}
	defer func() { _ = windows.FindClose(windows.Handle(h)) }()

	for {
		// stream names arrive as ":name:$DATA"; the unnamed default stream is "::$DATA"
		full := windows.UTF16ToString(data.name[:])
		if parts := strings.Split(full, ":"); len(parts) > 1 && parts[1] != "" {
			names = append(names, parts[1])
		}

		ret, _, callErr := procFindNextStreamW.Call(h, uintptr(unsafe.Pointer(&data)))
		if ret == 0 {
			if callErr == windows.ERROR_HANDLE_EOF {
				return names, 0
			}
			return nil, -fuse.EIO
		}
	}
}
