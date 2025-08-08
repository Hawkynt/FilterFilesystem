// Package pattern provides pattern matching functionality for FilterFS.
// It supports glob-style patterns for blacklisting files and directories.
package pattern

import (
	"path/filepath"
	"strings"
)

// Matcher provides pattern matching against a set of blacklist patterns.
// It supports various glob patterns including:
//   - */filename - matches files in first sublevel only
//   - **/filename - matches files at any level
//   - /**/*.ext - matches files with extension from root
//   - **/*.ext - matches files with extension anywhere
type Matcher struct {
	patterns []Pattern
}

// Pattern represents a single blacklist pattern with its parsed form.
type Pattern struct {
	original string // The original pattern string
	regex    string // The processed pattern for matching
	isGlob   bool   // Whether this uses standard glob matching
}

// NewMatcher creates a new pattern matcher with the given blacklist patterns.
// Each pattern string is parsed and optimized for efficient matching.
//
// Example patterns:
//   - "**/*.log" - matches all .log files at any depth
//   - "*/temp" - matches temp directories in immediate subdirectories only
//   - "/**/.git" - matches .git directories from root
func NewMatcher(patterns []string) *Matcher {
	m := &Matcher{
		patterns: make([]Pattern, 0, len(patterns)),
	}
	
	for _, p := range patterns {
		m.patterns = append(m.patterns, parsePattern(p))
	}
	
	return m
}

// parsePattern converts a glob pattern string into an optimized Pattern struct.
// It handles special cases like ** wildcards and normalizes the pattern for matching.
func parsePattern(pattern string) Pattern {
	p := Pattern{
		original: pattern,
		isGlob:   true,
	}
	
	// Convert pattern to filepath.Match compatible format
	if strings.HasPrefix(pattern, "**/") {
		// Match at any level
		p.regex = pattern[3:]
		p.isGlob = false
	} else if strings.HasPrefix(pattern, "*/") {
		// Match only at first sublevel
		p.regex = pattern
	} else if strings.HasPrefix(pattern, "/**/") {
		// Match at any sublevel from root
		p.regex = pattern[4:]
		p.isGlob = false
	} else {
		p.regex = pattern
	}
	
	return p
}

// IsBlacklisted checks if the given path matches any of the blacklist patterns.
// The path should be relative to the filesystem root and use forward slashes.
//
// Returns true if the path matches any pattern, false otherwise.
func (m *Matcher) IsBlacklisted(path string) bool {
	// Normalize path separators
	path = filepath.ToSlash(path)
	
	for _, pattern := range m.patterns {
		if m.matchesPattern(path, pattern) {
			return true
		}
	}
	
	return false
}

// matchesPattern tests if a path matches a specific pattern.
// It handles both glob patterns and ** wildcard patterns efficiently.
func (m *Matcher) matchesPattern(path string, pattern Pattern) bool {
	if pattern.isGlob {
		// Standard glob matching
		matched, _ := filepath.Match(pattern.regex, path)
		if matched {
			return true
		}
		
		// For patterns like "*/file", check each directory level
		if strings.HasPrefix(pattern.regex, "*/") {
			parts := strings.Split(path, "/")
			if len(parts) >= 2 {
				// Match against the pattern at each level
				for i := 1; i < len(parts); i++ {
					testPath := strings.Join(parts[:i], "/") + "/" + parts[i]
					if matched, _ := filepath.Match(pattern.regex, testPath); matched {
						return true
					}
				}
			}
		}
	} else {
		// For ** patterns (any level matching)
		parts := strings.Split(path, "/")
		
		// Check if the pattern matches any part of the path
		for i := 0; i < len(parts); i++ {
			for j := i; j < len(parts); j++ {
				// Try matching subpaths at different levels
				subPath := strings.Join(parts[i:j+1], "/")
				if matched, _ := filepath.Match(pattern.regex, subPath); matched {
					return true
				}
				
				// For extension patterns like "*.log"
				if strings.HasPrefix(pattern.regex, "*.") {
					if matched, _ := filepath.Match(pattern.regex, parts[j]); matched {
						return true
					}
				}
			}
		}
		
		// Direct filename/dirname matching
		for _, part := range parts {
			if matched, _ := filepath.Match(pattern.regex, part); matched {
				return true
			}
		}
		
		// Full path matching for cases like "dir/file"
		if matched, _ := filepath.Match(pattern.regex, path); matched {
			return true
		}
	}
	
	return false
}

// WouldCreateBlacklisted checks if creating a file or directory with the given name
// in the specified parent path would result in a blacklisted item.
//
// This is used to prevent creation of files/directories that match blacklist patterns.
func (m *Matcher) WouldCreateBlacklisted(parentPath, name string) bool {
	fullPath := filepath.Join(parentPath, name)
	return m.IsBlacklisted(fullPath)
}

// HasBlacklistedChildren checks if a directory contains any blacklisted items
// by scanning the provided directory entries.
func (m *Matcher) HasBlacklistedChildren(dirPath string, entries []string) bool {
	for _, entry := range entries {
		childPath := filepath.Join(dirPath, entry)
		if m.IsBlacklisted(childPath) {
			return true
		}
	}
	return false
}