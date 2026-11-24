package main

import "strings"

// parseText folds any plain-text file by indentation: every non-blank
// line is a node, nested under the nearest preceding less-indented
// line. There are no comment, string, or decorator rules — a
// markdown heading, a TOML section header, and a config key are all
// just lines, so they all show. This is the fallback parser for
// files with no registered language: it makes --ask (and outline)
// work on any text file instead of erroring with "no extractor".
// Binary files are rejected by the caller (Extract checks for NUL
// bytes), so this function only ever sees text.
func parseText(lines []string) []*Node {
	type codeLine struct {
		lineno int
		indent int
		text   string
	}
	var code []codeLine
	for i, line := range lines {
		stripped := strings.TrimSpace(line)
		if stripped == "" {
			continue
		}
		expanded := expandTabs(line)
		indent := len(expanded) - len(strings.TrimLeft(expanded, " "))
		code = append(code, codeLine{lineno: i + 1, indent: indent, text: stripped})
	}
	// Indent stack: a line opens a block when the next line is more
	// indented. Same rule as the indent parser's second pass, without
	// any of its language rules.
	var root []*Node
	type frame struct {
		indent int
		kids   *[]*Node
	}
	stack := []frame{{indent: -1, kids: &root}}
	for idx, e := range code {
		for stack[len(stack)-1].indent >= e.indent {
			stack = stack[:len(stack)-1]
		}
		node := &Node{Line: e.lineno, Text: cleanText(e.text)}
		*stack[len(stack)-1].kids = append(*stack[len(stack)-1].kids, node)
		var next *codeLine
		if idx+1 < len(code) {
			next = &code[idx+1]
		}
		if next != nil && next.indent > e.indent {
			stack = append(stack, frame{indent: e.indent, kids: &node.Children})
		}
	}
	return root
}
