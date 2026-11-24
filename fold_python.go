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
	inDocstring := false
	var curDoc *commentBlock
	var docstrings []commentBlock
	blockDepth := 0
	inHTMLComment := false
	for i, line := range lines {
		if inTriple != "" {
			if inDocstring && curDoc != nil {
				if t := docstringText(line, inTriple); t != "" {
					curDoc.text += " " + t
				}
				curDoc.end = i + 1
			}
			if strings.Contains(line, inTriple) {
				inTriple = ""
				if inDocstring {
					if curDoc != nil {
						docstrings = append(docstrings, *curDoc)
						curDoc = nil
					}
					inDocstring = false
				}
			}
			continue
		}
		if blockDepth > 0 {
			blockDepth += strings.Count(line, "/*") - strings.Count(line, "*/")
			if blockDepth < 0 {
				blockDepth = 0
			}
			continue
		}
		if inHTMLComment {
			if strings.Contains(line, "-->") {
				inHTMLComment = false
			}
			continue
		}
		stripped := strings.TrimSpace(line)
		// A PHP tag (<?php / ?>) is boilerplate, not a statement.
		if stripped == "<?php" || stripped == "?>" {
			continue
		}
		// A # line is a comment — except #[...], which is an
		// attribute (Rust), and a preprocessor directive like
		// #include / #import (C, Objective-C, C++), which is a
		// statement, not a comment. Only a # at the start of
		// the line counts either way.
		if stripped == "" || (strings.HasPrefix(stripped, "#") && !strings.HasPrefix(stripped, "#[") && !isPreprocessorLine(stripped)) {
			continue
		}
		// A // line comment is not a statement.
		if strings.HasPrefix(stripped, "//") {
			continue
		}
		// Lua (and SQL): -- starts a line comment.
		if strings.HasPrefix(stripped, "--") {
			continue
		}
		// MATLAB: % starts a line comment, and %% starts a
		// section header. Ruby uses % for string literals
		// (%w[...], %q{...}), so a % followed by a literal
		// letter and a delimiter is code, not a comment.
		if strings.HasPrefix(stripped, "%") && !isRubyPercentLiteral(stripped) {
			continue
		}
		// Ruby, Lua: "end" closes a block, like "}" — not a statement.
		if stripped == "end" {
			continue
		}
		// HTML, XML: a closing tag (</...>) closes a block,
		// like "}" or "end" — not a statement. It is skipped
		// only when nothing but whitespace follows the ">".
		if strings.HasPrefix(stripped, "</") {
			if idx := strings.Index(stripped, ">"); idx >= 0 {
				if strings.TrimSpace(stripped[idx+1:]) == "" {
					continue
				}
			}
		}
		// A line that is only closers (and commas/semicolons) is part
		// of a multi-line construct above it, not a statement of its
		// own: it is not shown, and the next line's indent pops the
		// stack back on its own.
		if isPunctuationOnly(stripped) {
			continue
		}
		// An HTML/XML <!-- --> comment is not a statement.
		// HTML comments do not nest, so a flag (not a depth
		// counter) tracks a multi-line one. A line that opens
		// and closes one is skipped unless code follows the
		// closer. Only a comment at the start of the line is
		// treated this way.
		if strings.HasPrefix(stripped, "<!--") {
			if idx := strings.LastIndex(stripped, "-->"); idx >= 0 {
				if strings.TrimSpace(stripped[idx+3:]) == "" {
					continue
				}
			} else {
				inHTMLComment = true
				continue
			}
		}
		// A /* */ block comment is not a statement either. Block
		// comments nest (SQL), so the depth is counted, not just
		// a flag. A line that only opens one starts a block
		// comment; a line that opens and closes one is skipped
		// unless code follows the closer. Only a comment at the
		// start of the line is treated this way — a /* inside a
		// string or after code is left alone.
		if strings.HasPrefix(stripped, "/*") {
			opens := strings.Count(stripped, "/*")
			closes := strings.Count(stripped, "*/")
			if opens > closes {
				blockDepth = opens - closes
				continue
			}
			if idx := strings.LastIndex(stripped, "*/"); idx >= 0 {
				if strings.TrimSpace(stripped[idx+2:]) == "" {
					continue
				}
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
				// Multi-line docstring opens here: record it and
				// track its content lines (the inTriple block
				// above) until the closing quotes.
				inTriple = opened
				inDocstring = true
				curDoc = &commentBlock{start: i + 1, end: i + 1, text: docstringText(stripped, opened)}
			} else {
				// Single-line docstring: opens and closes here.
				quote := `"""`
				if !strings.Contains(stripped, quote) {
					quote = `'''`
				}
				if t := docstringText(stripped, quote); t != "" {
					docstrings = append(docstrings, commentBlock{start: i + 1, end: i + 1, text: t})
				}
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
	// Attach docstrings to the node they document: the most recent
	// code line before the docstring starts (the def/class above
	// it), or the first node for a module docstring. Docstrings come
	// after the node they describe, unlike comments, so they attach
	// to the previous node, not the next. The text lands in
	// node.Comments, the same field comments use, so retrieval
	// indexes it with no further change.
	if len(docstrings) > 0 && len(root) > 0 {
		var flat []*Node
		var walk func(nodes []*Node)
		walk = func(nodes []*Node) {
			for _, n := range nodes {
				flat = append(flat, n)
				walk(n.Children)
			}
		}
		walk(root)
		for _, d := range docstrings {
			text := strings.TrimSpace(d.text)
			if text == "" {
				continue
			}
			target := flat[0]
			for _, n := range flat {
				if n.Line < d.start {
					target = n
				} else {
					break
				}
			}
			target.Comments = append(target.Comments, text)
		}
	}
	return root
}

// docstringText returns the searchable text of a docstring line: the
// line with an r/b/f/u prefix and the triple quotes removed,
// trimmed. Interior lines (no quotes) just get trimmed. The prefix
// is only stripped when it precedes the quote, so an interior line
// starting with "r" (like "return ...") is not mangled.
func docstringText(line, quote string) string {
	s := strings.TrimSpace(line)
	if !strings.Contains(s, quote) {
		return s
	}
	t := s
	for len(t) > 0 && strings.ContainsRune("rRbBfFuU", rune(t[0])) {
		t = t[1:]
	}
	if strings.HasPrefix(t, quote) {
		s = t
	}
	s = strings.ReplaceAll(s, quote, "")
	return strings.TrimSpace(s)
}

// isPreprocessorLine reports whether a line is a C-style
// preprocessor directive (#include, #import, #define, ...).
// Those are statements, not comments. (C, Objective-C, C++.)
func isPreprocessorLine(stripped string) bool {
	if !strings.HasPrefix(stripped, "#") {
		return false
	}
	rest := strings.TrimSpace(stripped[1:])
	for _, kw := range []string{"include", "import", "define", "undef", "ifdef", "ifndef", "if", "else", "elif", "endif", "pragma", "error", "warning", "line"} {
		if rest == kw || strings.HasPrefix(rest, kw+" ") || strings.HasPrefix(rest, kw+"\t") {
			return true
		}
	}
	return false
}

// isRubyPercentLiteral reports whether a line starts with a Ruby
// % literal (like %w[...], %q{...}, %r/.../): a % followed by one
// of the literal letters w W q Q r s x i I and then a delimiter
// (a character that is not a letter, digit or underscore). A %
// followed by anything else is a MATLAB comment, not a Ruby
// literal — including %write, where the letter after w is a
// letter, not a delimiter.
func isRubyPercentLiteral(stripped string) bool {
	if len(stripped) < 3 || stripped[0] != '%' {
		return false
	}
	switch stripped[1] {
	case 'w', 'W', 'q', 'Q', 'r', 's', 'x', 'i', 'I':
	default:
		return false
	}
	c := stripped[2]
	return !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_')
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
