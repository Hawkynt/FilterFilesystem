//go:build linux || darwin

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

	"github.com/Hawkynt/FilterFilesystem/pkg/filter"
	fusefs "github.com/Hawkynt/FilterFilesystem/pkg/fuse"
)

// mountPointPerm is the permission mode used when creating the mount point directory.
const mountPointPerm = 0o755

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
			AllowOther:    allowOther,
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
