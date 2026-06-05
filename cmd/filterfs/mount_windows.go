//go:build windows

package main

import (
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
	"github.com/winfsp/cgofuse/fuse"
	"go.uber.org/zap"

	"github.com/Hawkynt/FilterFilesystem/pkg/filter"
	"github.com/Hawkynt/FilterFilesystem/pkg/winfs"
)

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

	err = validateMountPoint(config)
	if err != nil {
		return err
	}

	filterFS, err := winfs.New(config, logger)
	if err != nil {
		return fmt.Errorf("failed to create FilterFS: %w", err)
	}

	logger.Info("Mounting FilterFS (WinFsp)",
		zap.String("source", config.SourcePath),
		zap.String("mount", config.MountPath),
		zap.Bool("readonly", config.ReadOnly),
		zap.Int("blacklist_patterns", len(config.Blacklist)),
		zap.Strings("patterns", config.Blacklist))

	host := fuse.NewFileSystemHost(filterFS)
	host.SetCapReaddirPlus(true)

	// Handle Ctrl+C gracefully
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	go func() {
		<-sigChan
		logger.Info("Received shutdown signal")
		host.Unmount()
	}()

	// Mount blocks until the filesystem is unmounted.
	if !host.Mount(config.MountPath, nil) {
		return fmt.Errorf("failed to mount filesystem at %s (is WinFsp installed? https://winfsp.dev)",
			config.MountPath)
	}

	logger.Info("FilterFS unmounted successfully")
	return nil
}

// validateMountPoint checks the source directory; unlike on Unix the mount
// point must NOT pre-exist — WinFsp creates it (drive letter or directory).
func validateMountPoint(config *filter.Config) error {
	if _, err := os.Stat(config.SourcePath); err != nil {
		return fmt.Errorf("source directory not accessible: %w", err)
	}

	if info, err := os.Stat(config.MountPath); err == nil && info.IsDir() {
		return fmt.Errorf("mount point %s must not exist on Windows (WinFsp creates it); "+
			"use an unused drive letter like X: or a non-existing directory", config.MountPath)
	}

	return nil
}

func runUnmount(cmd *cobra.Command, args []string) error {
	return fmt.Errorf("the unmount command is not supported on Windows; " +
		"stop the running filterfs process (Ctrl+C) to unmount")
}
