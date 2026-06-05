//go:build windows

package winfs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/winfsp/cgofuse/fuse"
	"go.uber.org/zap/zaptest"

	"github.com/Hawkynt/FilterFilesystem/pkg/filter"
)

// newTestFS creates a FilterFS over a temp source tree.
// Layout: public.txt, secret.log, dir1/file1.txt, dir1/hidden.tmp
func newTestFS(t *testing.T, readOnly bool, blacklist []string) (fsys *FilterFS, sourceDir string) {
	t.Helper()
	sourceDir = t.TempDir()

	files := map[string]string{
		"public.txt":      "public content",
		"secret.log":      "secret log content",
		"dir1/file1.txt":  "file1 content",
		"dir1/hidden.tmp": "hidden temp file",
	}
	for rel, content := range files {
		full := filepath.Join(sourceDir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	}

	cfg := &filter.Config{
		SourcePath: sourceDir,
		MountPath:  "X:",
		ReadOnly:   readOnly,
		Blacklist:  blacklist,
	}
	fsys, err := New(cfg, zaptest.NewLogger(t))
	require.NoError(t, err)
	return fsys, sourceDir
}

func TestGetattr(t *testing.T) {
	fsys, _ := newTestFS(t, false, []string{"**/*.log", "**/*.tmp"})

	t.Run("visible file returns its attributes", func(t *testing.T) {
		// given a visible file, when stat'ing it, then size and mode are reported
		stat := &fuse.Stat_t{}
		errc := fsys.Getattr("/public.txt", stat, ^uint64(0))
		assert.Equal(t, 0, errc)
		assert.EqualValues(t, len("public content"), stat.Size)
		assert.EqualValues(t, fuse.S_IFREG, stat.Mode&fuse.S_IFMT)
	})

	t.Run("blacklisted file reports ENOENT", func(t *testing.T) {
		stat := &fuse.Stat_t{}
		assert.Equal(t, -fuse.ENOENT, fsys.Getattr("/secret.log", stat, ^uint64(0)))
	})

	t.Run("missing file reports ENOENT", func(t *testing.T) {
		stat := &fuse.Stat_t{}
		assert.Equal(t, -fuse.ENOENT, fsys.Getattr("/no-such-file", stat, ^uint64(0)))
	})

	t.Run("root directory is a directory", func(t *testing.T) {
		stat := &fuse.Stat_t{}
		assert.Equal(t, 0, fsys.Getattr("/", stat, ^uint64(0)))
		assert.EqualValues(t, fuse.S_IFDIR, stat.Mode&fuse.S_IFMT)
	})
}

func TestReaddir(t *testing.T) {
	fsys, _ := newTestFS(t, false, []string{"**/*.log", "**/*.tmp"})

	list := func(path string) []string {
		var names []string
		errc := fsys.Readdir(path, func(name string, _ *fuse.Stat_t, _ int64) bool {
			names = append(names, name)
			return true
		}, 0, ^uint64(0))
		require.Equal(t, 0, errc)
		return names
	}

	t.Run("root listing hides blacklisted entries", func(t *testing.T) {
		names := list("/")
		assert.Contains(t, names, "public.txt")
		assert.Contains(t, names, "dir1")
		assert.NotContains(t, names, "secret.log")
	})

	t.Run("subdirectory listing hides blacklisted entries", func(t *testing.T) {
		names := list("/dir1")
		assert.Contains(t, names, "file1.txt")
		assert.NotContains(t, names, "hidden.tmp")
	})
}

func TestOpenReadWrite(t *testing.T) {
	fsys, _ := newTestFS(t, false, []string{"**/*.log"})

	t.Run("full create-write-read-release roundtrip", func(t *testing.T) {
		errc, fh := fsys.Create("/newfile.txt", fuse.O_WRONLY|fuse.O_CREAT, 0o644)
		require.Equal(t, 0, errc)
		assert.Equal(t, len("hello"), fsys.Write("/newfile.txt", []byte("hello"), 0, fh))
		assert.Equal(t, 0, fsys.Release("/newfile.txt", fh))

		errc, fh = fsys.Open("/newfile.txt", fuse.O_RDONLY)
		require.Equal(t, 0, errc)
		buff := make([]byte, 16)
		n := fsys.Read("/newfile.txt", buff, 0, fh)
		assert.Equal(t, "hello", string(buff[:n]))
		// boundary: reading past EOF yields zero bytes
		assert.Equal(t, 0, fsys.Read("/newfile.txt", buff, 100, fh))
		assert.Equal(t, 0, fsys.Release("/newfile.txt", fh))
	})

	t.Run("blacklisted file cannot be opened", func(t *testing.T) {
		errc, _ := fsys.Open("/secret.log", fuse.O_RDONLY)
		assert.Equal(t, -fuse.ENOENT, errc)
	})

	t.Run("blacklisted file cannot be created", func(t *testing.T) {
		errc, _ := fsys.Create("/evil.log", fuse.O_WRONLY|fuse.O_CREAT, 0o644)
		assert.Equal(t, -fuse.EPERM, errc)
	})

	t.Run("stale handle reports EBADF", func(t *testing.T) {
		assert.Equal(t, -fuse.EBADF, fsys.Read("/public.txt", make([]byte, 4), 0, 9999))
		assert.Equal(t, -fuse.EBADF, fsys.Release("/public.txt", 9999))
	})
}

func TestReadOnlyMode(t *testing.T) {
	fsys, _ := newTestFS(t, true, nil)

	t.Run("reads succeed", func(t *testing.T) {
		errc, fh := fsys.Open("/public.txt", fuse.O_RDONLY)
		require.Equal(t, 0, errc)
		defer fsys.Release("/public.txt", fh)
		buff := make([]byte, 32)
		n := fsys.Read("/public.txt", buff, 0, fh)
		assert.Equal(t, "public content", string(buff[:n]))
	})

	t.Run("every mutation is refused with EROFS", func(t *testing.T) {
		errc, _ := fsys.Open("/public.txt", fuse.O_WRONLY)
		assert.Equal(t, -fuse.EROFS, errc)
		errc, _ = fsys.Create("/new.txt", fuse.O_WRONLY|fuse.O_CREAT, 0o644)
		assert.Equal(t, -fuse.EROFS, errc)
		assert.Equal(t, -fuse.EROFS, fsys.Mkdir("/newdir", 0o755))
		assert.Equal(t, -fuse.EROFS, fsys.Unlink("/public.txt"))
		assert.Equal(t, -fuse.EROFS, fsys.Rmdir("/dir1"))
		assert.Equal(t, -fuse.EROFS, fsys.Rename("/public.txt", "/renamed.txt"))
		assert.Equal(t, -fuse.EROFS, fsys.Truncate("/public.txt", 0, ^uint64(0)))
		assert.Equal(t, -fuse.EROFS, fsys.Chmod("/public.txt", 0o600))
	})
}

func TestDirectoryPolicies(t *testing.T) {
	t.Run("directory with hidden children cannot be removed by default", func(t *testing.T) {
		fsys, _ := newTestFS(t, false, []string{"**/*.tmp"})
		// dir1 contains hidden.tmp which is blacklisted
		assert.Equal(t, -fuse.EPERM, fsys.Rmdir("/dir1"))
	})

	t.Run("directory with hidden children cannot be renamed by default", func(t *testing.T) {
		fsys, _ := newTestFS(t, false, []string{"**/*.tmp"})
		assert.Equal(t, -fuse.EPERM, fsys.Rename("/dir1", "/dir2"))
	})

	t.Run("rename to a blacklisted name is refused", func(t *testing.T) {
		fsys, _ := newTestFS(t, false, []string{"**/*.log"})
		assert.Equal(t, -fuse.EPERM, fsys.Rename("/public.txt", "/public.log"))
	})

	t.Run("mkdir of a blacklisted name is refused", func(t *testing.T) {
		fsys, _ := newTestFS(t, false, []string{"**/temp"})
		assert.Equal(t, -fuse.EPERM, fsys.Mkdir("/temp", 0o755))
	})

	t.Run("unlink of a hidden file reports ENOENT", func(t *testing.T) {
		fsys, sourceDir := newTestFS(t, false, []string{"**/*.log"})
		assert.Equal(t, -fuse.ENOENT, fsys.Unlink("/secret.log"))
		// and the source file is untouched
		_, err := os.Stat(filepath.Join(sourceDir, "secret.log"))
		assert.NoError(t, err)
	})
}

func TestStatfs(t *testing.T) {
	fsys, _ := newTestFS(t, false, nil)

	// given a mounted source volume, when querying statfs,
	// then plausible non-zero capacity figures are reported
	stat := &fuse.Statfs_t{}
	require.Equal(t, 0, fsys.Statfs("/", stat))
	assert.NotZero(t, stat.Bsize)
	assert.NotZero(t, stat.Frsize)
	assert.NotZero(t, stat.Blocks)
	assert.NotZero(t, stat.Namemax)
	// boundary: free space never exceeds total, available never exceeds free
	assert.LessOrEqual(t, stat.Bfree, stat.Blocks)
	assert.LessOrEqual(t, stat.Bavail, stat.Bfree)
}

func TestTruncate(t *testing.T) {
	fsys, sourceDir := newTestFS(t, false, nil)

	// given an existing file, when truncating by path, then the size shrinks
	assert.Equal(t, 0, fsys.Truncate("/public.txt", 3, ^uint64(0)))
	content, err := os.ReadFile(filepath.Join(sourceDir, "public.txt"))
	require.NoError(t, err)
	assert.Equal(t, "pub", string(content))
}
