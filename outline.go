// Package main implements codecore, a fast structural outline extractor.
//
// The tool is split into three layers that never bleed into each other:
//
//   - extraction: an Extractor turns source bytes into an Outline by
//     folding blocks — indentation for Python, brackets for Go and
//     PowerShell (fold.go, fold_python.go, fold_braces.go). No kind
//     classification, no parsing: every code line is a node, and a line
//     that opens a multi-line block is a node with children. That is the
//     whole model, the same as manually collapsing every code block in
//     an editor and looking at the big picture.
//   - model: Node carries a 1-based line number, the line's text, and
//     its children in source order. Outline carries the file, the
//     language, and Unbalanced, set when brackets did not balance (a
//     stray closer, or a block left open at end of file): the outline
//     is then partial rather than failing.
//   - formatting: a Formatter renders Outlines. Text and JSON ship now;
//     adding a format means implementing one method, never touching an
//     extractor.
package main

import (
	"io"
	"path/filepath"
	"sort"
	"strings"
)

// Node is one line in the structural outline: its 1-based line number,
// the line's text (trimmed, capped), and the lines nested inside it in
// source order. A node with children is a block header — a line whose
// block stays open past the end of the line. A node without children is
// a leaf: a plain line, or a block that opens and closes on one line.
type Node struct {
	Line     int     `json:"line"`
	Text     string  `json:"text"`
	Children []*Node `json:"children,omitempty"`
}

// Outline is the extraction result for one file. Unbalanced is set when
// the file's brackets did not balance: the outline is then partial
// rather than failing. DefaultDepth is the depth the CLI shows when
// the user does not pass --depth: 0 for most languages (top level
// only), 2 for HTML/XML, whose top level is a single wrapper tag
// and 0 would show almost nothing. It is a display concern, not
// part of the outline data, so it is excluded from JSON output.
type Outline struct {
	File         string  `json:"file"`
	Language     string  `json:"language"`
	Unbalanced   bool    `json:"unbalanced,omitempty"`
	DefaultDepth int     `json:"-"`
	Nodes        []*Node `json:"nodes"`
}

// Extractor turns source bytes into an Outline. Implementations must be
// deterministic: the same input bytes always produce the same outline, in
// source order. They must not read the file themselves; the CLI owns I/O,
// so extractors stay pure and testable.
type Extractor interface {
	Language() string     // e.g. "go"
	Extensions() []string // e.g. []string{".go"}
	Extract(filename string, src []byte) (*Outline, error)
}

// Formatter renders Outlines. Extraction and formatting are separate, so a
// new output shape never touches the extractors.
type Formatter interface {
	Format(w io.Writer, outlines []*Outline) error
}

var (
	extractors = map[string]Extractor{}
	byLanguage = map[string]Extractor{}
)

// RegisterExtractor makes e available for its extensions and language
// name. Called from a language file's init, e.g. fold.go.
func RegisterExtractor(e Extractor) {
	for _, ext := range e.Extensions() {
		extractors[strings.ToLower(ext)] = e
	}
	byLanguage[strings.ToLower(e.Language())] = e
}

// ExtractorFor returns the extractor for a file's extension.
func ExtractorFor(filename string) (Extractor, bool) {
	e, ok := extractors[strings.ToLower(filepath.Ext(filename))]
	return e, ok
}

// ExtractorByLanguage returns the extractor registered under a language
// name such as "go".
func ExtractorByLanguage(name string) (Extractor, bool) {
	e, ok := byLanguage[strings.ToLower(name)]
	return e, ok
}

// Languages returns the registered language names, sorted.
func Languages() []string {
	names := make([]string, 0, len(byLanguage))
	for n := range byLanguage {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
