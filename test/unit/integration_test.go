package unit

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	fs2 "github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/filterfs/filterfs/pkg/filter"
	fusefs "github.com/filterfs/filterfs/pkg/fuse"
)

func TestFilterFS_Integration(t *testing.T) {
	// Create temporary directories for testing
	sourceDir := t.TempDir()
	mountDir := t.TempDir()

	// Create test structure in source directory
	testFiles := map[string]string{
		"public.txt":          "public content",
		"secret.log":          "secret log content",
		"dir1/file1.txt":      "file1 content",
		"dir1/hidden.tmp":     "hidden temp file",
		"dir2/subdir/app.log": "application log",
		"dir2/data.json":      "json data",
		".git/config":         "git config",
		"temp/cache.dat":      "cache data",
	}

	for path, content := range testFiles {
		fullPath := filepath.Join(sourceDir, path)
		err := os.MkdirAll(filepath.Dir(fullPath), 0755)
		require.NoError(t, err)
		err = os.WriteFile(fullPath, []byte(content), 0644)
		require.NoError(t, err)
	}

	// Create config with blacklist patterns
	config := &filter.Config{
		SourcePath: sourceDir,
		MountPath:  mountDir,
		ReadOnly:   false,
		Blacklist: []string{
			"**/*.log", // Hide all log files
			"**/*.tmp", // Hide all temp files
			"**/.git",  // Hide git directories
			"**/temp",  // Hide temp directories
		},
		AllowDelete: false,
		AllowRename: false,
	}

	logger := zaptest.NewLogger(t)

	// Create and mount FilterFS
	filterFS, err := fusefs.NewFilterFS(config, logger)
	require.NoError(t, err)

	opts := &fs2.Options{
		MountOptions: fuse.MountOptions{
			Debug:      false,
			AllowOther: false,
			FsName:     "filterfs-test",
		},
	}

	server, err := fs2.Mount(mountDir, filterFS, opts)
	require.NoError(t, err)

	// Give the mount time to initialize
	time.Sleep(100 * time.Millisecond)

	defer func() {
		err := server.Unmount()
		if err != nil {
			t.Logf("Failed to unmount: %v", err)
		}
	}()

	t.Run("Visible files are accessible", func(t *testing.T) {
		// Check that non-blacklisted files are visible
		content, err := os.ReadFile(filepath.Join(mountDir, "public.txt"))
		assert.NoError(t, err)
		assert.Equal(t, "public content", string(content))

		content, err = os.ReadFile(filepath.Join(mountDir, "dir1", "file1.txt"))
		assert.NoError(t, err)
		assert.Equal(t, "file1 content", string(content))

		content, err = os.ReadFile(filepath.Join(mountDir, "dir2", "data.json"))
		assert.NoError(t, err)
		assert.Equal(t, "json data", string(content))
	})

	t.Run("Blacklisted files are hidden", func(t *testing.T) {
		// Check that blacklisted files are not visible
		_, err := os.ReadFile(filepath.Join(mountDir, "secret.log"))
		assert.Error(t, err)
		assert.True(t, os.IsNotExist(err))

		_, err = os.ReadFile(filepath.Join(mountDir, "dir1", "hidden.tmp"))
		assert.Error(t, err)
		assert.True(t, os.IsNotExist(err))

		_, err = os.ReadFile(filepath.Join(mountDir, "dir2", "subdir", "app.log"))
		assert.Error(t, err)
		assert.True(t, os.IsNotExist(err))

		_, err = os.Stat(filepath.Join(mountDir, ".git"))
		assert.Error(t, err)
		assert.True(t, os.IsNotExist(err))

		_, err = os.Stat(filepath.Join(mountDir, "temp"))
		assert.Error(t, err)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("Directory listing filters content", func(t *testing.T) {
		// Check root directory
		entries, err := os.ReadDir(mountDir)
		assert.NoError(t, err)

		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}

		assert.Contains(t, names, "public.txt")
		assert.Contains(t, names, "dir1")
		assert.Contains(t, names, "dir2")
		assert.NotContains(t, names, "secret.log")
		assert.NotContains(t, names, ".git")
		assert.NotContains(t, names, "temp")

		// Check subdirectory
		entries, err = os.ReadDir(filepath.Join(mountDir, "dir1"))
		assert.NoError(t, err)

		names = make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}

		assert.Contains(t, names, "file1.txt")
		assert.NotContains(t, names, "hidden.tmp")
	})

	t.Run("File operations work on visible files", func(t *testing.T) {
		// Create a new file
		testFile := filepath.Join(mountDir, "newfile.txt")
		err := os.WriteFile(testFile, []byte("new content"), 0644)
		assert.NoError(t, err)

		// Read it back
		content, err := os.ReadFile(testFile)
		assert.NoError(t, err)
		assert.Equal(t, "new content", string(content))

		// Modify it
		err = os.WriteFile(testFile, []byte("modified content"), 0644)
		assert.NoError(t, err)

		content, err = os.ReadFile(testFile)
		assert.NoError(t, err)
		assert.Equal(t, "modified content", string(content))

		// Delete it
		err = os.Remove(testFile)
		assert.NoError(t, err)

		_, err = os.Stat(testFile)
		assert.Error(t, err)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("Cannot create blacklisted files", func(t *testing.T) {
		// Try to create a .log file
		err := os.WriteFile(filepath.Join(mountDir, "test.log"), []byte("log content"), 0644)
		assert.Error(t, err)

		// Try to create a .tmp file
		err = os.WriteFile(filepath.Join(mountDir, "test.tmp"), []byte("tmp content"), 0644)
		assert.Error(t, err)

		// Try to create a temp directory
		err = os.Mkdir(filepath.Join(mountDir, "temp"), 0755)
		assert.Error(t, err)
	})
}

func TestFilterFS_ReadOnlyMode(t *testing.T) {
	sourceDir := t.TempDir()
	mountDir := t.TempDir()

	// Create a test file
	testFile := filepath.Join(sourceDir, "readonly.txt")
	err := os.WriteFile(testFile, []byte("readonly content"), 0644)
	require.NoError(t, err)

	config := &filter.Config{
		SourcePath: sourceDir,
		MountPath:  mountDir,
		ReadOnly:   true,
		Blacklist:  []string{},
	}

	logger := zaptest.NewLogger(t)
	filterFS, err := fusefs.NewFilterFS(config, logger)
	require.NoError(t, err)

	opts := &fs2.Options{
		MountOptions: fuse.MountOptions{
			Debug:      false,
			AllowOther: false,
			FsName:     "filterfs-readonly-test",
		},
	}

	server, err := fs2.Mount(mountDir, filterFS, opts)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	defer func() {
		err := server.Unmount()
		if err != nil {
			t.Logf("Failed to unmount: %v", err)
		}
	}()

	t.Run("Can read files in readonly mode", func(t *testing.T) {
		content, err := os.ReadFile(filepath.Join(mountDir, "readonly.txt"))
		assert.NoError(t, err)
		assert.Equal(t, "readonly content", string(content))
	})

	t.Run("Cannot write files in readonly mode", func(t *testing.T) {
		err := os.WriteFile(filepath.Join(mountDir, "newfile.txt"), []byte("new content"), 0644)
		assert.Error(t, err)

		err = os.WriteFile(filepath.Join(mountDir, "readonly.txt"), []byte("modified"), 0644)
		assert.Error(t, err)
	})

	t.Run("Cannot create directories in readonly mode", func(t *testing.T) {
		err := os.Mkdir(filepath.Join(mountDir, "newdir"), 0755)
		assert.Error(t, err)
	})

	t.Run("Cannot delete files in readonly mode", func(t *testing.T) {
		err := os.Remove(filepath.Join(mountDir, "readonly.txt"))
		assert.Error(t, err)
	})
}
