package main

import (
	"fmt"
	"iter"
	"maps"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

type tokenKind int

const (
	tokIdent tokenKind = iota
	tokKeyword
	tokNativeType
	tokString
	tokNumber
	tokOperator
	tokBracket
)

type token struct {
	kind tokenKind
	text string
	line int
}

const patternOperators = "=<>!+-*/%&|^~?:;,#"

// preprocessorKeywords are the words that make a # at the start of a
// line a C-style preprocessor directive instead of a comment.
var preprocessorKeywords = map[string]bool{
	"include": true, "import": true, "define": true, "undef": true,
	"ifdef": true, "ifndef": true, "if": true, "else": true,
	"elif": true, "endif": true, "pragma": true, "error": true,
	"warning": true, "line": true,
}

// isPatternPreprocessorLine reports whether the # at pos in src starts a
// C-style preprocessor directive: the # is the first non-whitespace
// character on its line, and the word after it is a preprocessor
// keyword. A # anywhere else (or followed by other text) is a
// comment, as in Python, Ruby, Perl, and shell.
func isPatternPreprocessorLine(src string, pos int) bool {
	for i := pos - 1; i >= 0; i-- {
		if src[i] == '\n' {
			break
		}
		if src[i] != ' ' && src[i] != '\t' && src[i] != '\r' {
			return false
		}
	}
	j := pos + 1
	for j < len(src) && (src[j] == ' ' || src[j] == '\t') {
		j++
	}
	k := j
	for k < len(src) && unicode.IsLetter(rune(src[k])) {
		k++
	}
	return preprocessorKeywords[src[j:k]]
}

// codeComment reports whether the text at pos in src starts a comment:
// "/*" for a block comment, "line" for //, --, or # to end of line.
// A # that starts a preprocessor directive is code, not a comment.
func codeComment(src string, pos int) string {
	s := src[pos:]
	switch {
	case strings.HasPrefix(s, "/*"):
		return "/*"
	case strings.HasPrefix(s, "//"), strings.HasPrefix(s, "--"):
		return "line"
	case len(s) > 0 && s[0] == '#' && !isPatternPreprocessorLine(src, pos):
		return "line"
	}
	return ""
}

func quoteDelimiter(s string) string {
	if strings.HasPrefix(s, "\"\"\"") || strings.HasPrefix(s, "'''") {
		return s[:3]
	}
	return s[:1]
}

func quotedLength(s string) int {
	delim := quoteDelimiter(s)
	for i := len(delim); i < len(s); i++ {
		if strings.HasPrefix(s[i:], delim) {
			return i + len(delim)
		}
		if delim != "`" && s[i] == '\\' && i+1 < len(s) {
			i++
		}
	}
	return len(s)
}

func tokenizeCode(src []byte, lang string) []token {
	words := keywordTable[strings.ToLower(lang)]
	var tokens []token
	line := 1
	srcStr := string(src)
	for s := srcStr; len(s) > 0; {
		r, size := utf8.DecodeRuneInString(s)
		end := size
		var kind tokenKind
		emit := true
		pos := len(srcStr) - len(s)
		// Phase 1 treats % as an operator and docstrings as strings.
		switch comment := codeComment(srcStr, pos); {
		case comment == "/*":
			end = len(s)
			if close := strings.Index(s[2:], "*/"); close >= 0 {
				end = close + 4
			}
			emit = false
		case comment == "line":
			end = len(s)
			if newline := strings.IndexByte(s, '\n'); newline >= 0 {
				end = newline
			}
			emit = false
		case unicode.IsSpace(r):
			emit = false
		case r == '"' || r == '\'' || r == '`':
			kind, end = tokString, quotedLength(s)
		case unicode.IsLetter(r) || r == '_' || unicode.IsDigit(r):
			kind = tokIdent
			if unicode.IsDigit(r) {
				kind = tokNumber
			}
			for end < len(s) {
				next, width := utf8.DecodeRuneInString(s[end:])
				if !unicode.IsLetter(next) && !unicode.IsDigit(next) && next != '_' &&
					!(kind == tokNumber && next == '.') {
					break
				}
				end += width
			}
			if kind == tokIdent {
				switch word := s[:end]; {
				case words.keywords[word]:
					kind = tokKeyword
				case words.types[word]:
					kind = tokNativeType
				}
			}
		case strings.ContainsRune("()[]{}", r):
			kind = tokBracket
		case r == '.':
			kind = tokOperator
		case strings.ContainsRune(patternOperators, r):
			kind = tokOperator
			for end < len(s) && strings.ContainsRune(patternOperators, rune(s[end])) && codeComment(srcStr, pos+end) == "" {
				end++
			}
		default:
			emit = false
		}
		if emit {
			tokens = append(tokens, token{kind: kind, text: s[:end], line: line})
		}
		line += strings.Count(s[:end], "\n")
		s = s[end:]
	}
	return tokens
}

type atomKind int

const (
	atomAny atomKind = iota
	atomKeyword
	atomNativeType
	atomOperator
	atomNumber
	atomOct
	atomHex
	atomString
	atomLiteral
	atomPlaceholder
	atomEllipsis
	atomNot
	atomOr
	atomExact
	atomOrMarker // unresolved OR during parsing
)

type atom struct {
	kind  atomKind
	text  string
	tok   token
	inner *atom
	left  *atom
	right *atom
}

var patternWildcards = map[string]atomKind{
	"ANY": atomAny, "KEY": atomKeyword, "TYP": atomNativeType,
	"OP": atomOperator, "NUM": atomNumber, "OCT": atomOct,
	"HEX": atomHex, "STR": atomString, "LIT": atomLiteral,
}

func placeholderName(s string) (string, bool) {
	name, ok := strings.CutPrefix(s, "_")
	return name, ok && strings.IndexFunc(name, func(r rune) bool { return !unicode.IsDigit(r) }) == -1
}

func baseAtom(t token) atom {
	if t.kind == tokIdent {
		if kind, ok := patternWildcards[t.text]; ok {
			return atom{kind: kind}
		}
		if name, ok := placeholderName(t.text); ok {
			return atom{kind: atomPlaceholder, text: name}
		}
		if rest, ok := strings.CutPrefix(t.text, "_"); ok {
			_, wildcard := patternWildcards[rest]
			_, placeholder := placeholderName(rest)
			if wildcard || placeholder || rest == "NOT" || rest == "OR" {
				t.text = rest
			}
		}
	}
	return atom{kind: atomExact, text: t.text, tok: t}
}

func parsePattern(pat string) ([]atom, error) {
	tokens := tokenizeCode([]byte(pat), "")
	if len(tokens) == 0 {
		return nil, fmt.Errorf("pattern must contain at least one token")
	}

	// Group dots from the start of each run before parsing combinators.
	grouped := tokens[:0]
	for i := 0; i < len(tokens); i++ {
		t := tokens[i]
		if t.kind == tokOperator && t.text == "." && i+2 < len(tokens) &&
			tokens[i+1].kind == tokOperator && tokens[i+1].text == "." &&
			tokens[i+2].kind == tokOperator && tokens[i+2].text == "." {
			t.text = "..."
			i += 2
		}
		grouped = append(grouped, t)
	}
	tokens = grouped

	// Work from right to left so NOT binds before right-associative OR.
	parts := make([]atom, 0, len(tokens))
	for i := len(tokens) - 1; i >= 0; i-- {
		t := tokens[i]
		switch {
		case t.kind == tokOperator && t.text == "...":
			parts = append(parts, atom{kind: atomEllipsis})
		case t.kind == tokIdent && t.text == "OR":
			parts = append(parts, atom{kind: atomOrMarker, text: t.text, tok: t})
		case t.kind == tokIdent && t.text == "NOT" && len(parts) > 0 &&
			parts[len(parts)-1].kind != atomEllipsis && parts[len(parts)-1].kind != atomOrMarker:
			inner := parts[len(parts)-1]
			parts[len(parts)-1] = atom{kind: atomNot, inner: &inner}
		default:
			parts = append(parts, baseAtom(t))
		}
	}

	atoms := make([]atom, 0, len(parts))
	for i := 0; i < len(parts); i++ {
		a := parts[i]
		switch {
		case a.kind == atomOrMarker:
			a.kind = atomExact
		case i > 0 && parts[i-1].kind == atomOrMarker && len(atoms) > 1 &&
			a.kind != atomEllipsis && atoms[len(atoms)-2].kind != atomEllipsis:
			left, right := a, atoms[len(atoms)-2]
			atoms = atoms[:len(atoms)-2]
			a = atom{kind: atomOr, left: &left, right: &right}
		}
		atoms = append(atoms, a)
	}
	for len(atoms) > 1 && atoms[len(atoms)-1].kind == atomEllipsis && atoms[len(atoms)-2].kind == atomEllipsis {
		atoms = atoms[:len(atoms)-1]
	}
	slices.Reverse(atoms)
	return atoms, nil
}

func unquotedPatternString(s string) string {
	delim := quoteDelimiter(s)
	if len(s) >= 2*len(delim) && strings.HasSuffix(s, delim) {
		s = s[len(delim) : len(s)-len(delim)]
	}
	return strings.TrimSpace(s)
}

func isWordKind(k tokenKind) bool {
	return k == tokIdent || k == tokKeyword || k == tokNativeType
}

// matchSingleAtom shares bindings until a new name needs to be bound.
// This keeps rejected alternatives and negated matches isolated.
func matchSingleAtom(a atom, t token, bindings map[string]string) (map[string]string, bool) {
	switch a.kind {
	case atomPlaceholder:
		// Preserve placeholder matching for words classified in Phase 2.
		if !isWordKind(t.kind) || a.text == "" {
			return bindings, isWordKind(t.kind)
		}
		if bound, ok := bindings[a.text]; ok {
			return bindings, bound == t.text
		}
		if bindings == nil {
			bindings = make(map[string]string)
		} else {
			bindings = maps.Clone(bindings)
		}
		bindings[a.text] = t.text
		return bindings, true
	case atomNot:
		_, ok := matchSingleAtom(*a.inner, t, bindings)
		return bindings, !ok
	case atomOr:
		if next, ok := matchSingleAtom(*a.left, t, bindings); ok {
			return next, true
		}
		return matchSingleAtom(*a.right, t, bindings)
	case atomAny:
		return bindings, true
	case atomKeyword:
		return bindings, t.kind == tokKeyword
	case atomNativeType:
		return bindings, t.kind == tokNativeType
	case atomOperator:
		return bindings, t.kind == tokOperator
	case atomNumber:
		return bindings, t.kind == tokNumber
	case atomOct:
		if t.kind == tokNumber && strings.HasPrefix(t.text, "0") {
			digit, _ := utf8.DecodeRuneInString(t.text[1:])
			return bindings, unicode.IsDigit(digit)
		}
	case atomHex:
		return bindings, t.kind == tokNumber &&
			(strings.HasPrefix(t.text, "0x") || strings.HasPrefix(t.text, "0X"))
	case atomString:
		return bindings, t.kind == tokString
	case atomLiteral:
		return bindings, t.kind == tokString || t.kind == tokNumber
	case atomExact:
		if a.tok.kind == tokString && t.kind == tokString {
			return bindings, unquotedPatternString(a.tok.text) == unquotedPatternString(t.text)
		}
		// Patterns are parsed once without a language.
		sameKind := a.tok.kind == t.kind || (isWordKind(a.tok.kind) && isWordKind(t.kind))
		return bindings, sameKind && a.tok.text == t.text
	}
	return bindings, false
}

// matchAtoms matches a sequence at i and returns the exclusive end.
func matchAtoms(atoms []atom, tokens []token, i int, bindings map[string]string) (int, bool) {
	if len(atoms) == 0 || (len(atoms) == 1 && atoms[0].kind == atomEllipsis) {
		return i, true
	}
	if i == len(tokens) {
		return i, false
	}
	if atoms[0].kind == atomEllipsis {
		return matchEllipsis(atoms[1:], tokens, i, bindings)
	}
	next, ok := matchSingleAtom(atoms[0], tokens[i], bindings)
	if !ok {
		return i, false
	}
	return matchAtoms(atoms[1:], tokens, i+1, next)
}

func matchEllipsis(rest []atom, tokens []token, start int, bindings map[string]string) (int, bool) {
	const maxEllipsisTokens = 64
	depth := 0
	for i := start; ; i++ {
		if depth == 0 {
			if end, ok := matchAtoms(rest, tokens, i, bindings); ok {
				return end, true
			}
		}
		if i == len(tokens) || i-start == maxEllipsisTokens {
			return start, false
		}
		t := tokens[i]
		if t.kind == tokBracket {
			switch t.text {
			case "(", "[", "{":
				depth++
			case ")", "]", "}":
				if depth == 0 {
					return start, false
				}
				depth--
			}
		} else if depth == 0 && t.kind == tokOperator && t.text == ";" {
			return start, false
		}
	}
}

type patternResult struct {
	File string `json:"file"`
	Line int    `json:"line"`
	End  int    `json:"end"`
	Text string `json:"text"`
}

func findPatternMatches(file string, src []byte, atoms []atom, tokens []token) []patternResult {
	return slices.Collect(patternMatches(file, src, atoms, tokens))
}

// patternMatches lets the CLI stop as soon as it detects the first
// match beyond the cap, even when a single file has many matches.
func patternMatches(file string, src []byte, atoms []atom, tokens []token) iter.Seq[patternResult] {
	return func(yield func(patternResult) bool) {
		lines := splitLines(src)
		for i := range tokens {
			end, ok := matchAtoms(atoms, tokens, i, nil)
			if !ok || end <= i {
				continue
			}
			first, last := tokens[i].line, tokens[end-1].line
			if !yield(patternResult{File: file, Line: first, End: last, Text: strings.Join(lines[first-1:last], "\n")}) {
				return
			}
		}
	}
}
