package main

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
)

const (
	productionCategory = "production"
	docsCategory       = "docs"
	testsCategory      = "tests"
	fixturesCategory   = "fixtures"
	generatedCategory  = "generated"
)

func parseCategories(value string) (map[string]bool, error) {
	selected := make(map[string]bool)
	for category := range strings.SplitSeq(value, ",") {
		category = strings.TrimSpace(category)
		switch category {
		case productionCategory, docsCategory, testsCategory, fixturesCategory, generatedCategory:
			selected[category] = true
		default:
			return nil, fmt.Errorf("unknown category %q (choose production,docs,tests,fixtures,generated)", category)
		}
	}
	return selected, nil
}

func isDocumentationFile(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	ext := filepath.Ext(base)
	switch ext {
	case ".md", ".mdx", ".markdown", ".rst", ".adoc", ".asciidoc", ".org", ".html", ".htm":
		return true
	}
	switch strings.TrimSuffix(base, ext) {
	case "readme", "changelog", "contributing", "license", "notice":
		return ext == "" || ext == ".txt" || ext == ".text"
	}
	return false
}

func sourceCategory(path string, src []byte) string {
	normalized := strings.ToLower(filepath.ToSlash(filepath.Clean(path)))
	filename := filepath.Base(normalized)
	name := strings.TrimSuffix(filename, filepath.Ext(filename))
	generatedPath := false
	for part := range strings.SplitSeq(filepath.ToSlash(filepath.Dir(normalized)), "/") {
		switch part {
		case "testdata", "fixture", "fixtures", "__fixtures__", "snapshots", "__snapshots__":
			return fixturesCategory
		case "mock", "mocks", "__mocks__", "generated", "__generated__":
			generatedPath = true
		}
	}
	if generatedPath ||
		strings.HasPrefix(name, "mock_") || strings.HasPrefix(name, "mock-") ||
		strings.HasSuffix(name, "_mock") || strings.HasSuffix(name, ".mock") ||
		strings.HasSuffix(name, "_generated") || strings.HasSuffix(name, ".generated") ||
		strings.HasPrefix(name, "zz_generated.") || hasGeneratedHeader(src) {
		return generatedCategory
	}
	if isTestFile(normalized) {
		return testsCategory
	}
	if isDocumentationFile(path) {
		return docsCategory
	}
	return productionCategory
}

func hasGeneratedHeader(src []byte) bool {
	for line := range bytes.SplitSeq(src[:min(len(src), 8192)], []byte("\n")) {
		line = bytes.TrimSpace(line)
		if bytes.HasPrefix(line, []byte("// Code generated ")) && bytes.HasSuffix(line, []byte(" DO NOT EDIT.")) {
			return true
		}
		for _, prefix := range [...]string{"//", "#", "/*", "*", "<!--"} {
			if comment, ok := bytes.CutPrefix(line, []byte(prefix)); ok {
				if rest, marked := bytes.CutPrefix(bytes.TrimSpace(comment), []byte("@generated")); marked &&
					(len(rest) == 0 || strings.ContainsRune(" \t:*", rune(rest[0]))) {
					return true
				}
			}
		}
	}
	return false
}
