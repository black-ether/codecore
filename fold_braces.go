package main

import "strings"

// parseBraces folds a brace-language file by brackets: '{', '(' and
// '[' open, their closers close. A line whose opener stays open past
// the end of the line is a block header (an import group, a multi-line
// call, a hashtable, a scriptblock); a line whose brackets all balance
// on the line is a leaf. Strings and comments are skipped by a small
// scanner so braces inside them never count. A closer pops only a
// frame whose opener matches, so a stray bracket in broken code is
// ignored instead of un-nesting everything after it; that, or a block
// left open at end of file, marks the outline Unbalanced (a warning,
// not an error — the outline is still produced).
func parseBraces(lines []string, lang string) ([]*Node, bool) {
	var root []*Node
	stack := []braceFrame{{opener: 0, kids: &root}}
	st := &braceState{}
	unbalanced := false
	// lastNode is the most recent code line's node. A lone opener on
	// a line of its own (Allman style: the '{' under an 'if') has no
	// header of its own, so it attaches its block to that line —
	// one level at a time, instead of the block's contents
	// flattening to the enclosing level.
	var lastNode *Node
	for lineno, line := range lines {
		// A PHP tag (<?php / ?>) is boilerplate, not a statement.
		if stripped := strings.TrimSpace(line); stripped == "<?php" || stripped == "?>" {
			continue
		}
		events, hasCode := scanBraces(line, lang, st)
		if !hasCode && len(events) == 0 {
			continue
		}
		var node *Node
		var rest []byte
		// poppedHeader is the node of the most recent frame a closer
		// popped on this line. A header-continuation line (") {")
		// belongs to that node, not to the previous code line.
		var poppedHeader *Node
		if hasCode {
			// Leading closers (before the first opener) close blocks
			// from earlier lines; the node attaches after those pops.
			// Remember the node a leading closer popped — a header
			// continuation below belongs to that line.
			firstOpen := len(events)
			for k, ev := range events {
				if isOpener(ev) {
					firstOpen = k
					break
				}
			}
			var header *Node
			for _, ev := range events[:firstOpen] {
				if isCloser(ev) {
					if owner := closeBraceFrame(&stack, ev, lineno+1, &unbalanced); owner != nil {
						header = owner
					}
				}
			}
			rest = events[firstOpen:]
			if header != nil && isHeaderContinuation(strings.TrimSpace(line)) {
				// The line is only the tail of the header above it
				// (") => {" or ") {"): no node of its own. Its
				// opener pushes the header's frame, so the body
				// nests under the header line instead of under
				// the closer line.
				node = header
			} else {
				node = &Node{Line: lineno + 1, Text: cleanText(line)}
				*stack[len(stack)-1].kids = append(*stack[len(stack)-1].kids, node)
				lastNode = node
			}
		} else {
			rest = events
		}
		for _, ev := range rest {
			if isOpener(ev) {
				// An opener pushes the line's node when that node is
				// not already on the stack (a '{' after a balanced
				// '[]' re-pushes it). On a line with no code of its
				// own, the opener belongs to the previous code line.
				header := node
				if header == nil {
					header = lastNode
				}
				// A header-continuation line (") {") belongs to
				// the header whose frame a closer just popped,
				// not to the previous code line — otherwise the
				// body would nest under the parameter line.
				if node == nil && poppedHeader != nil && isHeaderContinuation(strings.TrimSpace(line)) {
					header = poppedHeader
				}
				if header != nil && !nodeOnStack(stack, header) {
					stack = append(stack, braceFrame{opener: ev, node: header, kids: &header.Children})
				} else {
					stack = append(stack, braceFrame{opener: ev, kids: stack[len(stack)-1].kids})
				}
			} else if isCloser(ev) {
				if owner := closeBraceFrame(&stack, ev, lineno+1, &unbalanced); owner != nil {
					poppedHeader = owner
				}
			}
		}
	}
	if len(stack) > 1 {
		unbalanced = true
		for _, frame := range stack[1:] {
			if frame.node != nil {
				frame.node.SourceEnd = -1
			}
		}
	}
	return root, unbalanced
}

// isHeaderContinuation reports whether a code line is only the tail
// of a header that started on an earlier line: leading closers,
// then optionally "=>", then one opener, then nothing but a
// comment. Such a line belongs to the header above it instead of
// being a node of its own. A line with real content before the
// closer (like "y > 0) {") or after it (like "): number {") is not
// one — those keep their node.
func isHeaderContinuation(stripped string) bool {
	s := strings.TrimLeft(stripped, ")]} \t")
	if strings.HasPrefix(s, "=>") {
		s = strings.TrimSpace(s[2:])
	}
	if len(s) == 0 || (s[0] != '{' && s[0] != '(' && s[0] != '[') {
		return false
	}
	rest := strings.TrimSpace(s[1:])
	return rest == "" || strings.HasPrefix(rest, "//") || strings.HasPrefix(rest, "/*")
}

