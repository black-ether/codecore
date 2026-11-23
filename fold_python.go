package main

import "strings"

// parsePython folds a Python file by indentation: a line opens a
// block when the next code line is more indented — no ':' is
// required. Blank lines and comments are skipped; lines inside
// triple-quoted strings are skipped too, so a "def" inside a
// docstring never opens a block. A line that is only a
// triple-quoted string (a docstring) is not shown at all: it is
// pure symbols, or text with no structure, and it tells you
// nothing about the outline. Decorator lines merge into the
// following definition's header, so a decorated def shows as one
// node ("@property def label ...") instead of a stray leaf above it.
func parsePython(lines []string) []*Node {
	// First pass: code lines only, with their indent and text.
	type codeLine struct {
		lineno int
		indent int
		text   string
	}
	var code []codeLine
	inTriple := ""
	inBlockComment := false
	for i, line := range lines {
		if inTriple != "" {
			if strings.Contains(line, inTriple) {
				inTriple = ""
			}
			continue
		}
		if inBlockComment {
			if strings.Contains(line, "*/") {
				inBlockComment = false
			}
			continue
		}
		stripped := strings.TrimSpace(line)
		// A # line is a comment — except #[...], which is an
		// attribute (Rust), not a comment. Only a # at the start
		// of the line counts either way.
		if stripped == "" || (strings.HasPrefix(stripped, "#") && !strings.HasPrefix(stripped, "#[")) {
			continue
		}
		// A // line comment is not a statement.
		if strings.HasPrefix(stripped, "//") {
			continue
		}
		// A line that is only closers (and commas/semicolons) is part
		// of a multi-line construct above it, not a statement of its
		// own: it is not shown, and the next line's indent pops the
		// stack back on its own.
		if isPunctuationOnly(stripped) {
			continue
		}
		// A /* */ block comment is not a statement either. A line
		// that only opens one starts a block comment; a line that
		// opens and closes one is skipped unless code follows the
		// closer. Only a comment at the start of the line is
		// treated this way — a /* inside a string or after code
		// is left alone.
		if strings.HasPrefix(stripped, "/*") {
			if idx := strings.LastIndex(stripped, "*/"); idx >= 0 {
				if strings.TrimSpace(stripped[idx+2:]) == "" {
					continue
				}
			} else {
				inBlockComment = true
				continue
			}
		}
		opened := ""
		for _, q := range []string{`"""`, `'''`} {
			if strings.Count(line, q)%2 == 1 {
				opened = q
				break
			}
		}
		// A standalone docstring is skipped, but the string's content
		// still has to be tracked so it stays hidden.
		prevText := ""
		if len(code) > 0 {
			prevText = code[len(code)-1].text
		}
		if isDocstringLine(stripped, prevText) {
			if opened != "" {
				inTriple = opened
			}
			continue
		}
		expanded := expandTabs(line)
		indent := len(expanded) - len(strings.TrimLeft(expanded, " "))
		code = append(code, codeLine{lineno: i + 1, indent: indent, text: stripped})
		if opened != "" {
			inTriple = opened
		}
	}
	// Merge decorator lines into the following definition's header.
	var merged []codeLine
	var pending []string
	for _, e := range code {
		if strings.HasPrefix(e.text, "@") {
			pending = append(pending, e.text)
			continue
		}
		if len(pending) > 0 {
			e.text = strings.Join(pending, " ") + " " + e.text
			pending = nil
		}
		merged = append(merged, e)
	}
	code = merged

	// Second pass: indent stack. A line opens a block when the next
	// code line is more indented — no ':' is required. That is the
	// whole rule, and it is why the parser is not Python-specific:
	// a multi-line call, list or dict nests under its first line
	// (its contents are inside it, not top-level statements), and
	// the same rule folds Ruby, Lua and anything else that is
	// consistently indented.
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

// isDocstringLine reports whether a line is a standalone
// triple-quoted string (a docstring), not a string argument inside
// a multi-line call, list or dict. A docstring starts with a
// triple quote, after an optional r/b/f/u prefix; a string
// argument follows a line ending in an opener or a comma, so
// those lines stay. prevText is the previous code line's text,
// or "" when there is none.
func isDocstringLine(stripped, prevText string) bool {
	s := stripped
	for len(s) > 0 && strings.ContainsRune("rRbBfFuU", rune(s[0])) {
		s = s[1:]
	}
	if !strings.HasPrefix(s, `"""`) && !strings.HasPrefix(s, `'''`) {
		return false
	}
	prev := strings.TrimSpace(prevText)
	if strings.HasSuffix(prev, "(") || strings.HasSuffix(prev, "[") ||
		strings.HasSuffix(prev, "{") || strings.HasSuffix(prev, ",") {
		return false
	}
	return true
}

// isPunctuationOnly reports whether a line holds only brackets,
// commas and semicolons — a bare closer of a multi-line construct,
// not a statement.
func isPunctuationOnly(s string) bool {
	for _, r := range s {
		switch r {
		case '(', ')', '[', ']', '{', '}', ',', ';':
		default:
			return false
		}
	}
	return len(s) > 0
}

// expandTabs expands tabs to the next multiple of 8 columns, the way a
// terminal (and Python's own tab handling) does.
func expandTabs(s string) string {
	var b strings.Builder
	col := 0
	for _, r := range s {
		if r == '\t' {
			n := 8 - col%8
			b.WriteString(strings.Repeat(" ", n))
			col += n
		} else {
			b.WriteRune(r)
			col++
		}
	}
	return b.String()
}
