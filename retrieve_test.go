package main

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestTokenize(t *testing.T) {
	got := tokenize(" __authenticate_user--HTTP.2(ÉTÉ)\t")
	want := []string{"authenticate", "user", "http", "2", "été"}
	if !slices.Equal(got, want) {
		t.Errorf("tokens = %q, want %q", got, want)
	}
	if got := tokenize(" _-(). "); len(got) != 0 {
		t.Errorf("separator-only tokens = %q, want none", got)
	}
}

func TestBM25Scoring(t *testing.T) {
	outlines := []*Outline{{File: "terms.go", Nodes: []*Node{
		{Line: 1, Text: "common"},
		{Line: 2, Text: "rare"},
		{Line: 3, Text: "common longer document"},
		{Line: 4, Text: "common"},
	}}}
	got := retrieve(outlines, tokenize("rare common"), 5, false, false, false)
	if len(got) != 4 {
		t.Fatalf("got %d results, want 4", len(got))
	}
	scores := make(map[int]float64)
	for _, result := range got {
		scores[result.Line] = result.Score
	}
	if scores[2] <= scores[1] {
		t.Errorf("rare score %g should exceed common score %g", scores[2], scores[1])
	}
	if scores[1] <= scores[3] {
		t.Errorf("short score %g should exceed long score %g", scores[1], scores[3])
	}
	if scores[1] != scores[4] {
		t.Errorf("identical documents should tie, got %g and %g", scores[1], scores[4])
	}
}

func TestRetrieveRanking(t *testing.T) {
	outlines := []*Outline{
		{File: "z.go", Nodes: []*Node{{Line: 1, Text: "find"}}},
		{File: "a.go", Nodes: []*Node{
			{Line: 8, Text: "find"},
			{Line: 4, Text: "find"},
			{Line: 9, Text: "find find"},
		}},
	}
	want := []struct {
		file string
		line int
	}{{"a.go", 9}, {"a.go", 4}, {"a.go", 8}, {"z.go", 1}}
	for _, top := range []int{2, 5} {
		got := retrieve(outlines, tokenize("find missing"), top, false, false, false)
		if len(got) != min(top, len(want)) {
			t.Fatalf("top %d returned %d results, want %d", top, len(got), min(top, len(want)))
		}
		for i, result := range got {
			if result.Rank != i+1 || result.File != want[i].file || result.Line != want[i].line {
				t.Errorf("top %d result %d = %+v, want rank %d at %+v", top, i, result, i+1, want[i])
			}
		}
	}
	if got := retrieve(outlines, tokenize("missing"), 5, false, false, false); len(got) != 0 {
		t.Errorf("unmatched query returned %+v", got)
	}
}

func TestExtractName(t *testing.T) {
	cases := []struct {
		line string
		name string
		def  bool
	}{
		{"def quick_sort[T: Comparable](collection: list[T]) -> list[T]:", "quick_sort", true},
		{"func foo() {", "foo", true},
		{"class Manager:", "Manager", true},
		{"x = 1", "x", true},
		{"local keymap = {}", "keymap", true},
		{"return authenticate_user()", "authenticate_user", false},
		{"function keymap.set(modes, lhs, rhs, opts)", "keymap.set", true},
		{"function keymap.del(modes, lhs, opts)", "keymap.del", true},
		{"type Foo struct {", "Foo", true},
		{"type Bar interface {", "Bar", true},
		{"func (r *Receiver) Method()", "Method", true},
		{"func (h *MaxHeap) Init(slice []Comparable) {", "Init", true},
		{"func (s Server) Addr() string", "Addr", true},
		{"if x > 0:", "x", false},
		{"if x == y:", "x", false},
		{"x = authenticate_user()", "x", true},
		{"return [*quick_sort(lesser), *equal, *quick_sort(greater)]", "quick_sort", false},
		{"vim.api.nvim_set_keymap(m, l, rhs, opts)", "vim.api.nvim_set_keymap", false},
		{"package sample", "sample", false},
		// Lisp-style definitions open with a paren; the keyword and
		// name are inside it, and a kebab-case name keeps its hyphens.
		{"(defn lazy-heap-sort", "lazy-heap-sort", true},
		{"(defn heap-sort [coll]", "heap-sort", true},
		{"(defmacro with-temp [x]", "with-temp", true},
		{"(defmethod render :html [x]", "render", true},
		{"(ns sorts.heap-sort", "sorts.heap-sort", false},
		{"(x + y)", "y", false},
		{"a - b", "b", false},
	}
	for _, c := range cases {
		name, def := extractName(c.line)
		if name != c.name || def != c.def {
			t.Errorf("extractName(%q) = (%q, %v), want (%q, %v)", c.line, name, def, c.name, c.def)
		}
	}
}

