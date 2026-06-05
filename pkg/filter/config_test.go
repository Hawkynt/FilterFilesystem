package filter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("Valid config file", func(t *testing.T) {
		configContent := `
source_path: /tmp/source
mount_path: /tmp/mount
read_only: true
blacklist:
  - "**/*.log"
  - "**/.git"
allow_delete_with_hidden: false
allow_rename_with_hidden: false
`
		configFile := filepath.Join(tempDir, "config.yaml")
		err := os.WriteFile(configFile, []byte(configContent), 0644)
		require.NoError(t, err)

		// Create source directory so validation passes
		sourceDir := filepath.Join(tempDir, "source")
		err = os.MkdirAll(sourceDir, 0755)
		require.NoError(t, err)

		// Update config to use temp directory
		configContent = `
source_path: ` + sourceDir + `
mount_path: /tmp/mount
read_only: true
blacklist:
  - "**/*.log"
  - "**/.git"
allow_delete_with_hidden: false
allow_rename_with_hidden: false
`
		err = os.WriteFile(configFile, []byte(configContent), 0644)
		require.NoError(t, err)

		config, err := LoadConfig(configFile)
		assert.NoError(t, err)
		assert.Equal(t, sourceDir, config.SourcePath)
		assert.Equal(t, "/tmp/mount", config.MountPath)
		assert.True(t, config.ReadOnly)
		assert.Equal(t, []string{"**/*.log", "**/.git"}, config.Blacklist)
		assert.False(t, config.AllowDelete)
		assert.False(t, config.AllowRename)
	})

	t.Run("Invalid YAML", func(t *testing.T) {
		configContent := `
source_path: /tmp/source
mount_path: /tmp/mount
read_only: true
blacklist:
  - "**/*.log"
  - "**/.git
`
		configFile := filepath.Join(tempDir, "invalid.yaml")
		err := os.WriteFile(configFile, []byte(configContent), 0644)
		require.NoError(t, err)

		_, err = LoadConfig(configFile)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse config")
	})

	t.Run("Missing file", func(t *testing.T) {
		_, err := LoadConfig("/nonexistent/config.yaml")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read config file")
	})
}

func TestConfig_Validate(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("Valid config", func(t *testing.T) {
		sourceDir := filepath.Join(tempDir, "source")
		err := os.MkdirAll(sourceDir, 0755)
		require.NoError(t, err)

		config := &Config{
			SourcePath: sourceDir,
			MountPath:  "/tmp/mount",
			ReadOnly:   true,
			Blacklist:  []string{"**/*.log"},
		}

		err = config.Validate()
		assert.NoError(t, err)
	})

	t.Run("Missing source path", func(t *testing.T) {
		config := &Config{
			MountPath: "/tmp/mount",
		}

		err := config.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "source_path is required")
	})

	t.Run("Missing mount path", func(t *testing.T) {
		config := &Config{
			SourcePath: "/tmp/source",
		}

		err := config.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "mount_path is required")
	})

	t.Run("Nonexistent source path", func(t *testing.T) {
		config := &Config{
			SourcePath: "/nonexistent/path",
			MountPath:  "/tmp/mount",
		}

		err := config.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "source path does not exist")
	})
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assert.False(t, config.ReadOnly)
	assert.False(t, config.AllowDelete)
	assert.False(t, config.AllowRename)
	assert.Empty(t, config.Blacklist)
	assert.Empty(t, config.SourcePath)
	assert.Empty(t, config.MountPath)
}
