package main

import (
	"strings"
	"unicode"
)

func identifierLexemes(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_'
	})
}

func wholeIdentifierTerms(text string) []string {
	return identifierLexemes(strings.ToLower(text))
}

// identifierTerms emits one original term per lexeme and one occurrence of
// each distinct component within that lexeme. The caller uses the originals
// for document length and the whole-word bonus.
func identifierTerms(text string) (terms, originals []string) {
	for _, lexeme := range identifierLexemes(text) {
		whole := strings.ToLower(lexeme)
		originals = append(originals, whole)
		start := len(terms)
		terms = append(terms, whole)
		runes := []rune(lexeme)
		from := 0
		for i, r := range runes {
			if r == '_' || identifierBoundary(runes, i) {
				terms = appendIdentifierPart(terms, start, runes[from:i])
				from = i
				if r == '_' {
					from++
				}
			}
		}
		terms = appendIdentifierPart(terms, start, runes[from:])
	}
	return terms, originals
}

func identifierBoundary(runes []rune, i int) bool {
	if i == 0 || runes[i-1] == '_' {
		return false
	}
	prev, cur := runes[i-1], runes[i]
	if unicode.IsDigit(prev) != unicode.IsDigit(cur) {
		return true
	}
	return unicode.IsUpper(cur) && (unicode.IsLower(prev) ||
		(unicode.IsUpper(prev) && i+1 < len(runes) && unicode.IsLower(runes[i+1])))
}

func appendIdentifierPart(terms []string, start int, part []rune) []string {
	if len(part) > 0 {
		term := strings.ToLower(string(part))
		for _, existing := range terms[start:] {
			if existing == term {
				return terms
			}
		}
		return append(terms, term)
	}
	return terms
}
