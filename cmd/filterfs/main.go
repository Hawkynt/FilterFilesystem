// FilterFS is a FUSE-based filtering filesystem that allows mounting directories
// with configurable file and directory filtering based on glob patterns.
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
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/filterfs/filterfs/pkg/filter"
	fusefs "github.com/filterfs/filterfs/pkg/fuse"
)

// appName is the canonical binary and filesystem name.
const appName = "filterfs"

// mountPointPerm is the permission mode used when creating the mount point directory.
const mountPointPerm = 0o755

var (
	configFile string
	sourcePath string
	mountPath  string
	readOnly   bool
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

func runMount(cmd *cobra.Command, args []string) error {
	logger, err := setupLogger(logLevel)
	if err != nil {
		return fmt.Errorf("failed to setup logger: %w", err)
	}
	// Sync errors on stdout/stderr are expected and safe to ignore
	defer func() { _ = logger.Sync() }()

	config, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	err = prepareMountPoint(config, logger)
	if err != nil {
		return err
	}

	// Create FilterFS
	filterFS, err := fusefs.NewFilterFS(config, logger)
	if err != nil {
		return fmt.Errorf("failed to create FilterFS: %w", err)
	}

	logger.Info("Mounting FilterFS",
		zap.String("source", config.SourcePath),
		zap.String("mount", config.MountPath),
		zap.Bool("readonly", config.ReadOnly),
		zap.Int("blacklist_patterns", len(config.Blacklist)),
		zap.Strings("patterns", config.Blacklist))

	// Mount the filesystem
	server, err := fs.Mount(config.MountPath, filterFS, mountOptions())
	if err != nil {
		return fmt.Errorf("failed to mount filesystem: %w", err)
	}

	logger.Info("FilterFS mounted successfully")

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Handle shutdown gracefully
	go func() {
		sig := <-sigChan
		logger.Info("Received shutdown signal", zap.String("signal", sig.String()))
		if err := server.Unmount(); err != nil {
			logger.Error("Failed to unmount", zap.Error(err))
		}
	}()

	// Wait for unmount
	server.Wait()
	logger.Info("FilterFS unmounted successfully")

	return nil
}

// prepareMountPoint validates the source directory and ensures the mount point exists.
func prepareMountPoint(config *filter.Config, logger *zap.Logger) error {
	// Validate source directory exists and is accessible
	if _, err := os.Stat(config.SourcePath); err != nil {
		return fmt.Errorf("source directory not accessible: %w", err)
	}

	// Create mount point if it doesn't exist
	if err := os.MkdirAll(config.MountPath, mountPointPerm); err != nil {
		return fmt.Errorf("failed to create mount point: %w", err)
	}

	// Check if mount point is already in use
	if entries, err := os.ReadDir(config.MountPath); err == nil && len(entries) > 0 {
		logger.Warn("Mount point is not empty", zap.String("mount_path", config.MountPath))
	}

	return nil
}

// mountOptions returns the FUSE mount options used by FilterFS.
func mountOptions() *fs.Options {
	return &fs.Options{
		MountOptions: fuse.MountOptions{
			Debug:         logLevel == "debug",
			AllowOther:    true,
			FsName:        appName,
			Name:          appName,
			DisableXAttrs: true, // Disable extended attributes for better compatibility
		},
		// Set reasonable defaults for performance
		EntryTimeout:    nil, // Use system defaults
		AttrTimeout:     nil, // Use system defaults
		NegativeTimeout: nil, // Use system defaults
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

func runUnmount(cmd *cobra.Command, args []string) error {
	mountPath := args[0]

	logger, err := setupLogger("info")
	if err != nil {
		return fmt.Errorf("failed to setup logger: %w", err)
	}
	// Sync errors on stdout/stderr are expected and safe to ignore
	defer func() { _ = logger.Sync() }()

	logger.Info("Unmounting FilterFS", zap.String("mount_path", mountPath))

	// Try fusermount first (Linux/Unix)
	if err := tryFusermount(cmd.Context(), mountPath); err == nil {
		logger.Info("Successfully unmounted with fusermount")
		return nil
	}

	// Try umount as fallback
	if err := tryUmount(cmd.Context(), mountPath); err == nil {
		logger.Info("Successfully unmounted with umount")
		return nil
	}

	return fmt.Errorf("failed to unmount %s: try running 'fusermount -u %s' or 'umount %s' manually",
		mountPath, mountPath, mountPath)
}

func tryFusermount(ctx context.Context, mountPath string) error {
	return exec.CommandContext(ctx, "fusermount", "-u", mountPath).Run()
}

func tryUmount(ctx context.Context, mountPath string) error {
	return exec.CommandContext(ctx, "umount", mountPath).Run()
}
