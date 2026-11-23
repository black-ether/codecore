package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"unicode"
)

type retrievalResult struct {
	Rank    int      `json:"rank"`
	File    string   `json:"file"`
	Line    int      `json:"line"`
	End     int      `json:"end"`
	Name    string   `json:"name,omitempty"`
	Text    string   `json:"text"`
	Score   float64  `json:"score"`
	Parents []string `json:"parents,omitempty"`
}

func tokenize(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// queryStopwords are the words a natural-language question adds that
// carry no code meaning. They are stripped from the query only, never
// from the code, so "where is auth handled" searches for "auth
// handled". Action words (find, get, show, list) are kept — they can
// be part of a function name.
var queryStopwords = map[string]bool{
	"where": true, "when": true, "why": true, "how": true, "what": true,
	"which": true, "who": true, "whom": true, "whose": true,
	"is": true, "are": true, "was": true, "were": true, "be": true,
	"been": true, "being": true, "do": true, "does": true, "did": true,
	"have": true, "has": true, "had": true, "can": true, "could": true,
	"will": true, "would": true, "should": true, "may": true,
	"might": true, "must": true, "shall": true,
	"the": true, "a": true, "an": true, "and": true, "or": true,
	"but": true, "of": true, "to": true, "in": true, "on": true,
	"for": true, "with": true, "by": true, "at": true, "from": true,
	"as": true,
	"it": true, "its": true, "this": true, "that": true, "these": true,
	"those": true, "i": true, "you": true, "he": true, "she": true,
	"we": true, "they": true, "me": true, "him": true, "her": true,
	"us": true, "them": true, "my": true, "your": true, "his": true,
	"our": true, "their": true,
	"not": true, "no": true, "please": true,
}

// filterQueryTerms drops query stopwords from tokenized query terms.
func filterQueryTerms(terms []string) []string {
	kept := terms[:0]
	for _, term := range terms {
		if !queryStopwords[term] {
			kept = append(kept, term)
		}
	}
	return kept
}

// prefixMatches returns, for a query term, the documents containing it
// or any longer term it prefixes, mapping each document to the
// strongest frequency among its matched terms. A query term of fewer
// than 3 letters matches exactly only, so a one-letter query cannot
// match everything starting with that letter. The document frequency
// is the union over the matched terms, and a document is credited once
// for its strongest match, not once per matched word.
func prefixMatches(postings map[string][]termOccurrence, query string) map[int]float64 {
	result := make(map[int]float64)
	put := func(occurrences []termOccurrence) {
		for _, m := range occurrences {
			if tf := float64(m.frequency); tf > result[m.document] {
				result[m.document] = tf
			}
		}
	}
	put(postings[query])
	if len(query) < 3 {
		return result
	}
	for term, occurrences := range postings {
		if term != query && strings.HasPrefix(term, query) {
			put(occurrences)
		}
	}
	return result
}

type retrievalDocument struct {
	file    string
	node    *Node
	length  int
	parents []string
	end     int
	name    string
}

// definitionKeywords are the words that introduce a definition. A line
// whose first identifier is one of these defines a symbol; a line that
// merely calls or uses a symbol does not. Only definitions get the name
// boost, so a definition ranks above a use of the same name.
var definitionKeywords = map[string]bool{
	"def": true, "func": true, "function": true, "fn": true,
	"class": true, "struct": true, "interface": true, "type": true,
	"let": true, "var": true, "const": true, "local": true,
	"public": true, "private": true, "protected": true, "static": true,
	"async": true, "export": true, "method": true, "proc": true,
	"trait": true, "impl": true, "enum": true,
}

// nameBoost is how many extra times a name's tokens count in BM25. A
// name term therefore counts 1 + nameBoost times (once from the line
// text, nameBoost times from the name), so a definition ranks above a
// mere mention of the same word.
const nameBoost = 2

// extractName returns the key identifier of a line and whether the line
// defines it. The name is the last identifier before the first "(",
// "[", "=", ":" or "{" (the delimiter that starts the parameter list,
// the index, the assignment, or the body), extended backwards over
// ".identifier" segments so a method keeps its dotted path
// (keymap.set, not just set). The "[" matters: in a generic like
// def quick_sort[T: Comparable](...) the ":" inside the brackets is not
// the header colon, and the "[" stops the scan before it. For a
// definition it is the defined name (def quick_sort, func foo,
// class Manager, x = 1); for a call it is the called function
// (return authenticate_user()). An identifier starts with a letter or
// underscore and continues with letters, digits and underscores.
// A line defines its name when its first identifier is a definition
// keyword, or when the delimiter is a single "=" (an assignment, not a
// comparison). Returns ("", false) when there is no usable name.
func extractName(text string) (string, bool) {
	runes := []rune(text)
	// The name is in the text before the first delimiter. A "[" or "{"
	// is a delimiter only when attached to a name (a generic, an index,
	// or a block opener). A standalone list or dict literal is not —
	// the name is inside it (return [*quick_sort(...)] names
	// quick_sort, not return).
	end := len(runes)
	for i, r := range runes {
		if r == '(' || r == '=' || r == ':' {
			end = i
			break
		}
		if r == '[' && i > 0 && (unicode.IsLetter(runes[i-1]) || unicode.IsDigit(runes[i-1]) || runes[i-1] == '_') {
			end = i
			break
		}
		if r == '{' && i > 0 && (runes[i-1] == ')' || runes[i-1] == ':' || unicode.IsLetter(runes[i-1]) || unicode.IsDigit(runes[i-1]) || runes[i-1] == '_') {
			end = i
			break
		}
	}
	prefix := runes[:end]
	// Collect identifiers (with rune starts) in the prefix.
	type id struct {
		text  string
		start int
	}
	var ids []id
	start := -1
	for i, r := range prefix {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			if start < 0 && (unicode.IsLetter(r) || r == '_') {
				start = i
			}
		} else if start >= 0 {
			ids = append(ids, id{string(prefix[start:i]), start})
			start = -1
		}
	}
	if start >= 0 {
		ids = append(ids, id{string(prefix[start:]), start})
	}
	if len(ids) == 0 {
		return "", false
	}
	// A definition starts with a definition keyword, or assigns with a
	// single "=" (not "==", "!=", "<=" or ">=").
	isDef := definitionKeywords[strings.ToLower(ids[0].text)]
	if !isDef && end < len(runes) && runes[end] == '=' {
		after := end+1 < len(runes) && runes[end+1] == '='
		before := end > 0 && (runes[end-1] == '!' || runes[end-1] == '<' || runes[end-1] == '>')
		isDef = !after && !before
	}
	// The name is the last identifier, skipping definition keywords
	// (a Go method with a receiver lands on "func" and has no earlier
	// identifier to fall back to, so it has no usable name), extended
	// backwards over ".identifier" segments.
	for i := len(ids) - 1; i >= 0; i-- {
		if definitionKeywords[strings.ToLower(ids[i].text)] {
			continue
		}
		name := ids[i].text
		pos := ids[i].start
		for pos > 0 && prefix[pos-1] == '.' {
			// Find the identifier ending just before the dot.
			e := pos - 1
			for e > 0 && (unicode.IsLetter(prefix[e-1]) || unicode.IsDigit(prefix[e-1]) || prefix[e-1] == '_') {
				e--
			}
			if e == pos-1 || (!unicode.IsLetter(prefix[e]) && prefix[e] != '_') {
				break
			}
			name = string(prefix[e:pos-1]) + "." + name
			pos = e
		}
		return name, isDef
	}
	return "", false
}

