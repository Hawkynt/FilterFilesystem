// Package filter provides configuration management for FilterFS.
package filter

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the FilterFS configuration settings.
// It defines how the filesystem should be mounted and what filtering rules to apply.
type Config struct {
	// SourcePath is the path to the source directory to be filtered
	SourcePath string `yaml:"source_path"`

	// MountPath is the path where the filtered filesystem will be mounted
	MountPath string `yaml:"mount_path"`

	// ReadOnly determines if the filesystem should be mounted in read-only mode
	ReadOnly bool `yaml:"read_only"`

	// Blacklist contains the patterns for files/directories to hide
	Blacklist []string `yaml:"blacklist"`

	// AllowDelete controls whether directories containing hidden files can be deleted
	AllowDelete bool `yaml:"allow_delete_with_hidden"`

	// AllowRename controls whether directories containing hidden files can be renamed
	AllowRename bool `yaml:"allow_rename_with_hidden"`
}

// LoadConfig reads and parses a FilterFS configuration file from the given path.
// The configuration file should be in YAML format.
//
// Returns a validated Config struct or an error if the file cannot be read,
// parsed, or contains invalid settings.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &config, nil
}

// Validate checks if the configuration is valid and complete.
// It verifies that required fields are set and that the source directory exists.
//
// Returns an error if any validation fails.
func (c *Config) Validate() error {
	if c.SourcePath == "" {
		return fmt.Errorf("source_path is required")
	}

	if c.MountPath == "" {
		return fmt.Errorf("mount_path is required")
	}

	// Check if source path exists
	if _, err := os.Stat(c.SourcePath); err != nil {
		return fmt.Errorf("source path does not exist: %w", err)
	}

	return nil
}

// DefaultConfig returns a Config struct with sensible default values.
// All boolean options are set to their safe defaults (false),
// and string/slice fields are initialized to empty values.
func DefaultConfig() *Config {
	return &Config{
		ReadOnly:    false,
		AllowDelete: false,
		AllowRename: false,
		Blacklist:   []string{},
	}
}
