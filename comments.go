package main

import "strings"

// Comment support for --ask.
//
// A comment describes the code below it, so --ask should find that
// code from the comment's words. Trailing comments are already
// indexed: a node's Text is the full source line, so `x := 1 //
// initialize` searches on "initialize". The gap is standalone
// comment blocks — full-line comments and block comments — which no
// node contains. This file extracts them and attaches each block's
// text to the next outline node in source order (the code the
// comment describes). The answer to a search is then the code, not
// the comment, and a comment and its code never appear as two
// results for one location.
//
// Extraction is a post-processing step after Extract, in the CLI
// layer where the raw bytes live; the extractors stay pure and
// unchanged. It is table-driven and language-agnostic: one scanner
// for all languages, with the comment syntax in a per-language
// table. Block openers are checked before line markers, because
// several openers start with a line marker ("--[[", "#=", "<#").
//
// A comment is standalone when its marker is the first
// non-whitespace on the line. A marker after code is a trailing
// comment, already part of that line's node, so it is not extracted.
// Strings are not tracked: a marker at line start is a comment, so a
// multi-line string whose content line starts with a marker is a
// rare false positive. Nested block comments are not counted (SQL
// is the only language with them); a block ends at the first closer.

// commentSyntax is one language's comment forms.
type commentSyntax struct {
	line  []string    // line comment markers, e.g. "//", "#"
	block [][2]string // block comment delimiters, e.g. {"/*", "*/"}
}

