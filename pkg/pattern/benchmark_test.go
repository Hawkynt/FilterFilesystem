package pattern

import (
	"testing"
)

func BenchmarkMatcher_IsBlacklisted(b *testing.B) {
	patterns := []string{
		"**/*.log",
		"**/*.tmp",
		"**/.git",
		"**/node_modules",
		"**/*.cache",
		"**/temp",
		"**/*.bak",
		"**/.*",
	}

	matcher := NewMatcher(patterns)
	testPaths := []string{
		"documents/project/src/main.go",
		"logs/app.log",
		"temp/cache.tmp",
		"project/.git/config",
		"src/components/Button.tsx",
		"node_modules/react/index.js",
		"backup/data.bak",
		"images/.DS_Store",
		"very/deep/nested/directory/structure/file.txt",
		"another/deep/path/with/many/levels/document.pdf",
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		path := testPaths[i%len(testPaths)]
		matcher.IsBlacklisted(path)
	}
}

func BenchmarkMatcher_WouldCreateBlacklisted(b *testing.B) {
	patterns := []string{
		"**/*.log",
		"**/*.tmp",
		"**/.git",
		"**/temp",
	}

	matcher := NewMatcher(patterns)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		matcher.WouldCreateBlacklisted("/home/user/documents", "app.log")
	}
}

func BenchmarkMatcher_SinglePattern(b *testing.B) {
	matcher := NewMatcher([]string{"**/*.log"})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		matcher.IsBlacklisted("path/to/logfile.log")
	}
}

func BenchmarkMatcher_ManyPatterns(b *testing.B) {
	patterns := make([]string, 100)
	for i := 0; i < 100; i++ {
		patterns[i] = "**/*.ext" + string(rune('a'+i%26))
	}

	matcher := NewMatcher(patterns)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		matcher.IsBlacklisted("path/to/file.exta")
	}
}

func BenchmarkMatcher_DeepPath(b *testing.B) {
	matcher := NewMatcher([]string{"**/*.log"})
	deepPath := "level1/level2/level3/level4/level5/level6/level7/level8/level9/level10/app.log"

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		matcher.IsBlacklisted(deepPath)
	}
}
