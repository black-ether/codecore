package main

import (
	"path/filepath"
	"regexp"
	"strings"
)

type passageLine struct {
	text   string
	offset int
}

var markdownOrderedItem = regexp.MustCompile(`^[0-9]{1,9}[.)]([ 	]|$)`)

func isMarkdownFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown", ".mdx":
		return true
	}
	return false
}

// Consider only plain Markdown paragraphs with identical raw lines. Blank
// lines delimit paragraphs. The whole block is skipped if it contains markup
// that could change its meaning or introduce a code example.
func markdownPassages(src []byte) map[int]passageLine {
	lines := splitLines(src)
	keys := make(map[int]passageLine)
	frontMatter := strings.TrimPrefix(lines[0], "\uFEFF") == "---"
	fence := ""
	start := 0
	skip := false
	flush := func(end int) {
		if skip || start == end {
			return
		}
		text := strings.Join(lines[start:end], "\n")
		if len(text) < 32 {
			return
		}
		for i := start; i < end; i++ {
			keys[i+1] = passageLine{text: text, offset: i - start}
		}
	}
	for i, line := range lines {
		text := strings.TrimSpace(line)
		if frontMatter {
			if i > 0 && (text == "---" || text == "...") {
				frontMatter = false
			}
			start = i + 1
			continue
		}
		if fence != "" {
			trailing, closes := strings.CutPrefix(text, fence)
			if closes && strings.TrimSpace(strings.TrimLeft(trailing, fence[:1])) == "" {
				fence = ""
			}
			start = i + 1
			continue
		}
		if strings.HasPrefix(text, "```") || strings.HasPrefix(text, "~~~") {
			// Clear any adjacent block and honor the opening fence length.
			n := len(text) - len(strings.TrimLeft(text, text[:1]))
			fence, start, skip = text[:n], i+1, false
			continue
		}
		if text == "" {
			flush(i)
			start, skip = i+1, false
			continue
		}
		if line != strings.TrimLeft(line, " \t") || strings.ContainsRune("#<>|:-*+={}", rune(text[0])) || markdownOrderedItem.MatchString(text) {
			skip = true
		}
	}
	flush(len(lines))
	return keys
}

func suppressDuplicateDocs(results []retrievalResult, keys map[string]map[int]passageLine) []retrievalResult {
	preferred := make(map[passageLine]string)
	kept := results[:0]
	for _, result := range results {
		if key, ok := keys[result.File][result.Line]; ok {
			file, exists := preferred[key]
			if exists && file != result.File {
				continue
			}
			preferred[key] = result.File
		}
		kept = append(kept, result)
	}
	return kept
}