// commentSyntaxByLanguage maps a language name (as reported by
// --help and Outline.Language) to its comment forms. Languages
// absent here (json, text) have no comments to extract. The table
// is deliberately per-language, not per-parser: the indent parser
// treats any comment-looking marker as a comment for folding, but
// extraction must not call "#" a comment in C (preprocessor) or "%"
// one in Ruby (string literal).
var commentSyntaxByLanguage = map[string]commentSyntax{
	"bash":       {line: []string{"#"}},
	"c":          {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	"clojure":    {line: []string{";"}},
	"cpp":        {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	"csharp":     {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	"css":        {block: [][2]string{{"/*", "*/"}}},
	"dart":       {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	"go":         {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	"groovy":     {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	"haskell":    {line: []string{"--"}, block: [][2]string{{"{-", "-}"}}},
	"html":       {block: [][2]string{{"<!--", "-->"}}},
	"java":       {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	"javascript": {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	"julia":      {line: []string{"#"}, block: [][2]string{{"#=", "=#"}}},
	"kotlin":     {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	"latex":      {line: []string{"%"}},
	"lua":        {line: []string{"--"}, block: [][2]string{{"--[[", "]]"}}},
	"matlab":     {line: []string{"%"}, block: [][2]string{{"%{", "%}"}}},
	"objc":       {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	"perl":       {line: []string{"#"}},
	"php":        {line: []string{"//", "#"}, block: [][2]string{{"/*", "*/"}}},
	"powershell": {line: []string{"#"}, block: [][2]string{{"<#", "#>"}}},
	"python":     {line: []string{"#"}},
	"r":          {line: []string{"#"}},
	"ruby":       {line: []string{"#"}, block: [][2]string{{"=begin", "=end"}}},
	"rust":       {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	"scala":      {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	"sql":        {line: []string{"--"}, block: [][2]string{{"/*", "*/"}}},
	"swift":      {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	"typescript": {line: []string{"//"}, block: [][2]string{{"/*", "*/"}}},
	"xml":        {block: [][2]string{{"<!--", "-->"}}},
	"yaml":       {line: []string{"#"}},
	"zig":        {line: []string{"//"}},
}

// commentBlock is a run of consecutive line comments or one block
// comment, with its 1-based line range and cleaned text (markers
// stripped, lines joined with " ").
type commentBlock struct {
	start int
	end   int
	text  string
}

// lineMarkerAt returns the line comment marker that trimmed starts
// with, or "" when it starts with none.
func lineMarkerAt(trimmed string, markers []string) string {
	for _, m := range markers {
		if strings.HasPrefix(trimmed, m) {
			return m
		}
	}
	return ""
}

// blockOpenerAt returns the block delimiters whose opener trimmed
// starts with, or "" when it starts with none.
func blockOpenerAt(trimmed string, pairs [][2]string) (string, string) {
	for _, p := range pairs {
		if strings.HasPrefix(trimmed, p[0]) {
			return p[0], p[1]
		}
	}
	return "", ""
}

// cleanBlockText strips a block opener and closer from a line and
// trims it. Interior lines carry neither; a leading "*" (the usual
// block-comment continuation) is stripped too, though tokenize
// would split on it anyway.
func cleanBlockText(trimmed, opener, closer string) string {
	if opener != "" {
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, opener))
	}
	if closer != "" {
		if idx := strings.Index(trimmed, closer); idx >= 0 {
			trimmed = strings.TrimSpace(trimmed[:idx])
		}
	}
	trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "*"))
	return trimmed
}

// extractComments scans lines for standalone comments in lang and
// returns them as blocks in source order. Consecutive line comments
// form one block; a block comment is one block. A line that opens a
// block comment and has code after the closer is a code line, not a
// comment, so it is not extracted (its words are already in the
// node's text).
func extractComments(lines []string, lang string) []commentBlock {
	syn, ok := commentSyntaxByLanguage[lang]
	if !ok {
		return nil
	}
	var blocks []commentBlock
	var run *commentBlock // current run of line comments
	var blk *commentBlock // current block comment
	var blkCloser string
	flushRun := func() {
		if run != nil {
			blocks = append(blocks, *run)
			run = nil
		}
	}
	for i, raw := range lines {
		lineNo := i + 1
		trimmed := strings.TrimSpace(raw)
		if blk != nil {
			// Inside a block comment: the line is comment text.
			if t := cleanBlockText(trimmed, "", blkCloser); t != "" {
				blk.text += " " + t
			}
			blk.end = lineNo
			if strings.Contains(trimmed, blkCloser) {
				blocks = append(blocks, *blk)
				blk = nil
			}
			continue
		}
		// Block opener at line start?
		if opener, closer := blockOpenerAt(trimmed, syn.block); opener != "" {
			rest := trimmed[len(opener):]
			if idx := strings.Index(rest, closer); idx >= 0 && strings.TrimSpace(rest[idx+len(closer):]) != "" {
				// Code follows the closer on the same line: a code
				// line, not a comment. Its words are already in
				// the node's text.
				flushRun()
				continue
			}
			flushRun()
			blk = &commentBlock{start: lineNo, end: lineNo, text: cleanBlockText(trimmed, opener, closer)}
			blkCloser = closer
			if strings.Contains(rest, closer) {
				blocks = append(blocks, *blk)
				blk = nil
			}
			continue
		}
		// Line comment at line start?
		if marker := lineMarkerAt(trimmed, syn.line); marker != "" {
			text := strings.TrimSpace(trimmed[len(marker):])
			if run == nil {
				run = &commentBlock{start: lineNo, end: lineNo, text: text}
			} else {
				run.end = lineNo
				if text != "" {
					run.text += " " + text
				}
			}
			continue
		}
		// Not a comment line.
		flushRun()
	}
	flushRun()
	if blk != nil {
		blocks = append(blocks, *blk)
	}
	// Drop blocks whose text came out empty (a lone "//" line).
	out := blocks[:0]
	for _, b := range blocks {
		b.text = strings.TrimSpace(b.text)
		if b.text != "" {
			out = append(out, b)
		}
	}
	return out
}

// attachComments extracts the standalone comments from src and
// attaches each block's text to the next outline node in source
// order (the code the comment describes). A block after the last
// node attaches to the last node. Nodes gain a Comments field; the
// outline formatters ignore it, so the fold output is unchanged.
// Retrieval indexes the attached text, so a word that only appears
// in a comment still finds the code it describes.
func attachComments(o *Outline, src []byte) {
	if o == nil || len(o.Nodes) == 0 {
		return
	}
	blocks := extractComments(strings.Split(string(src), "\n"), o.Language)
	if len(blocks) == 0 {
		return
	}
	// Flatten the outline in source order (pre-order: a parent's
	// line precedes its children's).
	var flat []*Node
	var walk func(nodes []*Node)
	walk = func(nodes []*Node) {
		for _, n := range nodes {
			flat = append(flat, n)
			walk(n.Children)
		}
	}
	walk(o.Nodes)
	for _, b := range blocks {
		target := flat[len(flat)-1]
		for _, n := range flat {
			if n.Line > b.end {
				target = n
				break
			}
		}
		target.Comments = append(target.Comments, b.text)
	}
}
