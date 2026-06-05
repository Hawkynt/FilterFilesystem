package pattern

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatcher_IsBlacklisted(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		path     string
		expected bool
	}{
		{
			name:     "First sublevel match",
			patterns: []string{"*/hidden.txt"},
			path:     "dir1/hidden.txt",
			expected: true,
		},
		{
			name:     "First sublevel no match deep",
			patterns: []string{"*/hidden.txt"},
			path:     "dir1/dir2/hidden.txt",
			expected: false,
		},
		{
			name:     "Any sublevel match",
			patterns: []string{"**/secret.conf"},
			path:     "dir1/dir2/dir3/secret.conf",
			expected: true,
		},
		{
			name:     "Extension match",
			patterns: []string{"/**/*.log"},
			path:     "var/logs/app.log",
			expected: true,
		},
		{
			name:     "Multiple patterns",
			patterns: []string{"*/temp/*", "**/*.tmp", "/**/*.cache"},
			path:     "data/temp/file.dat",
			expected: true,
		},
		{
			name:     "No match",
			patterns: []string{"*.log", "*/hidden/*"},
			path:     "src/main.go",
			expected: false,
		},
		{
			name:     "Exact filename match",
			patterns: []string{"**/Thumbs.db"},
			path:     "photos/vacation/Thumbs.db",
			expected: true,
		},
		{
			name:     "Directory match",
			patterns: []string{"**/.git"},
			path:     "project/.git",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMatcher(tt.patterns)
			result := m.IsBlacklisted(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMatcher_WouldCreateBlacklisted(t *testing.T) {
	m := NewMatcher([]string{"**/*.tmp", "**/temp"})

	tests := []struct {
		parent   string
		name     string
		expected bool
	}{
		{
			parent:   "/home/user",
			name:     "file.tmp",
			expected: true,
		},
		{
			parent:   "/var",
			name:     "temp",
			expected: true,
		},
		{
			parent:   "/home/user",
			name:     "document.txt",
			expected: false,
		},
	}

	for _, tt := range tests {
		result := m.WouldCreateBlacklisted(tt.parent, tt.name)
		assert.Equal(t, tt.expected, result)
	}
}
