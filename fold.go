package main

import (
	"bytes"
	"fmt"
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

// foldExtractor is the one extractor for every language: it folds
// blocks by indentation, by brackets, or by tags, and returns the
// resulting tree of lines. The parser field says which one —
// "indent", "braces", or "tags" — and comes from the per-language
// routing table below, built from the 33-language test database
// (testdb/CHECKLIST.md). One parser per language, chosen by which
// handled it best.
type foldExtractor struct {
	language   string
	extensions []string
	parser     string // "indent", "braces", or "tags"
	// defaultDepth is the depth shown when the user does not pass
	// --depth. HTML/XML default to 2 (their top level is a single
	// wrapper tag, so 0 shows almost nothing); everything else is 0.
	defaultDepth int
}

func (e foldExtractor) Language() string     { return e.language }
func (e foldExtractor) Extensions() []string { return e.extensions }

func (e foldExtractor) Extract(filename string, src []byte) (*Outline, error) {
	lines := splitLines(src)
	var nodes []*Node
	var unbalanced bool
	switch e.parser {
	case "indent":
		nodes = parsePython(lines)
	case "tags":
		nodes, unbalanced = parseTags(lines)
	case "text":
		// The fallback parser only ever sees text: a binary file
		// (NUL bytes) is rejected here, not outlined as garbage.
		if bytes.Contains(src, []byte{0}) {
			return nil, fmt.Errorf("%s: not a text file (contains NUL bytes)", filename)
		}
		nodes = parseText(lines)
	default:
		nodes, unbalanced = parseBraces(lines, e.language)
	}
	return &Outline{
		File:         filename,
		Language:     e.language,
		Unbalanced:   unbalanced,
		DefaultDepth: e.defaultDepth,
		Nodes:        nodes,
	}, nil
}

func init() {
	// Brace languages: the structure is in the brackets, so the
	// brace parser folds them and does not depend on indentation.
	RegisterExtractor(foldExtractor{language: "go", extensions: []string{".go"}, parser: "braces"})
	RegisterExtractor(foldExtractor{language: "powershell", extensions: []string{".ps1", ".psm1"}, parser: "braces"})
	RegisterExtractor(foldExtractor{language: "javascript", extensions: []string{".js", ".mjs", ".cjs"}, parser: "braces"})
	RegisterExtractor(foldExtractor{language: "typescript", extensions: []string{".ts", ".tsx"}, parser: "braces"})
	RegisterExtractor(foldExtractor{language: "rust", extensions: []string{".rs"}, parser: "braces"})
	RegisterExtractor(foldExtractor{language: "c", extensions: []string{".c", ".h"}, parser: "braces"})
	RegisterExtractor(foldExtractor{language: "cpp", extensions: []string{".cpp", ".cc", ".cxx", ".hpp", ".hh", ".hxx"}, parser: "braces"})
	RegisterExtractor(foldExtractor{language: "csharp", extensions: []string{".cs"}, parser: "braces"})
	RegisterExtractor(foldExtractor{language: "java", extensions: []string{".java"}, parser: "braces"})
	RegisterExtractor(foldExtractor{language: "kotlin", extensions: []string{".kt", ".kts"}, parser: "braces"})
	RegisterExtractor(foldExtractor{language: "swift", extensions: []string{".swift"}, parser: "braces"})
	RegisterExtractor(foldExtractor{language: "scala", extensions: []string{".scala"}, parser: "braces"})
	RegisterExtractor(foldExtractor{language: "dart", extensions: []string{".dart"}, parser: "braces"})
	RegisterExtractor(foldExtractor{language: "groovy", extensions: []string{".groovy"}, parser: "braces"})
	RegisterExtractor(foldExtractor{language: "zig", extensions: []string{".zig"}, parser: "braces"})
	RegisterExtractor(foldExtractor{language: "css", extensions: []string{".css"}, parser: "braces"})
	RegisterExtractor(foldExtractor{language: "json", extensions: []string{".json"}, parser: "braces"})
	RegisterExtractor(foldExtractor{language: "php", extensions: []string{".php"}, parser: "braces"})

	// Indent languages: the structure is in the indentation (and in
	// the generic rules the indent parser carries — # and // and --
	// and % comments, bare end, </...> closers, <!-- --> comments,
	// <?php tags, #include/#import directives, nested /* */).
	RegisterExtractor(foldExtractor{language: "python", extensions: []string{".py"}, parser: "indent"})
	RegisterExtractor(foldExtractor{language: "ruby", extensions: []string{".rb"}, parser: "indent"})
	RegisterExtractor(foldExtractor{language: "perl", extensions: []string{".pl", ".pm"}, parser: "indent"})
	RegisterExtractor(foldExtractor{language: "lua", extensions: []string{".lua"}, parser: "indent"})
	RegisterExtractor(foldExtractor{language: "r", extensions: []string{".r"}, parser: "indent"})
	RegisterExtractor(foldExtractor{language: "julia", extensions: []string{".jl"}, parser: "indent"})
	// .m is shared by Objective-C and MATLAB; both route to the
	// indent parser, so the shared extension is not a conflict.
	RegisterExtractor(foldExtractor{language: "objc", extensions: []string{".m", ".mm"}, parser: "indent"})
	RegisterExtractor(foldExtractor{language: "matlab", extensions: []string{".m"}, parser: "indent"})
	RegisterExtractor(foldExtractor{language: "bash", extensions: []string{".sh", ".bash"}, parser: "indent"})
	RegisterExtractor(foldExtractor{language: "sql", extensions: []string{".sql"}, parser: "indent"})
	RegisterExtractor(foldExtractor{language: "latex", extensions: []string{".tex"}, parser: "indent"})
	RegisterExtractor(foldExtractor{language: "clojure", extensions: []string{".clj", ".cljs", ".edn"}, parser: "indent"})
	RegisterExtractor(foldExtractor{language: "haskell", extensions: []string{".hs"}, parser: "indent"})
	RegisterExtractor(foldExtractor{language: "yaml", extensions: []string{".yml", ".yaml"}, parser: "indent"})

	// Tag languages: the structure is in the tags themselves, so the
	// tag parser reads them and nests correctly regardless of
	// indentation. It replaces the indent-parser rules for HTML/XML
	// (the </...> and <!-- --> rules stay in the indent parser as
	// harmless generic rules, but no routed language uses them now).
	// Their default depth is 2: the top level is a single wrapper
	// tag (<html>, <manifest>, <resources>), so depth 0 shows
	// almost nothing.
	RegisterExtractor(foldExtractor{language: "html", extensions: []string{".html", ".htm"}, parser: "tags", defaultDepth: 2})
	RegisterExtractor(foldExtractor{language: "xml", extensions: []string{".xml"}, parser: "tags", defaultDepth: 2})

	// Plain text: the fallback parser for files with no registered
	// language. It folds by indentation with no comment or string
	// rules, so a markdown heading, a TOML section header, and a
	// config key are all just lines. The extensions below are the
	// common text types, included in directory walks; any other
	// text file still works when passed directly, because main.go
	// falls back to this parser for an unregistered extension
	// (the NUL check in Extract rejects binary files). Use
	// --lang text to force this parser on any file.
	RegisterExtractor(foldExtractor{language: "text", extensions: []string{".md", ".markdown", ".txt", ".toml", ".ini", ".cfg", ".conf", ".log", ".rst", ".adoc", ".text", ".env", ".properties"}, parser: "text"})
}
