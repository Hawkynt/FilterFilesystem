// FilterFS is a filtering filesystem that allows mounting directories with
// configurable file and directory filtering based on glob patterns. It uses
// FUSE on Linux/macOS and WinFsp on Windows.
//
// Usage:
//
//	filterfs mount -s /source/path -m /mount/point -b "**/*.log" -b "**/.git"
//	filterfs unmount /mount/point
//
// The filesystem supports both read-only and read-write modes, with comprehensive
// pattern matching for blacklisting files and directories.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/Hawkynt/FilterFilesystem/pkg/filter"
)

// appName is the canonical binary and filesystem name.
const appName = "filterfs"

var (
	configFile string
	sourcePath string
	mountPath  string
	readOnly   bool
	allowOther bool
	blacklist  []string
	logLevel   string
)

var rootCmd = &cobra.Command{
	Use:   appName,
	Short: "FilterFS - A filtering filesystem",
	Long: `FilterFS allows you to mount a filesystem with filtered files and directories.
It supports blacklisting files based on patterns and controlling access modes.`,
}

var mountCmd = &cobra.Command{
	Use:   "mount",
	Short: "Mount a filtered filesystem",
	Long: `Mount a directory with filtered content based on blacklist patterns.
The filtered filesystem will hide files and directories matching the specified patterns.`,
	RunE: runMount,
}

var unmountCmd = &cobra.Command{
	Use:   "unmount <mount-path>",
	Short: "Unmount a FilterFS filesystem",
	Long:  `Unmount a previously mounted FilterFS filesystem at the specified mount point.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runUnmount,
}

func init() {
	rootCmd.AddCommand(mountCmd)
	rootCmd.AddCommand(unmountCmd)

	mountCmd.Flags().StringVarP(&configFile, "config", "c", "", "Configuration file path")
	mountCmd.Flags().StringVarP(&sourcePath, "source", "s", "", "Source directory to filter")
	mountCmd.Flags().StringVarP(&mountPath, "mount", "m", "", "Mount point for filtered filesystem")
	mountCmd.Flags().BoolVarP(&readOnly, "readonly", "r", false, "Mount as read-only")
	mountCmd.Flags().BoolVar(&allowOther, "allow-other", false,
		"Allow other users to access the mount (requires 'user_allow_other' in /etc/fuse.conf; no-op on Windows)")
	mountCmd.Flags().StringSliceVarP(&blacklist, "blacklist", "b", []string{}, "Blacklist patterns")
	mountCmd.Flags().StringVar(&logLevel, "log-level", "info", "Log level (debug, info, warn, error)")

	// Mark required flags
	mountCmd.MarkFlagsRequiredTogether("source", "mount")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func loadConfig() (*filter.Config, error) {
	if configFile != "" {
		return filter.LoadConfig(configFile)
	}

	// Build config from command line flags
	config := filter.DefaultConfig()

	if sourcePath == "" {
		return nil, fmt.Errorf("source path is required")
	}
	config.SourcePath = sourcePath

	if mountPath == "" {
		return nil, fmt.Errorf("mount path is required")
	}
	config.MountPath = mountPath

	config.ReadOnly = readOnly
	config.Blacklist = blacklist

	if err := config.Validate(); err != nil {
		return nil, err
	}

	return config, nil
}

func setupLogger(level string) (*zap.Logger, error) {
	var zapLevel zapcore.Level
	if err := zapLevel.UnmarshalText([]byte(level)); err != nil {
		return nil, fmt.Errorf("invalid log level: %w", err)
	}

	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(zapLevel)
	config.OutputPaths = []string{"stdout"}
	config.ErrorOutputPaths = []string{"stderr"}

	return config.Build()
}