func TestRetrieveNameBoost(t *testing.T) {
	outlines := []*Outline{{File: "auth.py", Nodes: []*Node{
		{Line: 1, Text: "def authenticate_user():"},
		{Line: 5, Text: "return authenticate_user()"},
	}}}
	got := retrieve(outlines, tokenize("authenticate user"), 5, false, false, false)
	if len(got) != 2 {
		t.Fatalf("got %+v, want 2 results", got)
	}
	// The definition ranks above the use of the same name.
	if got[0].Line != 1 || got[0].Name != "authenticate_user" {
		t.Errorf("first result = %+v, want the definition at line 1", got[0])
	}
	if got[1].Line != 5 || got[1].Name != "authenticate_user" {
		t.Errorf("second result = %+v, want the use at line 5", got[1])
	}
	if got[0].Score <= got[1].Score {
		t.Errorf("definition score %g should exceed use score %g", got[0].Score, got[1].Score)
	}
	// The name is carried in the JSON output.
	var buf bytes.Buffer
	if err := formatResults(&buf, "json", "authenticate user", got); err != nil {
		t.Fatal(err)
	}
	var decoded []retrievalResult
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded) != 2 || decoded[0].Name != "authenticate_user" {
		t.Errorf("json output = %+v, want the name on the first result", decoded)
	}
}

func TestFilterQueryTerms(t *testing.T) {
	cases := []struct {
		query string
		want  []string
	}{
		{"where is auth handled", []string{"auth", "handled"}},
		{"how do I sort a list", []string{"sort", "list"}},
		{"authenticate user", []string{"authenticate", "user"}},
		{"where is it", []string{}},
	}
	for _, c := range cases {
		got := filterQueryTerms(tokenize(c.query))
		if len(got) != len(c.want) {
			t.Errorf("filterQueryTerms(%q) = %v, want %v", c.query, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("filterQueryTerms(%q) = %v, want %v", c.query, got, c.want)
				break
			}
		}
	}
}

func TestRetrievePrefixMatch(t *testing.T) {
	outlines := []*Outline{{File: "auth.py", Nodes: []*Node{
		{Line: 1, Text: "def authenticate_user():"},
		{Line: 5, Text: "x = 1"},
	}}}
	// "auth" finds "authenticate" by prefix.
	got := retrieve(outlines, tokenize("auth"), 5, false, false, false)
	if len(got) != 1 || got[0].Line != 1 {
		t.Errorf("retrieve(auth) = %+v, want the authenticate_user definition", got)
	}
	// "aut" (3 letters) also prefixes "authenticate".
	got = retrieve(outlines, tokenize("aut"), 5, false, false, false)
	if len(got) != 1 || got[0].Line != 1 {
		t.Errorf("retrieve(aut) = %+v, want the authenticate_user definition", got)
	}
	// "au" (2 letters) is too short to prefix-match, and matches nothing exactly.
	got = retrieve(outlines, tokenize("au"), 5, false, false, false)
	if len(got) != 0 {
		t.Errorf("retrieve(au) = %+v, want no results", got)
	}
}

func TestRetrieveRanges(t *testing.T) {
	outlines := []*Outline{{File: "ranges.go", Nodes: []*Node{
		{Line: 1, Text: "func outer()", Children: []*Node{
			{Line: 2, Text: "inner one"},
			{Line: 3, Text: "inner two", Children: []*Node{
				{Line: 4, Text: "deep leaf"},
			}},
		}},
		{Line: 5, Text: "standalone"},
	}}}
	// A block's end is its last descendant's line; a leaf's end is its own line.
	got := retrieve(outlines, tokenize("outer"), 5, false, false, false)
	if len(got) != 1 || got[0].Line != 1 || got[0].End != 4 {
		t.Fatalf("outer = %+v, want line 1 end 4", got)
	}
	got = retrieve(outlines, tokenize("inner"), 5, false, false, false)
	if len(got) != 2 {
		t.Fatalf("inner = %+v, want 2 results", got)
	}
	if got[0].Line != 2 || got[0].End != 2 || got[1].Line != 3 || got[1].End != 4 {
		t.Errorf("inner = %+v, want (2,2) then (3,4)", got)
	}
	got = retrieve(outlines, tokenize("standalone"), 5, false, false, false)
	if len(got) != 1 || got[0].Line != 5 || got[0].End != 5 {
		t.Fatalf("standalone = %+v, want line 5 end 5", got)
	}
}

