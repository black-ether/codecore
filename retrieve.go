package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"slices"
	"sort"
	"strings"
	"unicode"
)

type retrievalResult struct {
	Explanation *rankingExplanation `json:"explanation,omitempty"`
	Rank        int                 `json:"rank"`
	File        string              `json:"file"`
	Line        int                 `json:"line"`
	End         int                 `json:"end"`
	Name        string              `json:"name,omitempty"`
	Text        string              `json:"text"`
	Score       float64             `json:"score"`
	Parents     []string            `json:"parents,omitempty"`
	Unit        *structuralUnit     `json:"unit,omitempty"`
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
// Terms shorter than 3 letters are also dropped: a 1-2 letter term is
// too generic to search on (e.g. "pp" from "pp_settings" would match
// every "pp_*" identifier). Dropping them here, in the query filter
// only, keeps short tokens in the code index untouched.
// filterQueryTerms drops stopwords, and drops terms under 3 letters
// only when a longer term survives. A short term is usually a
// fragment of a compound name (the "pp" in "pp_settings.json" matched
// every pp_* identifier), but when the whole query is short ("go",
// "id", "C") the short terms ARE the query, and dropping them made
// those queries fail outright. A short term that survives matches
// exactly only: prefix matching and the substring bonus both skip
// terms under 3 letters, so "go" finds "go" but "pp" alone does not
// gain the prefix expansion that made it noisy.
func filterQueryTerms(terms []string) []string {
	var kept []string
	for _, term := range terms {
		if !queryStopwords[term] {
			kept = append(kept, term)
		}
	}
	hasLong := false
	for _, term := range kept {
		if len(term) >= 3 {
			hasLong = true
			break
		}
	}
	if !hasLong {
		return kept
	}
	out := make([]string, 0, len(kept))
	for _, term := range kept {
		if len(term) >= 3 {
			out = append(out, term)
		}
	}
	return out
}

// prefixMatches returns, for a query term, the documents containing it
// or any longer term it prefixes, mapping each document to the
// strongest frequency among its matched terms. A query term of fewer
// than 3 letters matches exactly only, so a one-letter query cannot
// match everything starting with that letter. With wordOnly (the
// --word flag) there is no prefix expansion at all: the term must
// match a whole token, so "read" does not match "reader". The document
// frequency is the union over the matched terms, and a document is
// credited once for its strongest match, not once per matched word.
func prefixMatches(postings map[string][]termOccurrence, query string, wordOnly bool) map[int]float64 {
	result := make(map[int]float64)
	put := func(occurrences []termOccurrence) {
		for _, m := range occurrences {
			if tf := float64(m.frequency); tf > result[m.document] {
				result[m.document] = tf
			}
		}
	}
	put(postings[query])
	if wordOnly || len(query) < 3 {
		return result
	}
	for term, occurrences := range postings {
		if term != query && strings.HasPrefix(term, query) {
			put(occurrences)
		}
	}
	return result
}

// containsWord reports whether term appears as a whole word (a token,
// not part of a longer token) in text. Used for the substring bonus in
// --word mode, where a prefix match must not earn the bonus: "read"
// must not boost a line that only contains "reader".
func containsWord(text, term string) bool {
	for _, token := range tokenize(text) {
		if token == term {
			return true
		}
	}
	return false
}

type retrievalOptions struct {
	splitIdentifiers bool
	explainRanking   bool
	groupStructure   bool
	docPassages      map[string]map[int]passageLine
}

type retrievalDocument struct {
	file       string
	node       *Node
	length     int
	parents    []string
	end        int
	name       string
	isDef      bool
	lowerText  string
	wholeTerms []string
	nameTerms  []string
	unit       *structuralUnit
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
	// Lisp/Clojure definitions, which open with a paren: (defn foo,
	// (defmacro bar, (defmethod baz.
	"defn": true, "defmacro": true, "defmethod": true,
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
	// Lisp-style definitions open with a paren: "(defn foo" names
	// "foo". Strip leading whitespace and opening parens so the
	// keyword and name are visible to the scan below. A plain
	// parenthesised expression ("(x + y)") just yields its last
	// identifier with isDef false, which is harmless.
	text = strings.TrimLeft(text, " \t(")
	runes := []rune(text)
	// Find the first identifier: on a definition header a bracket
	// starts the parameter list even after a space ("(defn f [x]"),
	// while in an expression a bracket after a space is a literal
	// ("return [*quick_sort(...)]" names quick_sort, inside the
	// brackets). So the bracket rules below depend on whether the
	// line opens with a definition keyword.
	firstID := ""
	for i, r := range runes {
		if unicode.IsLetter(r) || r == '_' {
			j := i
			for j < len(runes) && (unicode.IsLetter(runes[j]) || unicode.IsDigit(runes[j]) || runes[j] == '_') {
				j++
			}
			firstID = string(runes[i:j])
			break
		}
		if unicode.IsDigit(r) {
			break
		}
	}
	isDefHeader := definitionKeywords[strings.ToLower(firstID)]
	// The name is in the text before the first delimiter. A "[" or "{"
	// is a delimiter only when attached to a name (a generic, an index,
	// or a block opener), or — on a definition header — anywhere, since
	// it starts the parameter list. A standalone list or dict literal
	// is not a delimiter: the name is inside it (return
	// [*quick_sort(...)] names quick_sort, not return).
	end := len(runes)
	for i, r := range runes {
		if r == '(' || r == '=' || r == ':' {
			end = i
			break
		}
		if r == '[' && (isDefHeader || (i > 0 && (unicode.IsLetter(runes[i-1]) || unicode.IsDigit(runes[i-1]) || runes[i-1] == '_'))) {
			end = i
			break
		}
		if r == '{' && (isDefHeader || (i > 0 && (runes[i-1] == ')' || runes[i-1] == ':' || unicode.IsLetter(runes[i-1]) || unicode.IsDigit(runes[i-1]) || runes[i-1] == '_'))) {
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
	// identifier to fall back to, so it has no usable name — see the
	// receiver check below), extended backwards over ".identifier"
	// segments (a method keeps its dotted path) and over "-identifier"
	// segments (a kebab-case name like lazy-heap-sort keeps its
	// hyphens; the hyphen must sit between two identifier characters,
	// so "a - b" does not extend).
	for i := len(ids) - 1; i >= 0; i-- {
		if definitionKeywords[strings.ToLower(ids[i].text)] {
			continue
		}
		name := ids[i].text
		pos := ids[i].start
		for pos > 0 {
			sep := prefix[pos-1]
			if sep != '.' && sep != '-' {
				break
			}
			// A hyphen only joins a kebab-case name when it sits
			// between two identifier characters.
			if sep == '-' && (pos < 2 || (!unicode.IsLetter(prefix[pos-2]) && !unicode.IsDigit(prefix[pos-2]) && prefix[pos-2] != '_')) {
				break
			}
			// Find the identifier ending just before the separator.
			e := pos - 1
			for e > 0 && (unicode.IsLetter(prefix[e-1]) || unicode.IsDigit(prefix[e-1]) || prefix[e-1] == '_') {
				e--
			}
			if e == pos-1 || (!unicode.IsLetter(prefix[e]) && prefix[e] != '_') {
				break
			}
			name = string(prefix[e:pos-1]) + string(sep) + name
			pos = e
		}
		return name, isDef
	}
	// A method with a receiver: the prefix is just the definition
	// keyword ("func") and the method name hides behind the
	// parenthesised receiver, so no name was found above. When the
	// first delimiter is "(" on a definition header, skip the receiver
	// (matching parens) and take the first identifier after it. The
	// name is the method name alone ("Init"), not the receiver type —
	// enough for the name boost and for --def-only to keep it.
	if isDefHeader && end < len(runes) && runes[end] == '(' {
		depth := 0
		for i := end; i < len(runes); i++ {
			if runes[i] == '(' {
				depth++
			} else if runes[i] == ')' {
				depth--
				if depth == 0 {
					j := i + 1
					for j < len(runes) && !unicode.IsLetter(runes[j]) && runes[j] != '_' {
						j++
					}
					k := j
					for k < len(runes) && (unicode.IsLetter(runes[k]) || unicode.IsDigit(runes[k]) || runes[k] == '_') {
						k++
					}
					if j < k {
						return string(runes[j:k]), isDef
					}
					break
				}
			}
		}
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

// allMatches scores every outline node against the query terms and
// returns all matches (score > 0), sorted by score desc, then file,
// then line. It does not apply --per-file, --top, or ranks —
// retrieve() does that, and countMatches() aggregates it. The
// wordOnly flag (--word) turns off prefix matching and makes the
// substring bonus whole-word only, so "read" does not match "reader".
func (opts retrievalOptions) allMatches(outlines []*Outline, terms []string, defOnly bool, wordOnly bool) []retrievalResult {
	var documents []retrievalDocument
	postings := make(map[string][]termOccurrence)
	totalLength := 0
	var walk func(*Outline, []*Node, []string, *structuralUnit, int)
	walk = func(outline *Outline, nodes []*Node, parents []string, enclosing *structuralUnit, parentLine int) {
		for _, node := range nodes {
			// The searchable text is the node's line plus the
			// comments attached to it, so a word that only appears
			// in a comment still finds the code the comment
			// describes. The name still comes from the code line
			// alone, not the comments. The document length grows
			// with the comments, so BM25 normalises a long comment
			// instead of letting it dominate.
			searchText := node.Text
			if len(node.Comments) > 0 {
				searchText += " " + strings.Join(node.Comments, " ")
			}
			var tokens, wholeTerms []string
			if opts.splitIdentifiers {
				tokens, wholeTerms = identifierTerms(searchText)
			} else {
				tokens = tokenize(searchText)
			}
			name, isDef := extractName(node.Text)
			// Only a definition's name is boosted, so a definition
			// ranks above a mere use of the same name. The boost
			// raises term frequency only — it does not lengthen the
			// document, so a long definition line is not penalised
			// for the boost itself.
			nameTokens := []string{}
			if isDef {
				if opts.splitIdentifiers {
					nameTokens = wholeIdentifierTerms(name)
				} else {
					nameTokens = tokenize(name)
				}
			}
			length := len(tokens)
			if opts.splitIdentifiers {
				length = len(wholeTerms)
			}
			end := endLine(node)
			unit := enclosing
			if opts.groupStructure {
				unit = enclosingUnit(outline.Language, node, enclosing, parentLine)
			}
			id := len(documents)
			documents = append(documents, retrievalDocument{
				file: outline.File, node: node, length: length, parents: parents, end: end,
				name: name, nameTerms: nameTokens, isDef: isDef, lowerText: strings.ToLower(searchText), wholeTerms: wholeTerms, unit: unit,
			})
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
				walk(outline, node.Children, append(parents[:len(parents):len(parents)], strings.TrimSpace(node.Text)), unit, node.Line)
			}
		}
	}
	for _, outline := range outlines {
		walk(outline, outline.Nodes, nil, nil, 0)
	}

	results := make([]retrievalResult, 0)
	if totalLength == 0 {
		return results
	}
	const k1, b = 1.2, 0.75
	// substringBonus rewards a query term that appears as a whole word
	// in the node's line, so an exact identifier like Write-Warning
	// outranks a line that merely contains the word "warning". Applied
	// once per term per document, on top of BM25, whether or not the
	// term matched as a token. It is whole-word only in every mode:
	// a plain substring check also rewarded partial-word matches, so
	// "auth" collected the bonus on "author" lines ("<author>Else")
	// and outranked the genuinely relevant "Authenticode" line. A
	// term that only prefixes a longer word ("auth" in
	// "authenticate") still scores through BM25 prefix matching; the
	// bonus is reserved for the exact word.
	const substringBonus = 3.0
	n := float64(len(documents))
	avgdl := float64(totalLength) / n
	scores := make([]float64, len(documents))
	var trace []rankingExplanation
	if opts.explainRanking {
		trace = make([]rankingExplanation, len(documents))
	}
	// Add scores in query order to keep floating-point sums deterministic.
	// A query term matches the exact term and, when it is at least 3
	// letters, any longer term it prefixes, so "auth" finds
	// "authenticate" — unless --word is on.
	for _, term := range terms {
		matched := prefixMatches(postings, term, wordOnly)
		df := float64(len(matched))
		if df > 0 {
			idf := math.Log(1 + (n-df+0.5)/(df+0.5))
			for doc, tf := range matched {
				dl := float64(documents[doc].length)
				contribution := idf * (tf * (k1 + 1)) / (tf + k1*(1-b+b*dl/avgdl))
				scores[doc] += contribution
				if trace != nil {
					trace[doc].Terms = append(trace[doc].Terms, rankingTerm{Term: term, TF: int(tf), DF: int(df), IDF: idf, BM25: contribution})
				}
			}
		}
		for i, doc := range documents {
			// Terms under 3 letters are skipped for the substring bonus:
			// a 2-letter substring appears inside too many words to mean
			// anything (e.g. "au" inside "authenticate").
			if len(term) < 3 {
				continue
			}
			if (opts.splitIdentifiers && slices.Contains(doc.wholeTerms, term)) ||
				(!opts.splitIdentifiers && containsWord(doc.lowerText, term)) {
				scores[i] += substringBonus
				if trace != nil {
					if _, ok := matched[i]; ok {
						trace[i].Terms[len(trace[i].Terms)-1].Bonus = substringBonus
					} else {
						trace[i].Terms = append(trace[i].Terms, rankingTerm{Term: term, DF: int(df), Bonus: substringBonus})
					}
				}
			}
		}
	}
	for i, doc := range documents {
		if scores[i] > 0 {
			if defOnly && !doc.isDef {
				continue
			}
			var explanation *rankingExplanation
			if trace != nil {
				explanation = &trace[i]
				explanation.DocumentLength = doc.length
				explanation.MeanLength = avgdl
				explanation.Documents = len(documents)
				explanation.DefinitionTerms = doc.nameTerms
				distinct := make(map[string]bool)
				for _, detail := range explanation.Terms {
					distinct[detail.Term] = true
				}
				explanation.Covered = len(distinct)
			}
			results = append(results, retrievalResult{
				Explanation: explanation,
				File:        doc.file, Line: doc.node.Line, End: doc.end, Name: doc.name, Text: doc.node.Text,
				Score: scores[i], Parents: doc.parents, Unit: doc.unit,
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
	return results
}

// retrieve returns the top-ranked matches for a query: allMatches,
// then the --per-file dedup (best result per file only), then the
// --top cut, then ranks.
func (opts retrievalOptions) retrieve(outlines []*Outline, terms []string, top int, defOnly bool, perFile bool, wordOnly bool) []retrievalResult {
	results := opts.allMatches(outlines, terms, defOnly, wordOnly)
	if opts.docPassages != nil {
		results = suppressDuplicateDocs(results, opts.docPassages)
	}
	if opts.groupStructure {
		results = groupStructuralMatches(results)
	}
	// Per-file mode keeps only the best result from each file, so one
	// file cannot fill the whole top-N list and push other files out
	// (in a repo-wide search the Go file took 4 of the top 10 slots,
	// hiding the Clojure implementation entirely). The dedup runs
	// before the top cut, so --top then limits the number of files.
	if perFile {
		seen := make(map[string]bool)
		kept := results[:0]
		for _, r := range results {
			if seen[r.File] {
				continue
			}
			seen[r.File] = true
			kept = append(kept, r)
		}
		results = kept
	}
	if len(results) > top {
		results = results[:top]
	}
	for i := range results {
		results[i].Rank = i + 1
	}
	return results
}

// fileCount is one file's match count for --count.
type fileCount struct {
	File  string `json:"file"`
	Count int    `json:"count"`
}

// countMatches returns, for each file, how many outline nodes match
// the query (score > 0), before any --top cut. A file with many
// matches shows its full count, not just the ones that made the top
// N. Sorted by count desc, then file.
func (opts retrievalOptions) countMatches(outlines []*Outline, terms []string, defOnly bool, wordOnly bool) []fileCount {
	counts := make(map[string]int)
	for _, r := range opts.allMatches(outlines, terms, defOnly, wordOnly) {
		counts[r.File]++
	}
	result := make([]fileCount, 0, len(counts))
	for file, count := range counts {
		result = append(result, fileCount{file, count})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Count != result[j].Count {
			return result[i].Count > result[j].Count
		}
		return result[i].File < result[j].File
	})
	return result
}

// formatFileNames prints the files that have matches, one per line,
// deduplicated in rank order (so the first file listed is the one
// with the best match). In JSON mode it emits an array of file names.
func formatFileNames(w io.Writer, format, query string, results []retrievalResult) error {
	seen := make(map[string]bool)
	var files []string
	for _, r := range results {
		if !seen[r.File] {
			seen[r.File] = true
			files = append(files, r.File)
		}
	}
	if format == "json" {
		if files == nil {
			files = []string{}
		}
		return json.NewEncoder(w).Encode(files)
	}
	if len(files) == 0 {
		_, err := fmt.Fprintf(w, "no matches for %q\n", query)
		return err
	}
	for _, f := range files {
		if _, err := fmt.Fprintln(w, f); err != nil {
			return err
		}
	}
	return nil
}

// formatCounts prints the number of matching outline nodes per file,
// most matches first. In JSON mode it emits an array of
// {file, count} objects.
func formatCounts(w io.Writer, format, query string, counts []fileCount) error {
	if format == "json" {
		if counts == nil {
			counts = []fileCount{}
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(counts)
	}
	if len(counts) == 0 {
		_, err := fmt.Fprintf(w, "no matches for %q\n", query)
		return err
	}
	for _, c := range counts {
		if _, err := fmt.Fprintf(w, "  %d %s\n", c.Count, c.File); err != nil {
			return err
		}
	}
	return nil
}

// formatResults prints ranked --ask results: one entry per result with
// its file, line range, text, and score. In JSON mode it emits the
// result array.
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
		if unit := result.Unit; unit != nil {
			if _, err := fmt.Fprintf(w, "  %d. %s %s %s %s\n     match L:%d %s (score %.2f)\n",
				result.Rank, result.File, sourceRangeLabel(unit.Start, unit.End, unit.OutlineEnd, unit.SourceEndUnknown),
				unit.Kind, unit.Name, result.Line, result.Text, result.Score); err != nil {
				return err
			}
			continue
		}
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