// braceFrame is one open bracket on the stack: its opener, the line's
// node when this frame is that node's block (nil for a transparent
// grouping frame), and the list lines inside it attach to.
type braceFrame struct {
	opener byte
	node   *Node
	kids   *[]*Node
}

func closeBraceFrame(stack *[]braceFrame, closer byte, line int, unbalanced *bool) *Node {
	i := len(*stack) - 1
	frame := (*stack)[i]
	if i == 0 || frame.opener != matchCloser(closer) {
		*unbalanced = true
		return nil
	}
	*stack = (*stack)[:i]
	if frame.node != nil {
		frame.node.SourceEnd = line
	}
	return frame.node
}

func nodeOnStack(stack []braceFrame, node *Node) bool {
	for _, f := range stack {
		if f.node == node {
			return true
		}
	}
	return false
}

func isOpener(c byte) bool { return c == '(' || c == '[' || c == '{' }
func isCloser(c byte) bool { return c == ')' || c == ']' || c == '}' }

func matchCloser(c byte) byte {
	switch c {
	case ')':
		return '('
	case ']':
		return '['
	case '}':
		return '{'
	}
	return 0
}

// braceState carries block-comment / raw-string / here-string state
// across lines. hereString is 0, or the quote that opened the
// here-string ('"' or '\”).
type braceState struct {
	blockComment bool
	raw          bool
	hereString   byte
}

// scanBraces scans one line for brackets outside strings and comments.
//
// It returns the bracket chars in order and whether the line holds code
// besides brackets and punctuation. A line that is only a string
// literal counts as code (the string is a token); a line that is only
// a closer, a comma, or a comment does not.
func scanBraces(line, lang string, st *braceState) ([]byte, bool) {
	var events []byte
	var codeChars []byte
	n := len(line)
	if st.hereString != 0 {
		// A here-string ends when a line starts with its terminator.
		term := string(st.hereString) + "@"
		if strings.HasPrefix(strings.TrimSpace(line), term) {
			st.hereString = 0
			if j := strings.Index(line, term); j >= 0 {
				return scanBraces(line[j+len(term):], lang, st)
			}
		}
		return nil, false
	}
	i := 0
	for i < n {
		c := line[i]
		if st.blockComment {
			closer := "*/"
			if lang == "powershell" {
				closer = "#>"
			}
			if strings.HasPrefix(line[i:], closer) {
				st.blockComment = false
				i += len(closer)
			} else {
				i++
			}
			continue
		}
		if st.raw { // Go backtick raw string (multi-line)
			if c == '`' {
				st.raw = false
			}
			i++
			continue
		}
		if i+1 < n {
			two := line[i : i+2]
			// C-style block comments (/* */) are used by every brace
			// language except PowerShell (which uses <# #>).
			if lang != "powershell" && two == "/*" {
				st.blockComment = true
				i += 2
				continue
			}
			if lang == "powershell" && two == "<#" {
				st.blockComment = true
				i += 2
				continue
			}
			// C-style line comments (//) are used by every brace
			// language except PowerShell (which uses #).
			if lang != "powershell" && two == "//" {
				break
			}
			if lang == "powershell" && (two == "@\"" || two == "@'") {
				st.hereString = two[1]
				return events, hasCode(codeChars)
			}
		}
		if lang == "powershell" && c == '#' {
			break
		}
		if c == '"' {
			i++
			for i < n && line[i] != '"' {
				// Backslash escapes strings in every brace language
				// except PowerShell (which uses ` instead).
				if (lang != "powershell" && line[i] == '\\') || (lang == "powershell" && line[i] == '`') {
					i += 2
				} else {
					i++
				}
			}
			if i < n {
				i++
			}
			codeChars = append(codeChars, 'x') // the string is a code token
			continue
		}
		if c == '\'' {
			i++
			for i < n {
				if line[i] == '\'' {
					// In PowerShell, '' is an escaped quote inside a
					// literal string; a lone ' closes it.
					if lang == "powershell" && i+1 < n && line[i+1] == '\'' {
						i += 2
						continue
					}
					i++
					break
				}
				if lang != "powershell" && line[i] == '\\' {
					i += 2
				} else {
					i++
				}
			}
			codeChars = append(codeChars, 'x') // the rune/literal is a code token
			continue
		}
		if lang == "go" && c == '`' {
			st.raw = true
			codeChars = append(codeChars, 'x')
			i++
			continue
		}
		if isOpener(c) || isCloser(c) {
			events = append(events, c)
		} else {
			codeChars = append(codeChars, c)
		}
		i++
	}
	return events, hasCode(codeChars)
}

// hasCode reports whether the collected code chars hold anything
// besides whitespace and bare punctuation (a line that is only a
// closer, a comma, or a semicolon is not a code line).
func hasCode(codeChars []byte) bool {
	for _, c := range codeChars {
		switch c {
		case ' ', '\t', ',', ';':
		default:
			return true
		}
	}
	return false
}