// endLine returns the last line of the outline lines in node's block:
// the greatest line number among node and all its descendants. A leaf's
// end is its own line. For brace languages the closing brace line is not
// an outline node, so it is not included — the range covers the outline
// lines belonging to the block, not the closer.
func endLine(node *Node) int {
	end := node.Line
	for _, child := range node.Children {
		if e := endLine(child); e > end {
			end = e
		}
	}
	return end
}

type termOccurrence struct {
	document  int
	frequency int
}

func retrieve(outlines []*Outline, terms []string, top int) []retrievalResult {
	var documents []retrievalDocument
	postings := make(map[string][]termOccurrence)
	totalLength := 0
	var walk func(string, []*Node, []string)
	walk = func(file string, nodes []*Node, parents []string) {
		for _, node := range nodes {
			tokens := tokenize(node.Text)
			name, isDef := extractName(node.Text)
			// Only a definition's name is boosted, so a definition
			// ranks above a mere use of the same name. The boost
			// raises term frequency only — it does not lengthen the
			// document, so a long definition line is not penalised
			// for the boost itself.
			nameTokens := []string{}
			if isDef {
				nameTokens = tokenize(name)
			}
			length := len(tokens)
			id := len(documents)
			documents = append(documents, retrievalDocument{file, node, length, parents, endLine(node), name})
			totalLength += length
			frequencies := make(map[string]int)
			for _, token := range tokens {
				frequencies[token]++
			}
			for _, token := range nameTokens {
				frequencies[token] += nameBoost
			}
			for term, frequency := range frequencies {
				postings[term] = append(postings[term], termOccurrence{id, frequency})
			}
			if len(node.Children) > 0 {
				// Give each branch its own path so siblings cannot overwrite it.
				walk(file, node.Children, append(parents[:len(parents):len(parents)], strings.TrimSpace(node.Text)))
			}
		}
	}
	for _, outline := range outlines {
		walk(outline.File, outline.Nodes, nil)
	}

	results := make([]retrievalResult, 0)
	if totalLength == 0 {
		return results
	}
	const k1, b = 1.2, 0.75
	n := float64(len(documents))
	avgdl := float64(totalLength) / n
	scores := make([]float64, len(documents))
	// Add scores in query order to keep floating-point sums deterministic.
	// A query term matches the exact term and, when it is at least 3
	// letters, any longer term it prefixes, so "auth" finds
	// "authenticate".
	for _, term := range terms {
		matched := prefixMatches(postings, term)
		if len(matched) == 0 {
			continue
		}
		df := float64(len(matched))
		idf := math.Log(1 + (n-df+0.5)/(df+0.5))
		for doc, tf := range matched {
			dl := float64(documents[doc].length)
			scores[doc] += idf * (tf * (k1 + 1)) / (tf + k1*(1-b+b*dl/avgdl))
		}
	}
	for i, doc := range documents {
		if scores[i] > 0 {
			results = append(results, retrievalResult{
				File: doc.file, Line: doc.node.Line, End: doc.end, Name: doc.name, Text: doc.node.Text,
				Score: scores[i], Parents: doc.parents,
			})
		}
	}
	sort.SliceStable(results, func(i, j int) bool {
		a, b := results[i], results[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.File != b.File {
			return a.File < b.File
		}
		return a.Line < b.Line
	})
	if len(results) > top {
		results = results[:top]
	}
	for i := range results {
		results[i].Rank = i + 1
	}
	return results
}

func formatResults(w io.Writer, format, query string, results []retrievalResult) error {
	if format == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	}
	if len(results) == 0 {
		_, err := fmt.Fprintf(w, "no matches for %q\n", query)
		return err
	}
	for _, result := range results {
		context := ""
		if len(result.Parents) > 0 {
			context = " · in " + strings.Join(result.Parents, " > ")
		}
		// A block shows its range; a single line shows just its line.
		ref := fmt.Sprintf("[L:%d]", result.Line)
		if result.End > result.Line {
			ref = fmt.Sprintf("[L:%d-%d]", result.Line, result.End)
		}
		if _, err := fmt.Fprintf(w, "  %d. %s %s %s\n     score %.2f%s\n",
			result.Rank, result.File, ref, result.Text, result.Score, context); err != nil {
			return err
		}
	}
	return nil
}
