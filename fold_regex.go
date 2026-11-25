package main

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

type jsTokenContext byte

const (
	jsExpectOperand jsTokenContext = iota
	jsAfterOperand
	jsAfterMember
	jsExpectControlParen
)

// Keep the lexical context across lines and comments. A control header
// is followed by a statement, but a call or a grouped expression ends
// an operand. Track nested parentheses to tell the closers apart.
type jsSlashContext struct {
	next          jsTokenContext
	controlParens []bool
}

func (s *jsSlashContext) consume(line string, start int) int {
	if end := jsWordEnd(line, start); end > start {
		word := line[start:end]
		if s.next == jsAfterMember {
			s.next = jsAfterOperand
			return end
		}
		switch word {
		case "if", "for", "while", "with", "switch", "catch":
			s.next = jsExpectControlParen
		case "await":
			if s.next != jsExpectControlParen { // for await (...)
				s.next = jsExpectOperand
			}
		case "return", "throw", "yield", "case", "delete", "void", "typeof", "new", "in", "instanceof", "else", "do":
			s.next = jsExpectOperand
		default:
			s.next = jsAfterOperand
		}
		return end
	}
	token, width := utf8.DecodeRuneInString(line[start:])
	if unicode.IsSpace(token) {
		return start + width
	}
	switch token {
	case '(':
		s.controlParens = append(s.controlParens, s.next == jsExpectControlParen)
		s.next = jsExpectOperand
	case ')':
		s.next = jsAfterOperand
		if i := len(s.controlParens) - 1; i >= 0 {
			if s.controlParens[i] {
				s.next = jsExpectOperand
			}
			s.controlParens = s.controlParens[:i]
		}
	case '.':
		if strings.HasPrefix(line[start:], "...") {
			s.next = jsExpectOperand
			return start + 3
		}
		s.next = jsAfterMember
	case '+', '-':
		if start+1 < len(line) && rune(line[start+1]) == token {
			return start + 2 // prefix expects an operand, postfix ends one
		}
		s.next = jsExpectOperand
	case '{', '[', '=', ',', ';', ':', '?', '*', '/', '%', '!', '~', '<', '>', '&', '|', '^':
		s.next = jsExpectOperand
	default:
		// This includes ']' and '}'. Treat '}' conservatively as the
		// end of an operand because it might close an object literal.
		s.next = jsAfterOperand
	}
	return start + width
}

// jsRegexEnd returns the end of a complete same-line regex token.
// Comments are recognized by the caller before testing for a regex opener.
func jsRegexEnd(line string, start int) int {
	inClass := false
	for i := start + 1; i < len(line); i++ {
		switch line[i] {
		case '\\':
			i++
		case '[':
			inClass = true
		case ']':
			inClass = false
		case '/':
			if !inClass {
				return jsWordEnd(line, i+1)
			}
		}
	}
	return start
}

// Words include identifier continuations and digits. For slash
// classification a number has the same effect as an ordinary name.
func jsWordEnd(line string, start int) int {
	for offset, r := range line[start:] {
		if r != '_' && r != '$' && r != '\u200c' && r != '\u200d' &&
			!unicode.IsLetter(r) && !unicode.IsDigit(r) && !unicode.IsMark(r) {
			return start + offset
		}
	}
	return len(line)
}