func TestFormatResultsRanges(t *testing.T) {
	results := []retrievalResult{
		{Rank: 1, File: "a.go", Line: 1, End: 4, Text: "func outer()", Score: 2.5},
		{Rank: 2, File: "a.go", Line: 5, End: 5, Text: "standalone", Score: 1.0},
	}
	var buf bytes.Buffer
	if err := formatResults(&buf, "text", "outer", results); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "a.go [L:1-4] func outer()") {
		t.Errorf("text output = %q, want [L:1-4] for the block", got)
	}
	if !strings.Contains(got, "a.go [L:5] standalone") {
		t.Errorf("text output = %q, want [L:5] for the single line", got)
	}
	buf.Reset()
	if err := formatResults(&buf, "json", "outer", results); err != nil {
		t.Fatal(err)
	}
	var decoded []retrievalResult
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded) != 2 || decoded[0].End != 4 || decoded[1].End != 5 {
		t.Errorf("json output = %+v, want ends 4 and 5", decoded)
	}
}

func TestRetrieveFromExtractor(t *testing.T) {
	ext, ok := ExtractorByLanguage("python")
	if !ok {
		t.Fatal("no python extractor")
	}
	src := "class Manager:\n    def check(self):\n        return authenticate_user()\n"
	outline, err := ext.Extract("auth.py", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	outlines := []*Outline{outline}
	got := retrieve(outlines, tokenize("authenticate user"), 5, false, false, false)
	if len(got) != 1 {
		t.Fatalf("got %+v, want one match", got)
	}
	result := got[0]
	if result.File != "auth.py" || result.Line != 3 || result.Text != "return authenticate_user()" {
		t.Errorf("top result = %+v, want auth.py line 3 with the authenticate_user call", result)
	}
	if want := []string{"class Manager:", "def check(self):"}; !slices.Equal(result.Parents, want) {
		t.Errorf("parents = %q, want %q", result.Parents, want)
	}

	got = retrieve(outlines, tokenize("manager"), 5, false, false, false)
	if len(got) != 1 || len(got[0].Parents) != 0 {
		t.Errorf("manager query returned %+v, want one top-level match", got)
	}
}

// Short query terms (under 3 letters) are dropped by the query filter:
// a 1-2 letter term is too generic to search on ("pp" from
// "pp_settings" would match every "pp_*" identifier).
func TestFilterQueryTermsShortTerms(t *testing.T) {
	got := filterQueryTerms(tokenize("pp_settings.json"))
	want := []string{"settings", "json"}
	if !slices.Equal(got, want) {
		t.Errorf("filterQueryTerms(pp_settings.json) = %v, want %v", got, want)
	}
	got = filterQueryTerms(tokenize("go sort"))
	if !slices.Equal(got, []string{"sort"}) {
		t.Errorf("filterQueryTerms(go sort) = %v, want [sort]", got)
	}
	// A query that is only short terms keeps them: "go" is the whole
	// query, not a fragment of a compound name, so it must not error.
	got = filterQueryTerms(tokenize("go"))
	if !slices.Equal(got, []string{"go"}) {
		t.Errorf("filterQueryTerms(go) = %v, want [go]", got)
	}
	got = filterQueryTerms(tokenize("C"))
	if !slices.Equal(got, []string{"c"}) {
		t.Errorf("filterQueryTerms(C) = %v, want [c]", got)
	}
}

// A query term that appears verbatim in the line earns a substring
// bonus on top of BM25, so an exact identifier outranks a line that
// merely shares a word with it.
func TestRetrieveSubstringBonus(t *testing.T) {
	outlines := []*Outline{{File: "warn.ps1", Nodes: []*Node{
		{Line: 1, Text: "Write-Warning $warningMessage"},
		{Line: 2, Text: "$warningMessage = \"already exists\""},
	}}}
	got := retrieve(outlines, tokenize("Write-Warning"), 5, false, false, false)
	if len(got) != 2 {
		t.Fatalf("got %+v, want 2 results", got)
	}
	if got[0].Line != 1 {
		t.Errorf("first result = %+v, want the exact Write-Warning call at line 1", got[0])
	}
	if got[0].Score <= got[1].Score {
		t.Errorf("exact score %g should exceed near-miss score %g", got[0].Score, got[1].Score)
	}
}

// A word that only appears in a node's attached comments still
// finds the node: the comments describe the code, so the code is
// the result.
func TestRetrieveCommentText(t *testing.T) {
	outlines := []*Outline{{File: "check.go", Nodes: []*Node{
		{Line: 1, Text: "func CheckConfig() {}", Comments: []string{"Check for Configuration File"}},
		{Line: 2, Text: "func Other() {}"},
	}}}
	got := retrieve(outlines, tokenize("configuration"), 5, false, false, false)
	if len(got) != 1 {
		t.Fatalf("got %+v, want 1 result", got)
	}
	if !strings.Contains(got[0].Text, "CheckConfig") {
		t.Errorf("result = %+v, want the CheckConfig function", got[0])
	}
}

// The substring bonus is whole-word only: a term that is merely a
// prefix or substring of a longer word ("auth" in "author") must not
// earn it, or author lines outrank the genuinely relevant
// "Authenticode" line. The two lines below have the same token count,
// so BM25 length normalisation cannot decide between them — only the
// bonus can, and the whole-word line must win.
func TestRetrieveSubstringBonusWholeWord(t *testing.T) {
	outlines := []*Outline{{File: "auth.txt", Nodes: []*Node{
		{Line: 1, Text: "<author>Else</author>"},
		{Line: 2, Text: "auth Else"},
	}}}
	got := retrieve(outlines, tokenize("auth"), 5, false, false, false)
	if len(got) != 2 {
		t.Fatalf("got %+v, want 2 results", got)
	}
	if got[0].Line != 2 {
		t.Errorf("first result = %+v, want the exact-word line 2 first", got[0])
	}
}

// With defOnly, only definitions are returned: a use of the same name
// is filtered out even when it scores highly.
func TestRetrieveDefOnly(t *testing.T) {
	outlines := []*Outline{{File: "sort.go", Nodes: []*Node{
		{Line: 1, Text: "func HeapSort(slice []int) []int {"},
		{Line: 5, Text: "HeapSort(arr)"},
	}}}
	got := retrieve(outlines, tokenize("heapsort"), 5, true, false, false)
	if len(got) != 1 || got[0].Line != 1 {
		t.Errorf("defOnly retrieve = %+v, want only the definition at line 1", got)
	}
	got = retrieve(outlines, tokenize("heapsort"), 5, false, false, false)
	if len(got) != 2 {
		t.Errorf("retrieve without defOnly = %+v, want both results", got)
	}
}

// Per-file mode keeps only the best result from each file, so one
// file cannot fill the whole top-N list and push other files out.
func TestRetrievePerFile(t *testing.T) {
	outlines := []*Outline{
		{File: "a.go", Nodes: []*Node{
			{Line: 1, Text: "func HeapSort()"},
			{Line: 5, Text: "HeapSort(arr)"},
		}},
		{File: "b.go", Nodes: []*Node{
			{Line: 1, Text: "func heapify()"},
		}},
	}
	got := retrieve(outlines, tokenize("heapsort heapify"), 5, false, true, false)
	if len(got) != 2 {
		t.Fatalf("per-file retrieve = %+v, want one result per file", got)
	}
	// One result per file, in score order (b.go's rarer "heapify"
	// outscores a.go's "heapsort" — the order is BM25, the dedup is
	// what matters). a.go's kept result is its definition, not the call.
	byFile := make(map[string]retrievalResult)
	for _, r := range got {
		byFile[r.File] = r
	}
	if len(byFile) != 2 || byFile["a.go"].Line != 1 || byFile["b.go"].Line != 1 {
		t.Errorf("per-file retrieve = %+v, want a.go line 1 and b.go line 1", got)
	}
	// Without per-file, the same query returns all three matches.
	got = retrieve(outlines, tokenize("heapsort heapify"), 5, false, false, false)
	if len(got) != 3 {
		t.Errorf("retrieve without per-file = %+v, want all 3 results", got)
	}
}

func TestRetrieveWordOnly(t *testing.T) {
	// --word turns off prefix matching: "auth" must not match
	// "authenticate", and "read" must not match "reader". Without
	// --word both match (the prefix rule). The substring bonus is
	// whole-word only too, so a line containing only "reader" does
	// not earn it for "read".
	outlines := []*Outline{{File: "a.go", Nodes: []*Node{
		{Line: 1, Text: "func authenticate_user(name string)"},
		{Line: 2, Text: "reader := NewReader()"},
		{Line: 3, Text: "read the config file"},
	}}}
	if got := retrieve(outlines, tokenize("auth"), 5, false, false, true); len(got) != 0 {
		t.Errorf("retrieve --word(auth) = %+v, want no results", got)
	}
	if got := retrieve(outlines, tokenize("read"), 5, false, false, true); len(got) != 1 || got[0].Line != 3 {
		t.Errorf("retrieve --word(read) = %+v, want only line 3", got)
	}
	// Without --word, both prefix matches work as before.
	if got := retrieve(outlines, tokenize("auth"), 5, false, false, false); len(got) != 1 || got[0].Line != 1 {
		t.Errorf("retrieve(auth) = %+v, want line 1", got)
	}
	if got := retrieve(outlines, tokenize("read"), 5, false, false, false); len(got) != 2 {
		t.Errorf("retrieve(read) = %+v, want lines 2 and 3", got)
	}
}

func TestCountMatches(t *testing.T) {
	// --count counts ALL matching nodes per file, before the --top
	// cut: a file with 3 matches shows 3, not just the ones that
	// made the top N. Sorted by count desc, then file.
	outlines := []*Outline{
		{File: "b.go", Nodes: []*Node{
			{Line: 1, Text: "sort items"},
			{Line: 2, Text: "sort again"},
		}},
		{File: "a.go", Nodes: []*Node{
			{Line: 1, Text: "sort once"},
			{Line: 2, Text: "sort twice"},
			{Line: 3, Text: "sort thrice"},
		}},
		{File: "c.go", Nodes: []*Node{{Line: 1, Text: "nothing here"}}},
	}
	got := countMatches(outlines, tokenize("sort"), false, false)
	want := []fileCount{{"a.go", 3}, {"b.go", 2}}
	if !slices.Equal(got, want) {
		t.Errorf("countMatches = %+v, want %+v", got, want)
	}
	if got := countMatches(outlines, tokenize("missing"), false, false); len(got) != 0 {
		t.Errorf("countMatches(missing) = %+v, want none", got)
	}
}

func TestFormatFileNames(t *testing.T) {
	// --filename-only lists each matching file once, in rank order
	// (the file with the best match first). The "no matches" message
	// is the same as the results format.
	results := []retrievalResult{
		{Rank: 1, File: "b.go", Line: 5, Text: "sort"},
		{Rank: 2, File: "a.go", Line: 1, Text: "sort"},
		{Rank: 3, File: "b.go", Line: 9, Text: "sort again"},
	}
	var buf bytes.Buffer
	if err := formatFileNames(&buf, "text", "sort", results); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "b.go\na.go\n" {
		t.Errorf("formatFileNames = %q, want %q", got, "b.go\na.go\n")
	}
	buf.Reset()
	if err := formatFileNames(&buf, "text", "missing", nil); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "no matches for \"missing\"\n" {
		t.Errorf("formatFileNames(no matches) = %q, want the no-matches message", got)
	}
	buf.Reset()
	if err := formatFileNames(&buf, "json", "sort", results); err != nil {
		t.Fatal(err)
	}
	var files []string
	if err := json.Unmarshal(buf.Bytes(), &files); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(files, []string{"b.go", "a.go"}) {
		t.Errorf("formatFileNames(json) = %q, want [b.go a.go]", files)
	}
}

func TestFormatCounts(t *testing.T) {
	counts := []fileCount{{"a.go", 3}, {"b.go", 2}}
	var buf bytes.Buffer
	if err := formatCounts(&buf, "text", "sort", counts); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "  3 a.go\n  2 b.go\n" {
		t.Errorf("formatCounts = %q, want %q", got, "  3 a.go\n  2 b.go\n")
	}
	buf.Reset()
	if err := formatCounts(&buf, "json", "sort", counts); err != nil {
		t.Fatal(err)
	}
	var decoded []fileCount
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(decoded, counts) {
		t.Errorf("formatCounts(json) = %+v, want %+v", decoded, counts)
	}
}
