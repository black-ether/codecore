package main

import (
	"strings"
)

// maxText is the longest a node's text may be; longer lines are capped
// with an ellipsis. The full line stays in the source file — the outline
// is a map, not a copy.
const maxText = 120

// cleanText trims a source line and caps it at maxText characters.
func cleanText(line string) string {
	t := strings.TrimSpace(line)
	if len(t) > maxText {
		t = t[:maxText] + "…"
	}
	return t
}

// splitLines splits source into lines, tolerating CRLF files.
func splitLines(src []byte) []string {
	lines := strings.Split(string(src), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSuffix(l, "\r")
	}
	return lines
}

// foldExtractor is the one extractor for all three languages: it folds
// blocks by indentation (Python) or by brackets (Go, PowerShell) and
// returns the resulting tree of lines.
type foldExtractor struct {
	language   string
	extensions []string
}

func (e foldExtractor) Language() string     { return e.language }
func (e foldExtractor) Extensions() []string { return e.extensions }

func (e foldExtractor) Extract(filename string, src []byte) (*Outline, error) {
	lines := splitLines(src)
	var nodes []*Node
	var unbalanced bool
	switch e.language {
	case "python":
		nodes = parsePython(lines)
	default:
		nodes, unbalanced = parseBraces(lines, e.language)
	}
	return &Outline{
		File:       filename,
		Language:   e.language,
		Unbalanced: unbalanced,
		Nodes:      nodes,
	}, nil
}

func init() {
	RegisterExtractor(foldExtractor{language: "go", extensions: []string{".go"}})
	RegisterExtractor(foldExtractor{language: "python", extensions: []string{".py"}})
	RegisterExtractor(foldExtractor{language: "powershell", extensions: []string{".ps1", ".psm1"}})
}
